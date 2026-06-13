# Steam（DepotDownloader）による Resonite 入手・更新 — 詳細設計

> P9-B。全OS（Windows/amd64・Linux/amd64・**Linux/arm64**）共通で Resonite ヘッドレスを
> 入手・更新する仕組みのバックエンド設計。2026-06-05 にユーザーレビューで確定。
> 関連: `DESIGN.md §5.7`、`resonite-domain-facts.md §4`、メモリ `arm-support-plan`。

## 0. スコープ

- **含む**: config スキーマ追加 / DepotDownloader 本体の自動取得（DL＋SHA-256検証＋chmod＋原子的rename）/
  DepotDownloader 実行（stdin 認証・進捗・成否）/ API / SSE 進捗 / 手動更新 /
  **予定再起動への自動更新統合**。
- **含まない（隣接フェーズ・別ドキュメント）**: launcher の ARM 分岐修正（**R-B・実装済**＝同梱
  dotnet の ELF arch 判定）/ 外部依存（freetype2 / .NET 10）の検出・案内（**R-C・実装済** →
  [`deps-onboarding.md`](./deps-onboarding.md)）/ Guard 2FA 入力 UI（将来）/ MRHC 自己更新 Lv1。

## 1. 用語：2系統のアカウントを混同しない

- **A = DL 用 Steam アカウント**（DepotDownloader が使う）。本設計が扱うのはこちら。
  予備アカウント・**Steam Guard オフを推奨**（v1 は 2FA 入力 UI を作らない）。
- **B = Resonite アカウント**（ヘッドレスの bot 身元＝config の `loginCredential`）。
  既存の `HeadlessCredentials`。本設計とは別物。

## 2. 確定した設計判断（2026-06-05 レビュー）

1. **更新は必ず「停止中」に行う**（統一原則）。
   - 手動更新（`POST /steam/download`）は稼働中なら `409` で拒否（自動停止しない）。
   - 予定再起動では orchestrator が再起動の一部として停止 → 更新 → 起動。
2. **`hub[T]` を `internal/pubsub` へ抽出**（rename のみ）。`headless` と `steam` で共用。
3. **新規パッケージ = `internal/steam`**（API `/api/v1/steam/*`・config `steam`・UI SteamSection と命名統一）。
4. DepotDownloader 本体 = **版固定＋SHA-256 検証**で self-contained 版を自動取得。PW は **stdin**。
   常に `-remember-password`。保存先 `{dataDir}/tools/depotdownloader/{version}/`。
5. 秘密（Steam PW / branchCode）は 0600 で復元可能保存（子プロセスへ渡すため不可避）。
   公開 API は `hasPassword`/`hasBranchCode`（bool）だけ返す（`headless-credentials` 踏襲）。
6. **予定再起動に自動更新を統合（v1 必須）**。`Manager.Update(ctx)` を HTTP ハンドラと
   restart-orchestrator の両方から呼ぶ**単一入口（single-flight）**にする。

## 3. パッケージ構成

```
internal/pubsub/pubsub.go   … 汎用ブロードキャスタ Hub[T]（headless から抽出・公開API化）
internal/steam/
  manager.go    … Manager。操作の入口・進行状態管理（mu保護）・single-flight・orchestrator統合
  acquire.go    … DD本体取得（版固定DL＋SHA＋展開＋chmod＋原子的rename・冪等skip）
  runner.go     … DD子プロセス実行（stdin認証・プロンプト検出・進捗/exit）。headless.Driver下層を踏襲
  progress.go   … 進捗行・プロンプト・マイルストーン検出の純関数（テスト対象）
  dotnet.go     … run() の .NET ランタイム確保ステップ（判定/取得 seam・dotnet-runtime.md）
  types.go      … 型・sentinel error
internal/dotnetruntime/       … .NET ランタイム取得・展開・原子的配置（公式フィード・dotnet-runtime.md）
```

## 4. config 追加

```go
// config.Config に追加
Steam *Steam `json:"steam,omitempty"`

// A = DL 用 Steam アカウント
type Steam struct {
    Username   string `json:"username,omitempty"`
    Password   string `json:"password,omitempty"`   // 復元可能保存（0600・LAN前提）
    BranchCode string `json:"branchCode,omitempty"` // headless branch password（Patreon配布・変動）
    InstallDir string `json:"installDir,omitempty"` // DL先（空=既定 {dataDir}/resonite・R-A）
}

// config.Restart に追加（更新トグルはスケジュールタブで設定する＝Restart配下）
UpdateOnScheduledRestart bool `json:"updateOnScheduledRestart"` // 予定再起動の前に更新
UpdateBeforeManualStart  bool `json:"updateBeforeManualStart"`  // 手動起動（トップバー）・手動「通常再起動」の前に更新
```

- `DefaultRestart()` で両トグルとも `true`（**新規インストールは ON**）。
  Steam 未設定なら実行時 no-op なので安全。既存保存 config（フィールド欠落）は false＝opt-in。
- 公開 API（`steam/config`）は秘密を返さず `hasPassword`/`hasBranchCode` を返す。
- PW は **ASCII 限定・最大 64 文字**（Steam 仕様）→ PUT で検証。

### 4.1 既定パス導出（R-A）

バンドル既定方針：**パス未指定なら既存 Steam インストールの有無に関わらず `{dataDir}/resonite` に新規 DL** する（自己完結 1 フォルダ・二重管理の衝突回避・DL 前はパスが無い鶏卵問題の解消）。既存インストールを使いたい上級者だけ `Steam.InstallDir`（設定 → Steam）を明示してオプトアウトする（Resonite の場所は installDir に一本化）。

- `config.InstallDirOrDefault(dataDir)`（純関数）：①明示 `Steam.InstallDir` → ②既定 `{dataDir}/resonite`。起動元（`HeadlessPathOrDefault`）も DL 先（本関数）もこの値から導出されるため、両者は常に同一フォルダに収束する（矛盾不能）。
- `config.HeadlessPathOrDefault(dataDir, binaryName)`：常に `InstallDirOrDefault/Headless/<binaryName>` を導出（Resonite 正規レイアウト前提）。`binaryName` は `platform.HeadlessBinaryName()`（Windows=`Resonite.exe` / 他=`Resonite.dll`）を DI。
- `steamParams` は常に `InstallDirOrDefault` で install 先を埋めるため、**資格（ユーザー名/PW/branchCode）のいずれかが欠けたときだけ** `ErrSteamNotConfigured`（install 先は未設定理由にしない）。
- `resolveLaunch` は `HeadlessPathOrDefault` で解決し、`handleStart` は解決後の実行ファイルが無い（未 DL）なら `headless_not_installed` で「設定 → 今すぐ更新」へ案内する。
- 利用時に `platform.ExpandHome` で先頭 `~` を展開（`filepath.Abs` は `~` 非展開のため。config には入力どおり保存）。
- セットアップウィザードは Resonite パスの入力を要求しない（CLI改訂 2026-06-06: S5 の
  インストール先プロンプトは任意・空 Enter=既定導出。[cli-onboarding.md](cli-onboarding.md) §3 S5a）。

## 5. DepotDownloader 本体の取得（acquire.go）

- 版固定: `ddVersion` 定数＋asset 名マップ＋**SHA-256 固定値**をコードに焼く。
- `runtime.GOOS/GOARCH` → asset:
  - `windows/amd64` → `DepotDownloader-windows-x64.zip`
  - `linux/amd64`   → `DepotDownloader-linux-x64.zip`
  - `linux/arm64`   → `DepotDownloader-linux-arm64.zip`（self-contained＝.NET 不要）
- 手順: GitHub Releases から tmp に DL → **SHA-256 検証** → zip 展開 → 実行ファイル取り出し →
  `chmod 0755`(Linux) → **原子的 rename** → 確定。
- 保存先: `{dataDir}/tools/depotdownloader/{version}/DepotDownloader[.exe]`（書き込みは data 配下のみ）。
- **冪等**: 既に同 version＋実行可能が在ればスキップ。SHA 不一致 / DL 失敗は tmp 破棄（部分残しなし）。
- ⚠️ `ddVersion` と各 SHA-256 は実調査して確定（preference でなく事実）。

## 6. DepotDownloader 実行（runner.go）

```
DepotDownloader -app 2519830 -branch headless -branchpassword <code> \
  -username <u> -remember-password -dir <install>
```

- **必ず `-remember-password`**（指定漏れの「stored credentials」罠回避）。
- **PW は stdin**（`-password` 引数は ps 露出のため不可。DD は `Console.IsInputRedirected` で `ReadLine`）。
- 子プロセス管理は `headless.Driver` 下層を踏襲（3 パイプ・行単位読み・WaitGroup で drain 後 `Wait`）。
- プロンプト検出（改行なし `Write` ＝末尾断片で出る → 既存 pending-tail と同型）:
  - PW: `Enter account password for "<user>": ` → `password+"\n"` 投入。**送信済みで再要求＝誤資格**とみなし
    即 kill → `ErrAuthFailed`（5分 stall を待たず原因を明示・M1）。
  - 2FA(app/email): **v1 非対応＝明示エラー**（Guard オフ予備アカ前提・将来 UI）
- 進捗パース（DD は `WriteLine` ＝行単位）:
  - `NN.NN% <path>` → percent（進捗行はログに流さない）
  - マイルストーン: `Using app branch` / `Downloading depot` / `Pre-allocating` / `Validating` / `Total downloaded`。
    **マイルストーン行はログに重複させずマイルストーンのみ通知**し、dedup は manager 側で phase 変化時に行う（§9・M4）。
- 成否: exit 0=成功 / 非0=失敗（`ContentDownloaderException` 行も拾う）。
- 文字コード: DD 出力は UTF-8 想定（`enc=nil`）。Windows console の UTF-8 確証は Phase8 実機で最終確認。
- ログに PW/code を出さない（コマンド表示時にマスク）。
- 完了後 Resonite install **全体**に `chmod -R +x`（Linux/ARM。DD は実行権を付けない・yt-dlp 等も要）。

## 7. オーケストレーション（manager.go）

`Manager.Update(ctx)` を単一入口にする（HTTP ハンドラと orchestrator が共用・single-flight）。
第3の呼び出し元としてセットアップウィザード S5b（CLI改訂 2026-06-06・`realSteamUpdate` が
独自に `NewManager` を生成）があるが、サーバー未起動段階のため single-flight の共有は不要
（衝突は構造的にゼロ・[cli-onboarding.md](cli-onboarding.md) §2）。

```
1. 進行中なら拒否（single-flight・mu）
2. Steam 資格(A)・branchCode 未設定 → エラー（install 先は常に導出されるため資格欠如のみが未設定・R-A）
3. DD 本体確保（acquire・無ければ DL）
4. DD 実行（入手＝更新は同一・差分再開可）・進捗を /steam/events へ
5. **DL後検証（H2）**: `UpdateParams.VerifyRelPath`（=`Headless/<OSバイナリ>`・OS 名は server が DI）が
   `InstallDir` 配下に在るか stat。無ければ失敗にする。ブランチコード誤りで headless depot が public
   フォールバックすると exit 0 でも headless 実体が落ちないため、**ファイル存在で判定**する（良性の
   `Error: Password was invalid for branch headless` は成功時にも出るので文字列マッチは使わない）。
6. 完了後 Resonite install 全体に chmod -R +x（Linux/ARM）
7. **.NET ランタイム確保**（steam/dotnet.go・2026-06-07 追加）: 要求（`Headless/Resonite.runtimeconfig.json`）
   を満たすランタイムがローカル（`<install>/dotnet-runtime`）にもシステムにも無ければ、
   internal/dotnetruntime が公式フィードから取得して設置する。失敗は更新失敗
   （`dotnet_install_failed`）。確定仕様: [dotnet-runtime.md](dotnet-runtime.md)
```

cancel = context＋process kill。差分は `.DepotDownloader/staging/` に残り次回再開。
ランタイム単独設置の入口 `Manager.InstallRuntime`（server の起動時ガード用・Steam 資格不要）も
同じ single-flight スロット・SSE・Status を使う。`Status`/result に `RunKind: "update"|"runtime"` を
持ち、表示層が「更新」/「ランタイム設置」を出し分ける。

### 起動/再起動への統合（共通フック `maybeUpdate`）

更新の判定・実行は **`maybeUpdate(ctx, triggerType, onUpdating)`** に一本化し、トリガー別トグルで
ゲーティングする（`scheduled`→`UpdateOnScheduledRestart` / `manual`→`UpdateBeforeManualStart` / 他→no-op）。
「更新確認」= DD 差分実行（走らせること自体が確認＋適用。VDF事前チェックは持たない）。クラッシュ復帰・
通常停止は対象外（素早さ優先・前者は `driver.Start` 直叩きで本フックを通らない）。

**① 予定再起動・手動「通常再起動」**（orchestrator 経由）: `doRestart`（終端④ stop→start）の間に
`beforeFlowStart`→`maybeUpdate` を挿入。進捗は /steam/events、restart-status の phase = "updating"。

```
③告知 → ④ shutdown → 完全停止待ち(StateStopped)
        → 【更新】maybeUpdate(triggerType)＝対応トグル ON && Steam設定済 のとき updateResonite()
        → 選択 config で Start
```

**② 手動コールド起動**（トップバー「起動」・`handleStart`）: 停止状態から `UpdateBeforeManualStart` ON
&& Steam設定済 のとき、`{accepted, updating:true}` を返して goroutine `startWithUpdate` ＝
`maybeUpdate("manual")` → 既存 `startWithRuntimeGuard`（.NETガード→起動→記録）を再利用。進捗は
/steam/events（設定タブ）。更新進行中の起動押下は `409 update_in_progress`。更新をユーザーが
**キャンセルしたときだけ起動を見送る**（明確な中止意思を尊重）。

- **更新が失敗しても必ず Start**（可用性優先。古い版で起動）。再起動経路はキャンセルでも Start 継続
  （④到達後）／コールド起動のみキャンセルで Start 見送り。
- **打ち切り = 進捗停滞で中断**（既定 5 分進捗なし → ハングとみなし中断 → Start）。遅いが進捗している
  DL は誤って殺さない。総時間上限ではなく stall（無進捗）タイムアウト。
- 更新 ctx は stall タイムアウト付き＋親 bg ctx 連動（MRHC 終了で中断 / ④はユーザー cancel 不可）。
- 新 phase `phaseUpdating` を追加。

## 8. API（全 `requireAuth`・統一 `{ok,data}`）

| メソッド/パス | 役割 |
|---|---|
| `GET /api/v1/steam/config` | `{username, installDir, hasPassword, hasBranchCode}`（秘密は出さない） |
| `PUT /api/v1/steam/config` | 資格・branchCode・installDir 更新（PW は ASCII/64 検証） |
| `POST /api/v1/steam/download` | 入手/更新を非同期開始（停止中のみ・稼働中は 409） |
| `POST /api/v1/steam/cancel` | 進行中 DL の中止 |
| `GET /api/v1/steam/status` | `{state, percent, phase, currentFile, lastResult, ...}` |
| `GET /api/v1/steam/events` | SSE（`steam-progress`/`steam-log`/`steam-result`・headless `/events` と分離） |

## 9. SSE 設計

- 専用 `/steam/events`（headless `/events` と分離＝関心の分離）。
- イベント: `steam-progress`（percent/file）/ `steam-log`（DD stdout/stderr 行・MRHC 生成行は
  `msgKey/msgArgs` 付き）/ `steam-milestone`（phase）/ `steam-result`（失敗時 text+`code/detail`・§9.1）。
- `pubsub.Hub[T]` を共用（**非ブロッキング＝満杯購読者にはドロップ**＝遅い1購読者が全体を詰まらせない）。
  log/milestone はリングバッファ(500)で再接続時に履歴配信。**履歴は run 開始（begin）でクリア**＝
  リプレイは常に現 run のみ（UI 側も接続毎の `steam-status` 受信でログ表示をリセットし、
  自動再接続でも二重表示にならない）。
- **マイルストーンは phase 変化時のみ publish/log（M4）**＝`Pre-allocating` が数百ファイル分続く洪水と、
  リングからの早期マイルストーン（`Using app branch` 等）の追い出しを防ぐ。`lastEvent` は dedup 前に更新＝
  stall ウォッチドッグは活動継続とみなす。
- **終端 `steam-result` はドロップし得る**（非ブロッキング）。UI は更新中だけ `GET /steam/status` を
  補助ポーリングし、終端(success/failed)の取りこぼしによる「更新中」固着を防ぐ（H1）。
- orchestrator 起点の更新も同じ `/steam/events` に流れる。

## 9.1 エラーコードと表示ポリシー（2026-06-07）

**分類は Go の sentinel＋code、表示文言は各表示層**に分離する（`headless_not_installed` と同じ流儀）。
Go エラーの `.Error()`（ja）は変えない＝サーバーコンソールログと未知コードのフォールバック原文を兼ねる。
`finish()` が `errorCode()/errorDetail()` で `Status.errorCode/errorDetail` と `steam-result` の
`code/detail` に焼き込み、Web UI（SteamSection）が既知 code を locale 変換（ja/en）・未知/無 code は
原文（lastError）→ 汎用文言へフォールバックする。CLI ウィザード S5b は sentinel を `errors.Is` で分岐。

| code | sentinel | 発生 | 表示 |
|---|---|---|---|
| `cancelled` | ErrCancelled | ユーザー中止/shutdown（**acquire 段階の ctx 中断もここへ正規化**） | 中立表示（赤い「失敗」にしない） |
| `auth_failed` | ErrAuthFailed | PW 再要求＝誤資格（M1） | 自己完結の locale 文言 |
| `two_factor_required` | ErrTwoFactorRequired | Steam Guard オン | 同上 |
| `stalled` | ErrStalled | stallTimeout 無進捗（正常終了との競合時＝runErr nil は成功扱い） | 同上 |
| `verify_missing` | ErrVerifyMissing | exit 0 でも headless 実体なし＝branch コード誤り（H2） | 同上 |
| `acquire_failed` | ErrAcquireFailed | DD 本体取得の失敗 | 見出し locale＋`detail` 併記 |
| `dd_failed` | ErrDDFailed / ErrDDStartFailed | DD 異常終了 / プロセス起動失敗 | 同上 |
| `chmod_failed` | ErrChmodFailed | 実行権付与の失敗（非 Windows） | 同上 |
| `dotnet_install_failed` | ErrDotnetInstallFailed | .NET ランタイム設置の失敗（[dotnet-runtime.md](dotnet-runtime.md)） | 同上 |

- **detail** ＝「`<sentinel>: <内側>`」型エラーの内側原文（HTTP 404・exit status 等の機械情報）。
  ja は従来表示がそのまま再構成され、en は見出しだけ英語＋機械情報が読める。
- sentinel の文言は**プレフィックス部のみ**とし、動的詳細は発生箇所で `%w` ラップして付加する規約
  （`"%w: %w"` で内側のエラーチェーンも保持・go 1.20+）。
- MRHC 生成のログ4行（DD 取得開始/完了・SHA OK・chmod 中）は `Event.msgKey/msgArgs`（名前付き引数）で
  locale 変換。`Text`=ja 原文を併存（未知キーのフォールバック）。DD の生 stdout/stderr は原文（英語）のまま。
- HTTP 層 code（`writeSteamErr`/`handleSteamConfigPut`）: `headless_running` / `steam_not_configured` /
  `update_in_progress` / `no_update` / `steam_password_invalid` をフロント（notify.ts）が locale 変換。
- **意識的スコープ外**: `ErrUnsupportedPlatform` は acquire_failed に縮退（配布は対象3プラットフォームのみで
  実害僅少）・`save_failed`/汎用 `bad_request` はアプリ全体の流儀どおり原文フォールバック・サーバー
  コンソールログ（log.Printf）は ja 固定・CLI ウィザード default 分岐（`wizard.dl.failed: %v`）は Go 原文が
  ja のため EN CLI で和文混じりが残る（Go エラー文言を ja 維持する以上の既知制限）。

## 10. セキュリティ

- Steam PW/branchCode は 0600 で復元可能保存（DESIGN §秘密情報・LAN 前提）。
- PW は ASCII 限定・最大 64（Steam 仕様）→ PUT 検証。
- ログに PW/code を出さない（マスク）。
- `account.config`（remember-password トークン）は **.NET IsolatedStorage**（MRHC 管理外）。
  「同一 OS ユーザー・同一 DD バイナリで実行する限り有効」前提（消えても再ログインで復帰）。
  **保存先は OS ユーザー単位で dataDir の外**＝MRHC の dataDir を削除してもこのトークンは残る
  （共有マシン/アンインストール時のクリーンアップでは別途消去が要る・M-x64 で確認）。

## 11. テスト戦略

- `progress.go` 純関数: 進捗/プロンプト/マイルストーン検出の単体（fixture は Phase8 実機 stdout で確定するまで暫定）。
- `acquire`: SHA 検証・原子的 rename・冪等スキップ（ローカル HTTP テストサーバ＋ダミー zip）。
- `runner`: **偽 DepotDownloader**（`poc/fakehl` 同様の偽プロセス）で stdin PW 投入・進捗・exit を e2e。
- `manager`: 停止→更新→起動の順序・cancel・single-flight・orchestrator 統合（scheduled のみ・失敗でも Start・stall 中断）。
- 実機（Phase8）: プロンプト文言・成否出力・文字コード・`account.config` 実保存先。

## 12. 実装順

1. **土台**: `pubsub` 抽出 ＋ `config.Steam` ＋ `Restart.UpdateOnScheduledRestart`（小）
2. `acquire.go` ＋単体 test
3. `runner.go` ＋偽プロセス e2e
4. `manager.go`（進行管理・single-flight・cancel）＋ orchestrator 統合（scheduled 更新）
5. API ＋ SSE 結線
6. UI `SteamSection` 実装（別途）

## 13. 実機で確定する事項（M-x64 で x64 確定済 / ARM は R-F）

**✅ M-x64（2026-06-05・実 Steam で本物 DL を Windows x64 で1回）で確定:**
- `ddVersion`=3.4.0・各 asset SHA-256（windows/linux x64＋**arm64** すべて実 GitHub と一致）。
- DD プロンプト/進捗/マイルストーン/成否の実 stdout（x64 で regex 妥当を確認）。**`Error: Password was
  invalid for branch headless` は成功時にも出る良性ノイズ**（headless ブランチを持たない共有再頒布アプリ
  向け）＝文字列マッチ禁止・exit コードで成否判定。Resonite 本体(2519830)/headless depot(2519832)は private
  beta で成功。
- Windows での実行・進捗パース動作（出力は UTF-8 想定で問題なし）。stall 5 分は妥当（x64 は約2分で完走）。

**未確定（R-F・ARM 実機）:**
- ARM で DD プロンプト/成否文言が x64 と同一か・`account.config` 実保存先（IsolatedStorage）。
- ARM での `<dataDir>/resonite` への DL→system .NET10 起動→freetype2 込みの e2e。

## 14. 出典

- Resonite Wiki: [Headless server software/ARM](https://wiki.resonite.com/Headless_server_software/ARM)、
  [Headless Server Software/Setup](https://wiki.resonite.com/Headless_Server_Software/Setup)
  （.NET 10・DepotDownloader・`chmod -R +x`・**freetype2** 依存・SteamCMD on ARM は box64 必要）。
- DepotDownloader 挙動（SteamRE/DepotDownloader ソース実読）: `resonite-domain-facts.md §4`。
