// Package selfupdate は MRHC 自身を GitHub Releases の最新版へ入れ替える自己更新を担う。
// 全OS（Windows/amd64・Linux/amd64・Linux/arm64）共通。設計: docs/design/self-update.md
//
// 入れ替えは「実行中の exe は削除・上書き不可だがリネームは可能」という両OS共通の性質を
// 使った2段 rename で行う（rclone selfupdate 等と同方式）。実行中のプロセスは旧イメージの
// まま動き続け、次回起動から新版が使われる。
package selfupdate

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// repoBase は既定の取得元。releases/latest（タグ解決）と releases/download/<tag>/（資産取得）を
// この配下から組み立てる。テスト・検証では Updater.BaseURL で差し替える（MRHC_UPDATE_BASE は
// 呼び出し側 main が読んで注入する。パッケージ内では環境変数を読まない）。
const repoBase = "https://github.com/MarkN2000/MarkNResoniteHeadlessController"

// ReleasesURL はリリース一覧ページ（リリース未公開エラー時の案内用）。
const ReleasesURL = repoBase + "/releases"

// assets は runtime.GOOS+"/"+runtime.GOARCH をキーにしたリリースアセット名の対応表。
// release.yml の3ターゲットと一致させる。未対応プラットフォームは明示エラーにする
// （存在しない名前で 404 を踏ませない）。
var assets = map[string]string{
	"windows/amd64": "mrhc-windows-amd64.zip",
	"linux/amd64":   "mrhc-linux-amd64.tar.gz",
	"linux/arm64":   "mrhc-linux-arm64.tar.gz",
}

// platformKey は現在の実行プラットフォーム。テストで差し替えられるよう変数にする。
var platformKey = runtime.GOOS + "/" + runtime.GOARCH

// 呼び出し側（CLI/API）が分岐に使う sentinel エラー。表示文言は呼び出し側が
// i18n（CLI=catalog / Web=errCode→locale）で組み立てる。
var (
	// ErrNoRelease はリリースが1つも公開されていない（releases/latest が 404）ことを表す。
	ErrNoRelease = errors.New("リリースが見つかりません（未公開の可能性）")
	// ErrUnsupportedPlatform は現在の OS/arch 用のリリースアセットが無いことを表す。
	ErrUnsupportedPlatform = errors.New("この OS/アーキテクチャ用の配布物がありません")
	// ErrNotReleaseBuild は焼込バージョンが semver でない（dev・ブランチ名等）ため適用不可を表す。
	ErrNotReleaseBuild = errors.New("リリースビルドではないため更新できません")
	// ErrUpToDate は最新リリースが現行版以下（更新不要）であることを表す。
	ErrUpToDate = errors.New("既に最新です")
	// ErrBusy は別の更新が進行中（ロック取得失敗）であることを表す。
	ErrBusy = errors.New("更新が既に進行中です")
)

// Updater は自己更新の実行器。フィールドはテストで差し替えられる。
type Updater struct {
	BaseURL string // 取得元（既定 repoBase）
	Version string // 実行中バイナリの焼込バージョン（main.version）
	ExePath string // 入れ替え対象（既定 os.Executable()）

	// CheckClient はタグ解決用（リダイレクトを追わず Location を読む）。
	// DLClient は資産取得用（アセットは objects.githubusercontent.com へ 302 するため
	// リダイレクト追跡が必須）。役割が逆だと動かないので共有しない。
	CheckClient *http.Client
	DLClient    *http.Client
}

// New は既定設定の Updater を返す。version には main.version を渡す。
func New(version string) (*Updater, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &Updater{
		BaseURL: repoBase,
		Version: version,
		ExePath: exe,
		CheckClient: &http.Client{
			Timeout: 30 * time.Second,
			// リダイレクトを追わない（Location からタグを読むため）
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		// Timeout は body 読み切りまで含む全体上限＝GitHub 側ハングでの無期限停止を防ぐ
		// （main.go の http.Server がタイムアウト未設定のため、ここで必ず打ち切る）。
		DLClient: &http.Client{Timeout: 15 * time.Minute},
	}, nil
}

// assetForPlatform は指定キーのアセット名を返す。未対応なら ErrUnsupportedPlatform。
// エラー合成は steam.assetForPlatform と同形（errors.Join は Error() が改行連結になり、
// 1行=1イベント前提のログ・UI 表示で2行目が孤立するため使わない）。
func assetForPlatform(key string) (string, error) {
	a, ok := assets[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedPlatform, key)
	}
	return a, nil
}

// archiveEntryName はアセット内の実行ファイルのエントリパス（例 "mrhc-linux-amd64/mrhc"）。
// アーカイブはトップレベルフォルダ mrhc-<os>-<arch>/ を持つ（release.yml の規約）。
func archiveEntryName(assetFile, key string) string {
	dir := strings.TrimSuffix(strings.TrimSuffix(assetFile, ".zip"), ".tar.gz")
	name := "mrhc"
	if strings.HasPrefix(key, "windows/") {
		name = "mrhc.exe"
	}
	return dir + "/" + name
}
