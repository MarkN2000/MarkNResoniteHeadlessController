# MRHC 作り直し 設計ドキュメント

Resonite Headless Controller を、要件からゼロ再定義して作り直すための設計書。
関連: コマンド/出力書式などの外部事実は [`resonite-domain-facts.md`](./resonite-domain-facts.md) を参照。

> ステータス: 章ごとレビュー反映済（実装着手前）。パーサの出力書式・Linux起動方法など「要検証」項目は実機採取で確定する。

---

## 1. 目的・利用者・運用形態

- **目的**: Resoniteヘッドレスサーバーを、LAN内のブラウザ／スクリプトから操作・管理する。
- **利用者**: Resoniteコミュニティの知人数名。各自が自宅サーバーに導入する（＝**配布とセットアップの容易さが最重要**）。
- **運用形態**: **24時間常時稼働**。Windows / Linux（**ARM Linux 含む**）を**対等にサポート**。MRHC自体は**手動起動**（PC起動時の自動起動は対象外）。
- **設計原則**: SOLID、関数型寄りで副作用を局所化、高凝集・低結合。「Resonite/外部ツールの事実」と「OS差」を専用層に閉じ込める。

---

## 2. 機能スコープ（段階出荷）

完成させ切ることを最優先し、**歩く骨格(v1.0)を先に出荷**してから肉付けする。

### v1.0 — 歩く骨格（端から端まで動く最小スライス）
全アーキテクチャ（プロセスI/O・SSE・認証・単一バイナリ・両OS）を実証する。
1. CLIセットアップウィザード（管理パスワード等の最小設定）
2. 既存configでヘッドレスを起動 / 停止
3. ログのライブ表示（SSE）
4. 任意のコンソールコマンド送信＋出力表示
5. パスワードログイン
6. 単一バイナリ（Win/Linux）

### v1.x — 肉付け（この順）
1. 構造化ダッシュボード（`worlds` / `status` / `users` のパース表示・フォーカス切替）
2. ユーザー操作（kick / ban / silence / unsilence / respawn / role / invite）
3. configエディタ（スキーマ駆動）
4. 自動再起動（scheduled / 手動通常）＋**ヘッドレスのクラッシュ自動復帰**（全員退出待ちは安全再起動フローの待機段に内包＝userZero 独立トリガーは廃止・§3.16）
5. フレンド管理 / BAN一覧
6. Steam更新（実行＋ログのみ）

### スコープ表（MoSCoW）
| 優先 | 内容 |
|---|---|
| Must | 起動/停止/再起動・コマンド送受信・ログ(SSE)・状態構造化・ユーザー操作・configエディタ・認証・単一バイナリ・CLIセットアップ |
| Should | scheduled/userZero/手動 再起動（事前アクション=chatWarning）・**ヘッドレスのクラッシュ自動復帰**・フレンド/BAN一覧・CPU/メモリ表示(監視)・**ワールド検索(go.resonite.com スクレイピング・2026-05-31 §Won't から格上げ・2026-06-04 実装＝新規セッションタブで検索→起動)** |
| Could | Steam更新(都度実行のみ・GuardはUI入力。§5.7)・外部連携の高度化・事前アクション拡張(itemSpawn等)・**MRHC自己更新=Lv1(バージョン表示＋GitHub更新通知＋DLリンク・差替は手動)** |
| Won't (v1) | highLoad自動再起動・Steamのbuildid事前チェック・設定ファイル二重管理・PC起動時の自動起動(systemd/サービス) |

---

## 3. 非機能要件

- **配布**: ランタイム不要の単一実行ファイル（`mrhc` / `mrhc.exe`）。フロント静的資産は `embed.FS` で同梱。インストール=アーカイブをDL→展開→実行（詳細は §8）。
- **クロスプラットフォーム**: **Windows (x64) / Linux (x64・ARM64)** を対等にサポート。ARM Linux は Resonite が ARM で動作する環境（SBC・ARMサーバー等）向け。MRHC本体は **CGO 不要の純 Go** のため `GOARCH=arm64` のクロスビルドで対応。⚠️ ただしARMは「ビルドだけ」では完結しない: **(1) Resonite同梱dotnetはx64でARMでは使えず、システムに.NET 10ランタイムが別途必要** **(2) SteamCMDがARM非対応のため入手/更新はDepotDownloaderに統一**（§5.7）。OS/arch差（パス・文字コード・プロセス起動・dotnet所在・Resonite入手手段）は**プラットフォーム抽象層**に隔離。起動は Win=`Resonite.exe` / Linux=`dotnet Resonite.dll`（同一.NETアプリ）。
- **前提ランタイム**: MRHC自体は単一Goバイナリで**ランタイム不要**。ヘッドレスは.NET 10で動く。**x86(Win/Linux)はResoniteが自前のランタイムを同梱**（`<install>/dotnet-runtime/dotnet`、実機確認済）するため**別途の.NET導入は不要**。⚠️ **ARM64は同梱dotnetがx64で使えないため、システムに.NET 10ランタイムが別途必要**（MRHCは同梱dotnetの実体アーキを確認し、不一致なら`~/.dotnet`→PATHの`dotnet`にフォールバック）。Linux起動は使用可能なdotnetで `dotnet Resonite.dll`。
- **セキュリティ（LAN前提で右サイズ）**: §7参照。
- **時刻（タイムゾーン）**: スケジュール再起動の毎日/毎週/日時は**サーバーのローカル時刻**で判定する。UIには適用タイムゾーンを明示する（「何時に再起動されるか」の事故防止）。
- **秘密情報の扱い**: 管理パスワードは**ハッシュ保存**。一方、Resoniteアカウント/Steamのパスワードは子プロセス（ヘッドレス・DepotDownloader）へ渡すため**復元可能な形で保存せざるを得ない**。よって防御は「**設定ファイルのパーミッション制限（Linuxは0600相当）＋LAN前提**」とする。
- **i18n**: 日本語 / 英語。メッセージカタログ外部化で言語追加容易（react-i18next）。
- **レスポンシブ**: モバイルファースト（Tailwindブレークポイント）。スマホからの簡易操作を想定。
- **デザイン**: Mantineのテーマシステムで配色を一元管理（light/dark・レスポンシブ部品）。管理ダッシュボード向けに表/フォーム/モーダル等が揃う。
- **テスト**: パーサは実出力フィクスチャで単体テスト（現行はテスト0件）。

---

## 4. アーキテクチャ全体

### スタック
- **バックエンド**: Go（単一静的バイナリ・両OSクロスビルド・goroutine/channel・gopsutil(メトリクス)・x/text(文字コード)）
- **フロントエンド**: React + Vite + TypeScript + **Mantine**（コンポーネント一式・テーマ/ダークモード）+ react-i18next（SPA、静的ビルド）
- **配布**: フロントの静的成果物を Go の `embed.FS` でバイナリに同梱 → **同一オリジン配信**
- **API契約**: Goの型から TS型を生成し単一ソース共有（**Go型を正に軽量生成（tygo系）が候補**。ツールは実装時に確定）

### レイヤー構成（バックエンド）
```
HTTP / SSE 層         … ルーティング・認証・(必要なら)軽い制限。Web UIもここの一クライアント
  ↓
アプリ層 (usecases)   … 「起動する」「再起動する」「configを保存する」等のユースケース
  ↓
ドメイン層
  ├─ Console Driver        … stdin送信(エンコード)・stdout収集(リングバッファ)・コマンドキュー＋応答相関・原子的コマンドグループ
  ├─ WorldsService         … worlds一覧 と「全ワールド巡回(ForEach)」の共通機構
  ├─ Output Parser         … worlds/status/users/listbans/friendRequests を構造化（★Resonite事実を集約。friendRequestsはv1互換のbest-effort: docs/resonite-domain-facts.md §6 参照）
  ├─ Restart Orchestrator  … 手動/scheduled → 単一の「安全に再起動」動作（全員退出待ちは待機段に内包＝userZero 独立トリガー廃止・§3.16）
  ├─ PreRestartAction(plugin) … chatWarning 等。レジストリで拡張
  └─ Process Lifecycle Monitor … ヘッドレスの異常終了を検知→状態反映＋(設定ONで)自動再起動(クラッシュループ保護)
  ↓
プラットフォーム抽象層 (★OS差を隔離)
  ├─ ProcessLauncher  … Win: `Resonite.exe -HeadlessConfig <f>` / Linux: dotnet経由で `Resonite.dll -HeadlessConfig <f>`。dotnetは同梱(`<install>/dotnet-runtime/dotnet`)のELF archが実行archと一致する時のみ採用し、不一致(ARM上のx64同梱等)なら `~/.dotnet`→PATH のシステムdotnetへフォールバック(R-B)。cwd=`Headless/`。両者とも同一の.NETアプリ
  ├─ Encoding         … Win: ロケール(Shift_JIS等) / Linux: UTF-8
  ├─ Paths            … Resoniteの場所は `steam.installDir` に一本化(既定 `{dataDir}/resonite`・R-A)。`~`展開・OS別バイナリ名(Resonite.exe/.dll)
  ├─ Deps             … 外部依存の検出+案内(Linux=freetype2 / ARM=.NET10。3値判定でabsent確定時のみ案内・R-C。詳細 design/deps-onboarding.md)
  └─ Steam(DepotDownloader) … Resoniteの入手・更新(全OS統一・ARM対応)。steamcmdは廃止。詳細は§5.7
```

### 並行モデル（Goの要点）
- プロセスハンドル・ログリングバッファ・再起動状態は**それぞれ単一のgoroutineが所有**し、他からは**channel経由のメッセージ**でのみ操作（"share memory by communicating"）。旧コードの3タイマー競合を構造的に排除。
- 背景goroutine: stdout読取 / SSEブロードキャスト / **scheduler（予定発火）・crash-monitor（プロセス死活監視→自動復帰）**（§5.4–5.6）。（userZero 常時監視・メトリクス収集は不採用/将来）
- **状態の永続化**: 最終起動などを、ツール再起動後も保つ小コンポーネント（JSON状態ファイル）。
- **実装メモ（§3.16・Phase 8）**: 再起動状態は上記「channel 所有」案ではなく **案A（mutex + per-flow goroutine + context cancel）** で実装（flow が最大60分のブロッキング I/O を抱え、channel 所有だと worker 分離が結局必要で複雑化するため）。実コマンド直列化は driver の execMu。永続化は **最終起動（lastStartAt/Trigger・手動起動も含む全起動）のみ** `runtime-state.json` に追記（次回予定・稼働時間は導出）。

---

## 5. ドメインモデル

> 構造化実行（コマンド/応答）・WorldsService の**詳細設計**: [docs/design/structured-driver.md](design/structured-driver.md)

### 5.1 Console Driver（プロセスI/O）
- 起動: `ProcessLauncher.Launch(configPath)`（OS差を抽象化）。
- **コマンドは直列化**: Resoniteのstdin/stdoutは1本なので、コマンドをキューに積み**1件ずつ実行**。同時実行しない。
- **原子的コマンドグループ**: `focus` は**サーバー全体で1つの共有状態**。「focus → 処理」のような複数手順は、**その間に他コマンドを割り込ませない"グループ"単位**で実行する（割り込みでfocusが書き換わり別ワールドに誤適用される事故を防ぐ）。
- **応答相関**: コマンド送信後の出力を、(a)タイムアウト、(b)プロンプト検出（末尾`>`の行）、(c)データ後プロンプト検出 のいずれかで「このコマンドの応答」として束ねる。一致後 settle 時間待って確定。
- **非同期ログとの分離**: Resoniteは応答とは無関係な**ログ（入退室・エラー等）を常時垂れ流す**。応答相関は、コマンド窓に紛れ込む無関係行を**許容/無視**できること（パーサは該当行のみマッチ）。
- 文字コード: 送信は `Encoding.Encode`（OS/ロケール依存）、受信は復号フォールバック。
- ログ: 固定長リングバッファ。新規行は SSE で push。
- 停止: `shutdown` 送信 → 猶予 → SIGTERM → SIGKILL。

### 5.2 WorldsService（全ワールド巡回の共通機構）
- `List()`: `worlds` を1回実行し、全ワールドの人数(Users/Present)・アクセスレベル等を構造化して返す。
- `ForEach(fn)`: 稼働中の各ワールドを**focusしてから `fn` を実行**する処理を、**原子的コマンドグループ内**で行う共通部品。事前アクション・セッション変更・各ワールドのユーザー一覧取得などが共用する（DRY＋focus競合防止）。

### 5.3 Output Parser
- `worlds` / `status` / `users` / `listbans` / `friendRequests` を構造体へ。**正規表現と出力書式は [`resonite-domain-facts.md`](./resonite-domain-facts.md) を正とする**（出力書式は実機採取で確定）。
- `friendRequests` は **v1 互換の単純実装**（split + trim、プロンプトのみ行は保険でスキップ）。prompt-glue は Driver の `stripExactPrompt` が事前除去。実機 pending entry の書式は未採取だが、v1 production で動いていた手法を踏襲。boot 直後等の ambient 多発時はノイズ混入の可能性あり（v1 と同じ受容、godoc に明記）。
- 既知の脆さ（空白入りユーザー名等）は採取したフィクスチャで検証しながら堅牢化。実機検証済：[`scripts/empirical-capture/fixtures/2026-05-28-lan-login/`](../scripts/empirical-capture/fixtures/2026-05-28-lan-login/)。

### 5.4 Restart Orchestrator
- **トリガー**: 手動 / scheduled（毎日・毎週・日時、サーバーローカル時刻） / userZero（全員退出）。highLoadは不採用。scheduledは時刻判定（cronライブラリ推奨、旧来の1分ポーリング廃止）。
- **userZero検知**: `WorldsService.List()`（＝`worlds`一発）で**全ワールドの在席者(Present)合計**を見て0かを判定（R0・`presentUserCount`）。ホストは `Present:False` のため自然に除外される。**1ワールドずつfocusする必要はない**。起動直後は除外（`minUptime`ゲート）。
- **単一の「安全に再起動」動作**（マルチワールド前提）:
  1. **全ワールドに対し**事前アクション実行（§5.5、`WorldsService.ForEach`）
  2. **全ワールドの在席者(Present)が0になるまで**待機（**締切付き**＝2区間モデル R9: `quietWaitMin`＋`announceWaitMin` の和を超過で強制実行）
  3. 停止 →（任意：Steam更新）→ 起動
- userZeroは「既に空」なので待機不要＝即実行。
- **二重起動防止**: 再起動中フラグで、同時/連打のトリガーを排他。
- **実装（確定仕様=phase-7-spec §3.16・2026-06-01 協議で確定差分）**: 本節は当初設計。実装では **userZero を独立トリガーにしない**＝「全員退出を待つ」は再起動フローの待機段②に内包（userZeroWatcher 常時監視は廃止）。**cron ライブラリは使わず**独自 once/weekly/daily を scheduler goroutine が自前算出（config 変更は Reload シグナルで再計算）。①セッション変更はトリガー時に即発火（新規参加を静かに止める）・③告知だけ締切前。並行モデルは**案A（mutex+goroutine+context cancel）**＝長時間ブロッキング flow のため channel 所有モデルは見送り。

### 5.5 PreRestartAction（プラグイン）
- 共通IF（OCP）。依存は `RestartContext` 経由でDI（具体プロセスに非依存）。アクションは原則**全ワールドに対して**適用（`WorldsService.ForEach`）。
```go
type RestartContext interface {
    SendCommand(cmd string) error      // 現在focus中のワールドへ
    MinutesUntilRestart() int
    World() WorldInfo                  // 現在処理中のワールド
    Users() []UserInfo                 // そのワールドの参加者
}
type PreRestartAction interface {
    Type() string
    Meta() ActionMeta                  // UI生成用(ラベル/説明/paramスキーマ)
    Validate(raw json.RawMessage) (Params, error)
    Execute(p Params, ctx RestartContext) error
}
```
- レジストリ `map[string]PreRestartAction`。設定は配列 `[{type,enabled,params}]`（均一形・前方互換・未知typeは無視）。
- **v1はchatWarningのみ**。`itemSpawn`等はレジストリに1件追加するだけで拡張。
- ⚠️ **chatWarningの到達範囲**: Resoniteに全体送信コマンドは無く、`message <friend> <msg>` は**フレンド宛DMのみ**＝非フレンドには届かない。現実解は「`users`列挙→各人へDM(フレンドのみ着)」または「`dynamicImpulseString`でワールド側の通知機構を起動(ワールド対応が前提)」。**v1はこの制約を明示**したうえでDM方式とし、ワールド内通知はitemSpawn/dynamicImpulse拡張で対応。
- **実装（§3.16）**: 上記プラグインIFは実装せず、事前アクションは **`PreActions{announce, sessionChanges}` の固定2種**に簡素化（YAGNI）。`announce`=**dynamicImpulse 告知（フル設定型・spawn→約10秒→impulse の2パス）**、`sessionChanges`=Private化/maxusers=1/改名（各独立トグル）。**chatWarning（DM方式）は到達不確実のため不採用**。新アクション追加時にプラグイン化を再検討。

### 5.6 Process Lifecycle Monitor（クラッシュ自動復帰）
- ヘッドレスの**プロセス終了を監視**。こちらの**意図的な停止**（stop/restart）以外の終了＝異常終了として、
  - (必須) 状態へ反映（ダッシュボードに「異常終了」表示）
  - (任意・設定ON時) **ヘッドレスを自動再起動**。**クラッシュループ保護**（短時間にN回以上落ちる場合は自動復帰を止めて通知）。
- 復帰対象は**ヘッドレス**であり、MRHC自身ではない（MRHCは手動起動）。
- **実装（§3.16(4)・P8-4b）**: Driver に **`SetOnUnexpectedExit` コールバック**を追加し、`waitExit` の `d.stopping==false`（管理外の異常終了）時のみ非ブロッキング通知（誤検知ゼロ）。crash-monitor が **rolling window でループ保護**（既定「10分に3回」で停止・通知）し直近 config で復帰。**再起動進行中（orchestrator 所有）なら skip**（二重起動防止）。

### 5.7 Resoniteの入手・更新（DepotDownloaderに統一・Could/後段機能）
> ⚠️ **方針転換（2026-05-29）**: 旧計画のsteamcmdは**ARM Linux非対応**のため廃止。**全OSでDepotDownloader（.NET製・ARM64ネイティブ）に統一**。詳細は[`resonite-domain-facts.md`](./resonite-domain-facts.md)§4 と メモリ`arm-support-plan`。
- Resoniteヘッドレスは**Steam(headlessブランチ)経由でのみ**入手・更新可能。手段は **DepotDownloader**（self-contained配布があり.NET不要。MRHCが実行OS/archに合う版を自動DL＋SHA検証）。
- **入手＝更新は同一コマンドで冪等（差分DLで再開可）**: `DepotDownloader -app 2519830 -branch headless -branchpassword <code> -username <u> -remember-password -dir <install>`。**更新有無の事前チェック(VDF/buildid)は持たない**（旧来の複雑さを排除）。出力はSSEでライブ表示。実行前に(稼働中なら)ヘッドレス停止→完了後に再起動（§7の非同期）。
- ⚠️ **取得後 `chmod -R +x <install>` が必須**（DepotDownloaderは実行権を付けない）。
- **外部依存の検出＋案内（R-C・実装済）**: Linux=freetype2（全distro）/ ARM=.NET 10 を検出し、不足が確定したときだけ導入コマンドを案内（[Y/n] 実行は初回ウィザード末尾のみ・通常起動とWeb UI起動時はログ案内）。詳細 [`design/deps-onboarding.md`](./design/deps-onboarding.md)。
- **2系統のアカウントを分離**: A=DL用Steamアカウント（DepotDownloaderが使う） / B=Resoniteアカウント（configの`loginCredential`＝bot身元。別物）。本節はA。
- **A(Steam)のv1方針＝予備アカウント・Steam Guardオフを推奨**: user+password(stdin)+betaコードで**完全無人DL**。パスワードは**stdin投入**（`-password`引数はps露出のため不可）。`-remember-password`でトークン保持（保存先は.NET IsolatedStorage＝MRHC管理外）。
- **Guardオン時の2FA入力UI（プロンプト検出→`POST /steam/guard-code`→stdin投入）は将来拡張**（DepotDownloaderの2FA系プロンプトは全て `ReadLine` ＝stdinで対応可）。
- ⚠️ パスワードは**ASCII限定・64文字以内**（Steam仕様）。betaコードは`/headlessCode`でResonite botから取得（変動しうる＝編集可に）。TOTPの共有シークレットは保存しない。

---

## 6. 設定ファイル（統一）

旧来の散乱（`.env`/`config/auth.json`/`backend/config/*`・二重example）を**1ファイルに集約**。秘密は1箇所で自動生成。

- **アプリ設定**（例 `mrhc.config.json`）:
  - `adminPasswordHash`（ハッシュ保存）、`sessionSecret`（自動生成・Cookie署名鍵の導出に使用）
  - `port`、`headlessConfigDir`（Resonite の場所は `steam.installDir` に一本化）
  - `headlessCredentials`（Resoniteアカウント）
  - `restart`（トリガー/事前アクション/クラッシュ復帰設定）、`steam`（任意）
- **配置場所**: アプリ設定・状態ファイルは**書き込み可能なデータディレクトリ**に置く（バイナリ隣 or 設定可能な data dir。Linuxの権限を考慮）。
- **ヘッドレスconfig**: 公式 `HeadlessConfig.schema.json` 準拠のファイル群（`headlessConfigDir`）。`loginCredential`/`loginPassword` を含む（§3の通りファイル権限で保護）。アプリは**カスタムフォームエディタ**で生成・編集。

### configエディタ（カスタムフォーム・phase-7-spec §3.14）
> 改訂（7-4）: 当初の react-jsonschema-form 自動生成方針を見直し、**手書きのカスタムフォーム**に変更（公式スキーマの構造が深く、日本語ラベル/特殊UIの宣言調整より直書きが低コストと判断）。
- config は不透明な JSON map として扱い、フォームは主要キーのビュー。保存は「GET 全文 → フォーム管理項目だけ上書き → PUT」で**未知/レア項目を温存**（working-map 方式・マージ不要）。
- 複数ワールドはタブ式。`customSessionId` は prefix/suffix ビルダー、tags/allowedUrlHosts/autoSpawnItems は配列⇔フォーム変換。**生JSON 編集は撤去**（ほぼ全項目をフォーム化するため不要・同期/セキュリティの複雑さ回避）。
- バックエンドは name サニタイズ・`loginPassword` マスク・`startWorlds` 配列検証・`$schema` 付与のみ（値の意味検証はせず Resonite が権威。フォームがガードレール役）。

---

## 7. API / SSE 仕様

### 方針: 単一HTTP APIに1本化
UI専用の内部APIを持たず、**公開HTTP API 1本を Web UI もただの一クライアントとして使う**（UIでできること = HTTPでできること = 自動化/curl/ワールド内ボタンから操作可）。外部API安定性のため **`/api/v1/` のバージョンprefix** を付け、応答は**統一エラー形式** `{ ok, error: { code, message } }` とする。

### 認証（2経路・CSRF安全）
> 改訂（Pre-Phase 7）: 旧 APIキー方式を撤去し、秘密を**管理パスワード1つに統一**。詳細は phase-7-spec §2.1。

| クライアント | 認証 | 操作メソッド | CSRF |
|---|---|---|---|
| Web UI（人間） | **stateless HMAC 署名 Cookie**（`SameSite=Strict`・絶対失効30日） | **操作はPOST** | SameSiteで防止 |
| スクリプト/ワールド内 | **`Authorization: Bearer <管理パスワード>`** | GET/POST両対応 | Cookie不使用＝環境権限なし＝CSRF不成立 |

- 署名鍵 = HMAC(SessionSecret, adminPasswordHash)。サーバー側にセッション状態を持たず、**パスワード変更で署名鍵が変わり全 Cookie が自動失効**（失効リスト不要）。
- パスワード再設定: CLI `mrhc reset-password`（旧PW不要・復旧用） ＋ Web `POST /password`（現PW検証→新Cookie再発行で操作端末は継続）。
- HTTPS不採用（LAN/HTTP前提）。これによりCORSは不要（同一オリジン）。レート制限はログイン失敗の軽い間引き程度。

### 長時間操作は非同期
起動・停止・再起動・**Steam更新（数分かかる）**は、HTTPリクエストで待たせない。**即「受け付けた」を返し、進捗・結果は SSE ＋ 状態エンドポイント**で見せる。`worlds`/`kick`等の短い操作は同期でよい。

### エンドポイント（抜粋・`/api/v1` 配下。実装の正は code ＋ phase-7-spec §2.3/§2.4）
- 認証: `POST /login`、`POST /logout`、`POST /password`（管理PW変更）
- サーバー: `GET /status`、`POST /start`、`POST /stop`、`POST /command`、`GET /events`(SSE)（start/stop は非同期・受付応答）
- アプリ設定: `GET/PUT /app-settings`（port / Resoniteパス / configDir）
- ヘッドレスconfig: `GET /headless-configs`、`GET/PUT/DELETE /headless-configs/{name}`、`GET /headless-configs/last-used`、`GET/PUT /headless-credentials`（中央アカウント）
- ランタイム取得（GET）: `GET /sessions`、`GET /sessions/{idx}/status|users|detail`、`GET /listbans`、`GET /friendrequests`
- セッション操作（POST `/sessions/{idx}/…`・idx は path / 識別子・引数は body）: kick/ban/silence/unsilence/respawn/invite/role/message・accesslevel/maxusers/name/description/hidefromlisting・spawn/impulse（R14）・restart/save/close
- 新規セッション: `POST /sessions/start`（url / template）
- フレンド/BAN（グローバル・focus不要）: `POST /friendrequests/accept`、`POST /friends/add|remove`、`POST /bans/unban`、`POST /bans/banByID`（ID 指定 BAN・R1）
- Resonite 公開API プロキシ: `GET /resonite/users?q=`（ユーザー検索・無認証先）
- 再起動（**Phase 8・実装済**）: `GET/PUT /restart-config`、`GET /restart-status`、`POST /restart/trigger`、`POST /restart/cancel`
- Steam（**Phase 9・未実装**）: `POST /steam/update`(非同期)、`POST /steam/guard-code`、`GET /steam/config`。Guard要求時はSSE `steam` を `guard-required` 状態にしUIへ入力を促す

### ライブ更新（SSE）
- `GET /api/v1/events`（SSEストリーム）。イベント: `log`, `status`, `metrics`, `update`（起動/停止/再起動/Steam等の進捗）。
- 認証は Cookie（Web UI・同一オリジンで自動送出）。外部購読は `Authorization: Bearer <管理パスワード>`（ブラウザ EventSource はヘッダ不可のため curl/スクリプト用）。
- socket.ioは不採用。

---

## 8. 配布・ビルド・セットアップ

### ビルド
- フロント: `vite build` → 静的成果物を Go の `embed.FS` に取り込み。
- バックエンド: `go build`（`GOOS`/`GOARCH` で **`windows/amd64`・`linux/amd64`・`linux/arm64`（ARM Linux）の3ターゲット**をクロスビルド。CGO 不要の純 Go なので環境変数の切替だけで可）。CI（GitHub Actions: `.github/workflows/release.yml` のビルドマトリクス）で生成。
- **バージョン埋め込み**: リリースビルドは `-ldflags "-X main.version=<タグ名>"` で焼き込み（ローカルビルドは `dev`）。`mrhc -version` と起動メッセージで確認可能。将来の自己更新 Lv1（UI 表示＋新版チェック）の土台。

### 配布（R-E・実装済み）
- **形態**: Linux=`tar.gz` / Windows=`zip`。中身はトップレベルフォルダ `mrhc-<os>-<arch>/`（`mrhc` 本体＋README＋LICENSE）。**tar が実行権を保持するため `chmod +x` 案内は不要**。
- **アセット名にバージョンを含めない**（`mrhc-linux-amd64.tar.gz` 等）→ GitHub の `releases/latest/download/<asset>` **固定リンク**が常に最新安定版を指す。過去版は `releases/download/<tag>/` で取得可能。
- **install.sh**（リポジトリルート＋リリースアセット）: `curl -fsSL .../releases/latest/download/install.sh | sh` の 1 行導入。POSIX sh のみ（curl/tar/uname・distro 非依存・sudo なし）。`uname -m` で amd64/arm64 判定（armv8l 等の 32bit ユーザーランドは明示エラー）→ 最新 tar.gz を**カレントディレクトリ**に展開 → 起動手順を案内。再実行＝バイナリ上書き更新（config 等はアーカイブ外なので保持）。依存導入は MRHC 本体の担当（R-C）。`MRHC_DOWNLOAD_BASE` でダウンロード元を差し替え可（特定タグ固定・テスト用）。
- **SHA256SUMS** をリリースに添付（アーカイブ 3 つが対象。install.sh からの検証は v1 ではしない）。
- **prerelease 自動判定**: タグ名に `-` を含む（例 `v2.0.0-rc.1`）と prerelease として公開 → `latest` 固定リンクを汚さない。
- リリースは専用 job が全ターゲットの成果物を集約して一括添付（1 ターゲットでも失敗したらリリースしない＝不完全リリース防止）。.deb / AppImage は v1 スコープ外。

### CLIセットアップウィザード（Win/Linux完全共通・確定仕様: [docs/design/cli-onboarding.md](design/cli-onboarding.md)）
- 初回起動でconfig無しを検知 → 同一バイナリが対話プロンプト表示。SSH越し可・ブラウザ不要・`.bat`/`.ps1`不要。
- 流れ: **言語選択（ja/en・OS検出既定・configに保存=CLI/起動ログ/sys案内に適用）** → 管理パスワード → ポート → 保存 → 外部依存（freetype2/ARMの.NET10）の **[Y/n] 同意実行**（R-C・拒否でも続行）→ **Resoniteセットアップ（任意）**＝Steam資格＋ヘッドレスコード入力→その場でDL実行（Guardオフ必須の理由・手順を画面に明記）→ **「今すぐ起動しますか? [Y/n]」でそのままサーバー起動**（2回起動の廃止）。
- 入力契約: 不正値→理由表示で再入力・EOF→中断（パイプ実行で無限ループしない）・空Enter=既定値。**Resoniteパスは聞かない**（未設定＝既定 `{dataDir}/resonite` に導出・R-A）。
- 残り（セッション定義・再起動・ヘッドレス認証等）はログイン後の Web UI。

### 運用メモ
- MRHC自体は**手動起動**（PC起動時の自動起動は対象外）。ヘッドレスのクラッシュ自動復帰は §5.6。
- **DepotDownloader はベースセットアップに含めない**（Steam機能＝Resonite入手/更新を使う時だけMRHCが遅延DL。旧来の起動時自動DLの重さを回避。steamcmdはARM非対応のため廃止）。
- **MRHC自己更新=Lv1**: UIに現バージョン表示＋**GitHubの最新リリースをチェックして「新版あり＋DLリンク」**を通知。差し替えは手動（新バイナリ上書き→MRHC再起動。設定・状態は別ファイルなので保持される）。自己置換(Lv2)は将来の任意拡張。バージョン焼き込み（`-X main.version`・`-version` フラグ）は R-E で実装済み。
- **ファイアウォール/ポート開放**の注意を案内（友人が繋がらない時の典型原因）。→ **実装済（R-C→CLI改訂で両OS化）**: 起動バナーでLAN URL を主役に表示し、FW案内は Linux=firewalld/ufw のラベル付き例・Windows=初回ダイアログ「アクセスを許可する」＋誤キャンセル時の復旧手順（「ファイアウォールによるアプリケーションの許可」）。ポート使用中は専用文言（`platform.IsAddrInUse`）。

---

## 9. テスト戦略・要検証・実装順

### テスト戦略
- **パーサ**: 実機採取した生出力をフィクスチャ化し単体テスト（最重要。Resoniteのバージョン差にも気付ける）。採取した生出力は**フィクスチャと [`resonite-domain-facts.md`](./resonite-domain-facts.md) の両方に反映**する。
- **Console Driver**: 偽stdin/stdoutでコマンド直列化・原子的グループ・応答相関・タイムアウトを検証。
- **Restart Orchestrator**: トリガー→安全再起動の状態遷移をchannelモックで検証。
- **結合（任意）**: 実機ヘッドレスに対する手動/任意の結合テスト。

### 要検証（実装初期に潰す）
1. Linux起動方法は判明（`dotnet Resonite.dll -HeadlessConfig <f>`＋.NET10 Runtime、`Headless/`フォルダ内 `Resonite.dll`）。**残る確認＝文字コード**（Linux=UTF-8 / Windows=ロケールcodepage の想定でよいか）と**Headlessフォルダの正確な構成**（実機ディレクトリで確認）。→ Go小実証で検証。
2. chatWarningの到達範囲（`message`=フレンド限定）。割り切り or dynamicImpulse方針の確定。
3. Resoniteのワールド内HTTP送信が現状可能か（「GET操作」動機の検証。不可でもcurl/スクリプト用途は残る）。
4. パーサ各種の実出力書式（§5.3、`resonite-domain-facts.md`）。

### 実装順（=出荷順）
1. **Go小実証**: subprocess spawn ＋ Shift_JIS/UTF-8 入出力 ＋ embed.FS配信 ＋ SSE を Win/Linux 両方で確認（§要検証1も同時に）。
2. **v1.0 歩く骨格**（§2）。
3. **v1.x** を §2 の順で増分。

---

## 付録: 主要な設計判断の根拠（要約）
- 言語=Go: 単一バイナリ・両OSクロスビルド・常時稼働デーモンに最適な並行モデル。Rustは目的でなく、I/Oバウンドな本用途では摩擦過大。Bun+TSは僅差の次点。
- フロント刷新（流用せず）: 旧8941行モノリス・レスポンシブ崩れ・配色未整理・i18n無しは移植不能な負債。
- セキュリティ右サイズ: 脅威モデルは「LAN内の友人の誤操作防止」程度。
- スキーマ駆動configエディタ: 旧コードの最大の汚物（手書きエディタ）を構造的に解消。
- 段階出荷（歩く骨格）: 個人の全面書き直しで最大のリスク=「完成させ切ること」への対策。
