// Package server はHTTP/SSE層。単一の公開APIをWeb UIもスクリプトも共用する。
// 認証は2経路（人間=stateless HMAC Cookie / スクリプト=Bearer パスワード）。
// 状態変更系（start/stop/command）は POST 限定。長時間操作（start/stop）は
// 即「受付」を返し、進捗・状態はSSEで配信する。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/hlconfig"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/selfupdate"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/steam"
)

type Server struct {
	cfg       *config.Config
	cfgPath   string
	dataDir   string // cfgPath のディレクトリ（runtime-state / .run / 既定configDir の基点）
	configDir string // headless config 格納ディレクトリ（解決済み）
	driver    *headless.Driver
	worlds    headless.WorldsService
	auth      *authManager
	webFS     fs.FS
	resonite  *resonite.Client     // Resonite 公開API（ユーザー検索）
	restart   *restartOrchestrator // 自動再起動の進行管理（Phase 8・P8-3）
	scheduler *restartScheduler    // 予定再起動の発火（Phase 8・P8-4a）
	crashMon  *crashMonitor        // クラッシュ自動復帰（Phase 8・P8-4b）
	steam     *steam.Manager       // Resonite 入手/更新（DepotDownloader・P9-B）

	// 自己更新（docs/design/self-update.md）。updater は main が注入する
	// （SetUpdater。未注入＝テストでは update API が 503 を返す）。
	updater      *selfupdate.Updater
	updateMu     sync.Mutex // updateStaged の保護
	updateStaged string     // 適用済み・再起動待ちの版（プロセス内のみ。再起動後は実体が追いつく）

	// requestShutdown は MRHC プロセスの終了依頼を main へ伝える（自己更新後の
	// 「今すぐ終了」）。main が Listen 前に設定する（serving 中の書き換えはしない）。
	requestShutdown func()

	// checkDeps は依存検出（R-C）。本番は platform.CheckHeadlessDeps・テストで偽装する。
	checkDeps func(goos, goarch string) []platform.DepIssue

	// 起動時ガード（.NET ランタイム自動設置・docs/design/dotnet-runtime.md）の seam。
	// system 判定は実マシンの dotnet・installRuntime はネットに依存するためテストで偽装する。
	readRuntimeReq  func(headlessDir string) (platform.RuntimeRequirement, bool)
	localSatisfies  func(installDir string, req platform.RuntimeRequirement, goarch string) bool
	systemSatisfies func(goos, goarch string, req platform.RuntimeRequirement) bool
	installRuntime  func(ctx context.Context, installDir string) error
	steamRunning    func() bool // Steam 更新が進行中か（ガード経路の起動見送り判定）

	// sysDotnetOK は「システム .NET が要求を満たす」と確認できた組のキャッシュ
	// （installDir+要求版）。Steam クライアント併用等でローカル設置が恒久に無い環境が、
	// 起動のたびに非同期経路＋ probe（最大10s のサブプロセス）へ落ちるのを防ぐ。
	sysDotnetMu sync.Mutex
	sysDotnetOK map[string]bool

	// bgCtx は Start() が設定する背景 ctx（shutdown で中断）。ガード goroutine の親に使う。
	bgMu  sync.Mutex
	bgCtx context.Context

	// cfgMu は cfg の実行時書き換え（credentials PUT / password 変更 / app-settings）と、
	// それらを読む経路（auth の署名鍵・起動時の creds/パス読取）の競合を防ぐ。
	// auth は &cfgMu を共有する。レート制限状態は auth.mu（別ロック）。
	cfgMu sync.RWMutex

	// runtimeMu は runtime-state.json（last-used / 最終起動）の read-modify-write を直列化する
	// （handleStart〔HTTP〕と orchestrator/crash-monitor〔goroutine〕からの並行書き込みを防ぐ）。
	runtimeMu sync.Mutex

	// favMu は favorites.json（ワールドお気に入り）の read-modify-write を直列化する
	// （add/remove の並行リクエストでの取りこぼし・上書きを防ぐ）。
	favMu sync.Mutex
}

func New(cfg *config.Config, cfgPath string, driver *headless.Driver, reso *resonite.Client, webFS fs.FS) *Server {
	dataDir := ""
	if cfgPath != "" {
		dataDir = filepath.Dir(cfgPath)
	}
	s := &Server{
		cfg:       cfg,
		cfgPath:   cfgPath,
		dataDir:   dataDir,
		configDir: cfg.HeadlessConfigDirOrDefault(dataDir),
		driver:    driver,
		worlds:    headless.NewWorldsService(driver),
		webFS:     webFS,
		resonite:  reso,
		checkDeps: platform.CheckHeadlessDeps,
	}
	s.auth = newAuthManager(cfg, &s.cfgMu)
	s.restart = newRestartOrchestrator(s)
	s.scheduler = newRestartScheduler(s)
	s.crashMon = newCrashMonitor(s)
	toolsDir := ""
	if dataDir != "" {
		toolsDir = filepath.Join(dataDir, "tools")
	}
	s.steam = steam.NewManager(toolsDir)
	s.readRuntimeReq = platform.ReadRuntimeRequirement
	s.localSatisfies = platform.LocalRuntimeSatisfies
	s.systemSatisfies = platform.SystemRuntimeSatisfies
	s.installRuntime = func(ctx context.Context, installDir string) error {
		return s.steam.InstallRuntime(ctx, installDir)
	}
	s.steamRunning = func() bool { return s.steam.Status().State == "running" }
	return s
}

// Start はバックグラウンドワーカー（scheduler・P8-4a／後続で crash-monitor）を起動し、
// 停止関数を返す。停止関数は bg ctx を cancel（ワーカー停止）し、進行中の再起動を
// best-effort で中止する（①②③のみ。④以降は仕様上 cancel 不可）。main から起動時に1回呼ぶ。
func (s *Server) Start() (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	s.bgMu.Lock()
	s.bgCtx = ctx // 起動時ガード goroutine の親（shutdown で設置を中断）
	s.bgMu.Unlock()
	s.restart.setParent(ctx)                                  // 進行中フローを bg ctx の子に＝shutdown で ①②③ を cancel
	s.steam.SetParent(ctx)                                    // 進行中の更新を bg ctx の子に＝shutdown で中断（P9-B）
	s.driver.SetOnUnexpectedExit(s.crashMon.onUnexpectedExit) // クラッシュ検知を crash-monitor へ（P8-4b）
	go s.scheduler.run(ctx)
	go s.crashMon.run(ctx)
	return func() {
		cancel()
		_ = s.restart.Cancel()
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 既存（プロセスライフサイクル・raw コマンド・SSE）
	mux.HandleFunc("POST /api/v1/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/v1/status", s.requireAuth(s.handleStatus))
	mux.HandleFunc("POST /api/v1/start", s.requireAuth(s.handleStart))                // 状態変更=POST限定
	mux.HandleFunc("POST /api/v1/stop", s.requireAuth(s.handleStop))                  // 強制停止（即時）=POST限定
	mux.HandleFunc("POST /api/v1/stop/graceful", s.requireAuth(s.handleGracefulStop)) // 通常停止（事前アクション→2分→停止・R7）
	mux.HandleFunc("POST /api/v1/command", s.requireAuth(s.handleCommand))            // 副作用あり=POST限定
	mux.HandleFunc("GET /api/v1/events", s.requireAuth(s.handleEvents))

	// 構造化API（Phase 4: Exec/WorldsService を介して構造化データを返す）
	mux.HandleFunc("GET /api/v1/sessions", s.requireAuth(s.handleSessions))
	mux.HandleFunc("GET /api/v1/sessions/{idx}/status", s.requireAuth(s.handleSessionStatus))
	mux.HandleFunc("GET /api/v1/sessions/{idx}/users", s.requireAuth(s.handleSessionUsers))
	mux.HandleFunc("GET /api/v1/sessions/{idx}/detail", s.requireAuth(s.handleSessionDetail))
	mux.HandleFunc("GET /api/v1/listbans", s.requireAuth(s.handleListBans))
	mux.HandleFunc("GET /api/v1/friendrequests", s.requireAuth(s.handleFriendRequests))

	// Headless Config CRUD（Pre-7b）。{name} ワイルドカードより literal の last-used が優先される。
	// POST（コレクション/duplicate）は即時作成方式: 押した瞬間にサーバーが採番して実体を作る。
	mux.HandleFunc("GET /api/v1/headless-configs", s.requireAuth(s.handleConfigList))
	mux.HandleFunc("POST /api/v1/headless-configs", s.requireAuth(s.handleConfigCreate))
	mux.HandleFunc("GET /api/v1/headless-config-defaults", s.requireAuth(s.handleConfigDefaults))
	mux.HandleFunc("GET /api/v1/headless-configs/last-used", s.requireAuth(s.handleConfigLastUsed))
	mux.HandleFunc("GET /api/v1/headless-configs/{name}", s.requireAuth(s.handleConfigGet))
	mux.HandleFunc("PUT /api/v1/headless-configs/{name}", s.requireAuth(s.handleConfigPut))
	mux.HandleFunc("DELETE /api/v1/headless-configs/{name}", s.requireAuth(s.handleConfigDelete))
	mux.HandleFunc("POST /api/v1/headless-configs/{name}/duplicate", s.requireAuth(s.handleConfigDuplicate))
	mux.HandleFunc("GET /api/v1/headless-credentials", s.requireAuth(s.handleCredentialsGet))
	mux.HandleFunc("PUT /api/v1/headless-credentials", s.requireAuth(s.handleCredentialsPut))

	// 設定タブ（7-5）: 管理パスワード変更 + アプリ設定 CRUD。
	mux.HandleFunc("POST /api/v1/password", s.requireAuth(s.handlePasswordChange))
	mux.HandleFunc("GET /api/v1/app-settings", s.requireAuth(s.handleAppSettingsGet))
	mux.HandleFunc("PUT /api/v1/app-settings", s.requireAuth(s.handleAppSettingsPut))

	// スケジュール（Phase 8・§3.16）: 自動再起動 設定/状態（P8-1）＋手動トリガー/中止（P8-3b）。
	mux.HandleFunc("GET /api/v1/restart-config", s.requireAuth(s.handleRestartConfigGet))
	mux.HandleFunc("PUT /api/v1/restart-config", s.requireAuth(s.handleRestartConfigPut))
	mux.HandleFunc("GET /api/v1/restart-status", s.requireAuth(s.handleRestartStatus))
	mux.HandleFunc("POST /api/v1/restart/trigger", s.requireAuth(s.handleRestartTrigger))
	mux.HandleFunc("POST /api/v1/restart/cancel", s.requireAuth(s.handleRestartCancel))

	// write API（Pre-7c）。全 POST・認証必須・idx は path・引数は JSON body。
	// セッション内ユーザー操作（focus idx → <cmd> "<user>"）
	mux.HandleFunc("POST /api/v1/sessions/{idx}/kick", s.requireAuth(s.sessionUserOp("kick")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/ban", s.requireAuth(s.sessionUserOp("ban")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/silence", s.requireAuth(s.sessionUserOp("silence")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/unsilence", s.requireAuth(s.sessionUserOp("unsilence")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/respawn", s.requireAuth(s.sessionUserOp("respawn")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/invite", s.requireAuth(s.sessionUserOp("invite")))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/role", s.requireAuth(s.handleSessionRole))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/message", s.requireAuth(s.handleSessionMessage))
	// セッション設定（focus idx → <cmd>）
	mux.HandleFunc("POST /api/v1/sessions/{idx}/accesslevel", s.requireAuth(s.handleSessionAccessLevel))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/maxusers", s.requireAuth(s.handleSessionMaxUsers))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/name", s.requireAuth(s.handleSessionName))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/description", s.requireAuth(s.handleSessionDescription))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/hidefromlisting", s.requireAuth(s.handleSessionHideFromListing))
	// セッション内コンテンツ操作（focus idx → <cmd>・R14）
	mux.HandleFunc("POST /api/v1/sessions/{idx}/spawn", s.requireAuth(s.handleSessionSpawn))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/impulse", s.requireAuth(s.handleSessionImpulse))
	// セッションライフサイクル（focus idx → <cmd>）
	mux.HandleFunc("POST /api/v1/sessions/{idx}/restart", s.requireAuth(s.sessionCmdOp("restart", headless.WithTimeout(restartTimeout))))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/save", s.requireAuth(s.sessionCmdOp("save", headless.WithTimeout(saveTimeout))))
	mux.HandleFunc("POST /api/v1/sessions/{idx}/close", s.requireAuth(s.sessionCmdOp("close", headless.WithTimeout(closeTimeout))))
	// 新規セッション（focus 不要・長 timeout）。literal "start" は {idx} と段数が違うため衝突しない。
	mux.HandleFunc("POST /api/v1/sessions/start", s.requireAuth(s.handleSessionStart))
	// グローバル（フレンド/BAN・focus 不要）
	mux.HandleFunc("POST /api/v1/friendrequests/accept", s.requireAuth(s.globalUserOp("acceptfriendrequest")))
	mux.HandleFunc("POST /api/v1/friends/add", s.requireAuth(s.globalUserOp("sendFriendRequest")))
	mux.HandleFunc("POST /api/v1/friends/remove", s.requireAuth(s.globalUserOp("removeFriend")))
	mux.HandleFunc("POST /api/v1/bans/unban", s.requireAuth(s.handleBanUnban))
	mux.HandleFunc("POST /api/v1/bans/banByID", s.requireAuth(s.handleBanByID)) // ID 指定 BAN（検索結果・R1）

	// Resonite 公開API（ユーザー検索）。フレンド申請/招待の相手探しに使う（無認証プロキシ・P9-A）。
	mux.HandleFunc("GET /api/v1/resonite/users", s.requireAuth(s.handleResoniteUserSearch))
	mux.HandleFunc("GET /api/v1/resonite/worlds", s.requireAuth(s.handleResoniteWorldSearch))

	// Steam（DepotDownloader）: Resonite の入手/更新（P9-B）。更新は停止中のみ・長時間操作は非同期。
	mux.HandleFunc("GET /api/v1/steam/config", s.requireAuth(s.handleSteamConfigGet))
	mux.HandleFunc("PUT /api/v1/steam/config", s.requireAuth(s.handleSteamConfigPut))
	mux.HandleFunc("POST /api/v1/steam/download", s.requireAuth(s.handleSteamDownload)) // 入手/更新を非同期開始
	mux.HandleFunc("POST /api/v1/steam/cancel", s.requireAuth(s.handleSteamCancel))
	mux.HandleFunc("GET /api/v1/steam/status", s.requireAuth(s.handleSteamStatus))
	mux.HandleFunc("GET /api/v1/steam/events", s.requireAuth(s.handleSteamEvents)) // SSE（進捗/ログ/結果）

	// 自己更新（docs/design/self-update.md）: チェックは要求時のみ・適用は同期（数秒〜十数秒）・
	// shutdown は graceful 終了（ヘッドレス停止込み）を main 経由で起動する。
	mux.HandleFunc("GET /api/v1/update/check", s.requireAuth(s.handleUpdateCheck))
	mux.HandleFunc("POST /api/v1/update/apply", s.requireAuth(s.handleUpdateApply))
	mux.HandleFunc("POST /api/v1/shutdown", s.requireAuth(s.handleShutdown))

	// ワールドお気に入り（favorites.json・新規セッションの検索→保存／一覧）。
	mux.HandleFunc("GET /api/v1/favorites", s.requireAuth(s.handleFavoritesList))
	mux.HandleFunc("POST /api/v1/favorites", s.requireAuth(s.handleFavoriteAdd))
	mux.HandleFunc("DELETE /api/v1/favorites/{recordId}", s.requireAuth(s.handleFavoriteRemove))

	// フロントエンド（埋め込み静的資産）。テストでは nil 渡しで未登録にできる。
	if s.webFS != nil {
		mux.Handle("/", http.FileServerFS(s.webFS))
	}
	return mux
}

// --- レスポンスヘルパ（統一形式 {ok, data} / {ok:false, error}） ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func writeErr(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": map[string]string{"code": errCode, "message": msg}})
}

// --- ハンドラ ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if locked, remain := s.auth.loginLocked(); locked {
		writeErr(w, http.StatusTooManyRequests, "rate_limited",
			fmt.Sprintf("ログイン試行が多すぎます。約%d秒後に再試行してください", int(remain.Seconds())+1))
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	ok := s.auth.checkPassword(body.Password)
	s.auth.recordLoginResult(ok)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid_password", "パスワードが違います")
		return
	}
	s.setSessionCookie(w, r, s.auth.issueToken())
	writeOK(w, map[string]any{"loggedIn": true})
}

// handleLogout は Cookie をクリアする。stateless 設計のためサーバー側に
// 失効させる状態は無く、このブラウザの Cookie 削除のみ（他端末は失効しない）。
// 全端末の失効が必要なときはパスワード変更で署名鍵を変える。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeOK(w, map[string]any{"loggedIn": false})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeOK(w, s.driver.Status())
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Config string `json:"config"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	name := strings.TrimSpace(body.Config)
	if name == "" {
		// 無config起動はワールドが公開(Anyone)になり危険なため不可。
		writeErr(w, http.StatusBadRequest, "config_required",
			"起動するコンフィグ名を指定してください（無config起動はワールドが公開になるため不可）")
		return
	}
	if err := hlconfig.SanitizeName(name); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_config_name", err.Error())
		return
	}
	headlessPath, launchPath, err := s.resolveLaunch(name)
	if err != nil {
		if errors.Is(err, hlconfig.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "config_not_found", "指定のコンフィグが見つかりません")
			return
		}
		// dataFolder/cacheFolder の作成失敗＝config のパス起因（ユーザーが直せる）→ 409（UI改善⑤）。
		if errors.Is(err, hlconfig.ErrFolderCreate) {
			writeErr(w, http.StatusConflict, "folder_create_failed", err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	// 既定パス（{dataDir}/resonite）にまだ Resonite が無い＝未DL のときは、
	// 実行失敗の素っ気ないメッセージでなく取得導線を案内する（R-A）。
	// この案内はユーザー起動（本ハンドラ）専用。orchestrator/crash 復帰は稼働中前提で
	// 未DL は実質起こらないため、その経路は driver.Start の generic 失敗ログに委ねる。
	if _, statErr := os.Stat(headlessPath); errors.Is(statErr, fs.ErrNotExist) {
		writeErr(w, http.StatusConflict, "headless_not_installed",
			"Resonite がまだダウンロードされていません。設定タブの『今すぐ更新』から取得してください。")
		return
	}
	// 起動時ガード（.NET ランタイム自動設置）: 要求が読めてローカルが不充足なら、受付を返して
	// goroutine で設置→起動する（数十MB の DL を HTTP 応答内で待たせない）。
	// 判定はローカル列挙のみ（ms オーダー・オフライン安全）＝充足・判定不能（runtimeconfig 無し）・
	// システム充足キャッシュ命中は従来どおりの同期起動で挙動不変。
	if s.runtimeGuardNeeded(headlessPath) {
		// 稼働中の start に accepted を返さない（同期経路では driver.Start の ErrAlreadyRunning が
		// 409 を返す。その振る舞いをガード経路でも保つ）。
		if s.driver.Status().State != headless.StateStopped {
			writeErr(w, http.StatusConflict, "start_failed", headless.ErrAlreadyRunning.Error())
			return
		}
		go s.startWithRuntimeGuard(name, headlessPath, launchPath)
		writeOK(w, map[string]any{"accepted": true, "runtimePrepare": true})
		return
	}
	// 依存不足の予防ガイド（R-C 経路③）: 起動は止めず、不足があれば sys ログで
	// 導入コマンドを案内する（UI コンソールにクラッシュログと並んで原因が見える）。
	// 検出はサブプロセス実行を含むため goroutine で行い start をブロックしない
	// （sys ログの位置が起動ログと前後しても実用上問題ない）。
	go s.publishDepGuide()
	if err := s.driver.Start(headlessPath, launchPath, name); err != nil {
		writeErr(w, http.StatusConflict, "start_failed", err.Error())
		return
	}
	s.recordLastUsed(name)
	s.recordLastStart("manual", time.Now().Format(time.RFC3339)) // 最終起動時刻（手動起動・§3.16(9)/R10）
	writeOK(w, map[string]any{"accepted": true})
}

// resolveLaunch は config 名から起動に必要な (headlessPath, launchPath) を解決する。
// 中央 creds は credentials で実行時に書き換わるため cfgMu RLock 下で読み、
// config の creds が空なら中央アカウントを注入した一時 config を ResolveForLaunch で生成する。
// ヘッドレスパスは InstallDirOrDefault（Steam.InstallDir→既定 {dataDir}/resonite）から
// /Headless/<OS バイナリ> を導出し、利用時に "~" を展開する（R-A）。
// handleStart と restart-orchestrator（P8-3）で共用する。name の検証・空判定は呼び出し側の責務。
func (s *Server) resolveLaunch(name string) (headlessPath, launchPath string, err error) {
	s.cfgMu.RLock()
	central := hlconfig.Credentials{
		Username: s.cfg.HeadlessCredentials.Username,
		Password: s.cfg.HeadlessCredentials.Password,
	}
	headlessPath = s.cfg.HeadlessPathOrDefault(s.dataDir, platform.HeadlessBinaryName())
	s.cfgMu.RUnlock()
	headlessPath = platform.ExpandHome(headlessPath)
	// config の dataFolder/cacheFolder（絶対パスのみ）を起動前に作成する（UI改善⑤）。
	// 失敗は起動を止めてエラーを返す（headless 側の分かりにくいクラッシュにしない）。
	// headlessPath は契約どおりエラー時も返す（呼び出し側が導出値を参照できる）。
	if err := hlconfig.EnsureFolders(s.configDir, name); err != nil {
		return headlessPath, "", err
	}
	runDir := filepath.Join(s.dataDir, ".run")
	launchPath, err = hlconfig.ResolveForLaunch(s.configDir, name, central, runDir)
	return headlessPath, launchPath, err
}

// publishDepGuide は不足依存（freetype2）があれば sys ログへ導入ガイドを流す
// （R-C 経路③・handleStart 起点）。コマンドは案内するだけで実行しない
// （sudo を勝手に走らせない）。Windows は checkDeps が常に空＝no-op。
// .NET ランタイムは自動設置（dotnetguard.go）が担うためここでは見ない。
func (s *Server) publishDepGuide() {
	lang := s.langSnapshot()
	for _, issue := range s.checkDeps(runtime.GOOS, runtime.GOARCH) {
		s.driver.PublishSys(i18n.T(lang, "deps.sysGuide", issue.Title(lang), issue.GuideText(lang)))
	}
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if err := s.driver.Stop(); err != nil {
		writeErr(w, http.StatusConflict, "stop_failed", err.Error())
		return
	}
	writeOK(w, map[string]any{"accepted": true})
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	// POST限定。cmd は URL query または JSON body で受理。
	// form-urlencoded body は対応外（必要なら呼び出し側で URL query を使う）。
	cmd := r.URL.Query().Get("cmd")
	if cmd == "" {
		var body struct {
			Cmd string `json:"cmd"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd = body.Cmd
	}
	if cmd == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "cmd がありません")
		return
	}
	if err := s.driver.SendCommand(cmd); err != nil {
		writeErr(w, http.StatusConflict, "command_failed", err.Error())
		return
	}
	writeOK(w, map[string]any{"sent": cmd})
}

// --- 構造化APIハンドラ（Phase 4） ---

// handleSessions: GET /api/v1/sessions → []World （worlds 一覧）
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	worlds, err := s.worlds.List(r.Context())
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, worlds)
}

// handleSessionStatus: GET /api/v1/sessions/{idx}/status → WorldStatus
// 内部で ExecGroup により focus → status を原子的に実行（他リクエストの割込防止）。
func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	idx, err := parseSessionIdx(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var got headless.WorldStatus
	err = s.driver.ExecGroup(r.Context(), func(tx headless.Tx) error {
		if _, e := tx.Exec(fmt.Sprintf("focus %d", idx)); e != nil {
			return e
		}
		lines, e := tx.Exec("status")
		if e != nil {
			return e
		}
		got = headless.ParseStatus(lines)
		return nil
	})
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, got)
}

// handleSessionUsers: GET /api/v1/sessions/{idx}/users → []UserInfo
func (s *Server) handleSessionUsers(w http.ResponseWriter, r *http.Request) {
	idx, err := parseSessionIdx(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var got []headless.UserInfo
	err = s.driver.ExecGroup(r.Context(), func(tx headless.Tx) error {
		if _, e := tx.Exec(fmt.Sprintf("focus %d", idx)); e != nil {
			return e
		}
		lines, e := tx.Exec("users")
		if e != nil {
			return e
		}
		got = headless.ParseUsers(lines)
		return nil
	})
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, got)
}

// sessionDetailResp は status と users を1回の取得で返すための封筒（B1）。
type sessionDetailResp struct {
	Status headless.WorldStatus `json:"status"`
	Users  []headless.UserInfo  `json:"users"`
}

// handleSessionDetail: GET /api/v1/sessions/{idx}/detail → {status, users}
// 1回の ExecGroup(focus → status → users) で取得し、focus 往復を1回に集約する。
// status と users が同一フォーカス時点のスナップショットになる（仕様 §3.4）。
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	idx, err := parseSessionIdx(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var resp sessionDetailResp
	err = s.driver.ExecGroup(r.Context(), func(tx headless.Tx) error {
		if _, e := tx.Exec(fmt.Sprintf("focus %d", idx)); e != nil {
			return e
		}
		statusLines, e := tx.Exec("status")
		if e != nil {
			return e
		}
		resp.Status = headless.ParseStatus(statusLines)
		userLines, e := tx.Exec("users")
		if e != nil {
			return e
		}
		resp.Users = headless.ParseUsers(userLines)
		return nil
	})
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, resp)
}

// handleListBans: GET /api/v1/listbans → []BanEntry （focus 不要・グローバル）
func (s *Server) handleListBans(w http.ResponseWriter, r *http.Request) {
	lines, err := s.driver.Exec(r.Context(), "listbans")
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, headless.ParseListBans(lines))
}

// handleFriendRequests: GET /api/v1/friendrequests → []string（focus 不要・グローバル）
// 注意: v1 互換の単純実装。boot 直後 ambient が多い時はノイズ混入可。
// 詳細: internal/headless/parser.go の ParseFriendRequests godoc 参照。
func (s *Server) handleFriendRequests(w http.ResponseWriter, r *http.Request) {
	lines, err := s.driver.Exec(r.Context(), "friendrequests")
	if err != nil {
		writeExecErr(w, err)
		return
	}
	writeOK(w, headless.ParseFriendRequests(lines))
}

// handleResoniteUserSearch: GET /api/v1/resonite/users?q=<term> → []resonite.User
// Resonite 公開API（api.resonite.com/users）への無認証プロキシ。q が "U-" 始まりなら ID 検索、
// それ以外は名前検索。ヘッドレス稼働は不要（クラウドAPI直叩き）。
func (s *Server) handleResoniteUserSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "missing_query", "検索語 q は必須です")
		return
	}
	users, err := s.resonite.SearchUsers(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "resonite_api_error", err.Error())
		return
	}
	if users == nil {
		users = []resonite.User{} // null ではなく [] を返す
	}
	writeOK(w, users)
}

// handleResoniteWorldSearch: GET /api/v1/resonite/worlds?q=<term> → []resonite.World
// go.resonite.com のワールド検索（HTML スクレイピング）への無認証プロキシ。公式APIにワールド
// 検索が無いため go.resonite.com 依存（HTML 構造変更で壊れ得る）。ヘッドレス稼働は不要。
// 検索失敗（不達・非200）は 502。フロントは getData の null→[] でゼロ件表示に吸収する。
func (s *Server) handleResoniteWorldSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "missing_query", "検索語 q は必須です")
		return
	}
	worlds, err := s.resonite.SearchWorlds(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "resonite_api_error", err.Error())
		return
	}
	if worlds == nil {
		worlds = []resonite.World{} // null ではなく [] を返す
	}
	writeOK(w, worlds)
}

// parseSessionIdx は /api/v1/sessions/{idx}/... のパスパラメータを int に変換する。
func parseSessionIdx(r *http.Request) (int, error) {
	v := r.PathValue("idx")
	idx, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("不正なセッションindex: %q", v)
	}
	if idx < 0 {
		return 0, fmt.Errorf("セッションindexが負: %d", idx)
	}
	return idx, nil
}

// writeExecErr は headless パッケージのセンチネルエラーを HTTP ステータスにマップする。
// 2区分:
//   - ErrNotReady → 409 (UI は「起動してください」を出す)
//   - その他 (Timeout/ProcessGone/Canceled/内部エラー) → 500 (UI は「失敗、再試行」)
//
// クライアント側で原因の細かい区別は不要と判断（複雑化を避ける）。
// 必要なら error.code フィールドで内訳を返すので、UI 詳細表示は可能。
func writeExecErr(w http.ResponseWriter, err error) {
	if errors.Is(err, headless.ErrNotReady) {
		writeErr(w, http.StatusConflict, "not_ready", "ヘッドレスが起動中/停止中で構造化コマンドを受け付けられません")
		return
	}
	// その他は全て 500 + 詳細コードで区別可能に
	code := "exec_failed"
	switch {
	case errors.Is(err, headless.ErrTimeout):
		code = "timeout"
	case errors.Is(err, headless.ErrProcessGone):
		code = "process_gone"
	case errors.Is(err, headless.ErrCanceled):
		code = "canceled"
	}
	writeErr(w, http.StatusInternalServerError, code, err.Error())
}

// --- SSE ---

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	logCh, history := s.driver.SubscribeLog(256)
	defer s.driver.UnsubscribeLog(logCh)
	statusCh, cur := s.driver.SubscribeStatus(16)
	defer s.driver.UnsubscribeStatus(statusCh)

	writeSSE(w, "status", cur)
	for _, l := range history {
		writeSSE(w, "log", l)
	}
	fl.Flush()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n") // keep-alive コメント
			fl.Flush()
		case l, ok := <-logCh:
			if !ok {
				return
			}
			writeSSE(w, "log", l)
			fl.Flush()
		case st, ok := <-statusCh:
			if !ok {
				return
			}
			writeSSE(w, "status", st)
			fl.Flush()
		}
	}
}

func writeSSE(w io.Writer, event string, data any) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}
