# Steam（DepotDownloader）による Resonite 入手・更新 — 詳細設計

> P9-B。全OS（Windows/amd64・Linux/amd64・**Linux/arm64**）共通で Resonite ヘッドレスを
> 入手・更新する仕組みのバックエンド設計。2026-06-05 にユーザーレビューで確定。
> 関連: `DESIGN.md §5.7`、`resonite-domain-facts.md §4`、メモリ `arm-support-plan`。

## 0. スコープ

- **含む**: config スキーマ追加 / DepotDownloader 本体の自動取得（DL＋SHA-256検証＋chmod＋原子的rename）/
  DepotDownloader 実行（stdin 認証・進捗・成否）/ API / SSE 進捗 / 手動更新 /
  **予定再起動への自動更新統合**。
- **含まない（隣接 Phase・別途）**: Phase2 `launcher.go` の ARM 分岐修正 / Phase3 検出ファースト /
  Phase6 CLI セットアップ拡張 / Guard 2FA 入力 UI（将来）/ MRHC 自己更新 Lv1 /
  ネイティブ依存（freetype2 等）の検出・案内（Phase3/6 と onboarding backlog で扱う）。

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
  types.go      … 型・sentinel error
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
UpdateOnScheduledRestart bool `json:"updateOnScheduledRestart"`
```

- `DefaultRestart()` で `UpdateOnScheduledRestart = true`（**新規インストールは ON**）。
  Steam 未設定なら実行時 no-op なので安全。既存保存 config（フィールド欠落）は false＝opt-in。
- 公開 API（`steam/config`）は秘密を返さず `hasPassword`/`hasBranchCode` を返す。
- PW は **ASCII 限定・最大 64 文字**（Steam 仕様）→ PUT で検証。

### 4.1 既定パス導出（R-A）

バンドル既定方針：**パス未指定なら既存 Steam インストールの有無に関わらず `{dataDir}/resonite` に新規 DL** する（自己完結 1 フォルダ・二重管理の衝突回避・DL 前はパスが無い鶏卵問題の解消）。既存インストールを使いたい上級者だけ `Steam.InstallDir`（設定 → Steam）か `ResoniteHeadless`（設定 → アプリ設定）を明示してオプトアウトする。

- `config.InstallDirOrDefault(dataDir)`（純関数）：①明示 `Steam.InstallDir` → ②`ResoniteHeadless` の 2 つ上 → ③既定 `{dataDir}/resonite`。②は `ResoniteHeadless` が正規レイアウト `.../Resonite/Headless/<bin>` 前提。非正規な明示パスでは更新先（DL）と起動元が食い違い得るため、**既存 install の更新先を確実にしたい場合は `Steam.InstallDir` を明示**する。
- `config.HeadlessPathOrDefault(dataDir, binaryName)`：①明示 `ResoniteHeadless` → ②`InstallDirOrDefault/Headless/<binaryName>`。`binaryName` は `platform.HeadlessBinaryName()`（Windows=`Resonite.exe` / 他=`Resonite.dll`）を DI。
- `steamParams` は常に `InstallDirOrDefault` で install 先を埋めるため、**資格（ユーザー名/PW/branchCode）のいずれかが欠けたときだけ** `ErrSteamNotConfigured`（install 先は未設定理由にしない）。
- `resolveLaunch` は `HeadlessPathOrDefault` で解決し、`handleStart` は解決後の実行ファイルが無い（未 DL）なら `headless_not_installed` で「設定 → 今すぐ更新」へ案内する。
- 利用時に `platform.ExpandHome` で先頭 `~` を展開（`filepath.Abs` は `~` 非展開のため。config には入力どおり保存）。
- セットアップウィザードは Resonite パスを尋ねない（未設定＝既定導出）。

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
  - PW: `Enter account password for "<user>": ` → `password+"\n"` 投入
  - 2FA(app/email): **v1 非対応＝明示エラー**（Guard オフ予備アカ前提・将来 UI）
- 進捗パース（DD は `WriteLine` ＝行単位）:
  - `NN.NN% <path>` → percent
  - マイルストーン: `Using app branch` / `Downloading depot` / `Pre-allocating` / `Validating` / `Total downloaded`
- 成否: exit 0=成功 / 非0=失敗（`ContentDownloaderException` 行も拾う）。
- 文字コード: DD 出力は UTF-8 想定（`enc=nil`）。Windows console の UTF-8 確証は Phase8 実機で最終確認。
- ログに PW/code を出さない（コマンド表示時にマスク）。
- 完了後 Resonite install **全体**に `chmod -R +x`（Linux/ARM。DD は実行権を付けない・yt-dlp 等も要）。

## 7. オーケストレーション（manager.go）

`Manager.Update(ctx)` を単一入口にする（HTTP ハンドラと orchestrator が共用・single-flight）。

```
1. 進行中なら拒否（single-flight・mu）
2. Steam 資格(A)・branchCode 未設定 → エラー
3. DD 本体確保（acquire・無ければ DL）
4. DD 実行（入手＝更新は同一・差分再開可）・進捗を /steam/events へ
5. 完了後 Resonite install 全体に chmod -R +x（Linux/ARM）
```

cancel = context＋process kill。差分は `.DepotDownloader/staging/` に残り次回再開。

### 予定再起動への統合（restart-orchestrator）

`doRestart`（終端④ stop→start）の間に更新ステップを挿入。**対象は scheduled トリガーのみ**
（手動再起動・userZero・クラッシュ復帰は素早さ優先で挟まない）。

```
③告知 → ④ shutdown → 完全停止待ち(StateStopped)
        → 【更新】Restart.UpdateOnScheduledRestart=ON && Steam設定済 のとき Manager.Update()
              ・進捗は /steam/events、restart-status の phase = "updating"
              ・「更新確認」= DD 差分実行（走らせること自体が確認＋適用。VDF事前チェックは持たない）
        → 選択 config で Start
```

- **更新が失敗しても必ず Start**（可用性優先。古い版で起動・状態に「最終更新失敗」を記録）。
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
- イベント: `steam-progress`（percent/phase/file）/ `steam-log`（DD stdout/stderr 行）/ `steam-result`（success/fail+message）。
- `pubsub.Hub[T]` を共用。log はリングバッファで再接続時に履歴配信。
- orchestrator 起点の更新も同じ `/steam/events` に流れる。

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

## 13. 未確定・実機（Phase8）で確定する事項

- `ddVersion` の固定値と各 asset の SHA-256（実調査で確定）。
- stall タイムアウト既定 5 分の妥当性（実機の DL 速度で調整）。
- DD プロンプト/成否/進捗の実 stdout 文言（regex 最終確定）。
- Windows console 出力の文字コード（UTF-8 想定の確証）。

## 14. 出典

- Resonite Wiki: [Headless server software/ARM](https://wiki.resonite.com/Headless_server_software/ARM)、
  [Headless Server Software/Setup](https://wiki.resonite.com/Headless_Server_Software/Setup)
  （.NET 10・DepotDownloader・`chmod -R +x`・**freetype2** 依存・SteamCMD on ARM は box64 必要）。
- DepotDownloader 挙動（SteamRE/DepotDownloader ソース実読）: `resonite-domain-facts.md §4`。
