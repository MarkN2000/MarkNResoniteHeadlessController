# Phase 7+ (フロントエンド統合) 仕様書 — 改訂版

> ステータス: **7-3 新規セッション（URL/テンプレ起動＋検索枠予約）実装済**。7-7 第1層（write 失敗/成功トースト）実装済。Phase 9-A フレンド検索（申請/解除/招待＋Resoniteユーザー検索）実装済でフレンドタブは完成（7-2 承認+unban のクリーンMVP → P9-A で検索/関係操作を解禁）。7-1 セッションタブ・7-0 Foundation 実装済。Foundation=§3.7、7-1=§3.8、7-2=§3.9、P9-A=§3.10、7-7第1層トースト=§3.11、7-3新規セッション=§3.12、7-7残自動poll/PageVisibility=§3.13、コンフィグ(7-4)確定仕様=§3.14。仕様は v1 全機能監査（2026-05-29）を踏まえ全面再設計。次はコンフィグタブ(7-4)実装。
> 親設計: [docs/DESIGN.md](../DESIGN.md)
> 関連: [docs/design/structured-driver.md](structured-driver.md), [docs/resonite-domain-facts.md](../resonite-domain-facts.md)
> ARM/Steam 方針: メモリ `arm-support-plan`（Steam 更新 = DepotDownloader 統一）

---

## 0. 改訂の経緯

v1（Node + Svelte）を網羅監査した結果、Phase 7 当初仕様に **Config 管理・自動再起動・Steam 更新・Resonite API 連携** が欠落していたことが判明。MVP を再選定し、フェーズスコープと順序を全面見直した。

---

## 1. 改訂フェーズ計画

```
Pre-Phase 7 (バックエンド完成):
  - 認証改修: APIKey 撤去 / password を Bearer / stateless HMAC cookie (30日)
  - Config schema: version, SessionTTLHours 追加
  - Config CRUD API (一覧/読/書/削除/生成/last-used)
  - write API (~20 endpoints) + sessions/start (新規セッション)
  - ログインのみレート制限
  - fakehl 拡張 + 統合テスト

Phase 7 (UI MVP):
  - React + Mantine AppShell、トップバー主導モデル
  - 7 タブ（スケジュールは状態表示のみ、Steam/検索は P9 待ち）
  - SSE ライブログ

Phase 8 (自動再起動):
  - scheduled + userZero + waitControl
  - preRestartActions (chatMessage / itemSpawn / sessionChanges)
  - スケジュールタブの機能実装 ← 再起動条件の詳細設計はここで協議

Phase 9 (Resonite / Steam 統合):
  - ✅ P9-A: Resonite 公開API ユーザー検索（名/ID・無認証）＋フレンド申請/解除/招待（実装済・§3.10）
  - P9-B: Steam 更新（DepotDownloader 統一・2FA UI 入力→stdin・進捗 SSE）← 別計画・DESIGN §5.7
  - ※ ワールド検索（go.resonite.com スクレイピング）は DESIGN Should（将来実装・2026-05-31 判断修正で §Won't から格上げ）。新規セッションは現状 URL/テンプレート方式（7-3）＋検索枠を予約（§3.12）
```

### 廃止した v1 機能（採用しない）
- **HighLoad 再起動**（CPU/Mem 閾値）— 実利用での価値が薄く複雑度増
- **CIDR ホワイトリスト** — LAN ツールにつき OS firewall / ルータで守る
- **Headless 固有 JSON-RPC API**（`/api/headless/action`）— write API が同等を実現するため重複
- **Web セットアップ wizard** — CLI 対話で十分（admin 一度のみ）
- **light テーマ** — Resonite 公式カラーで dark 固定
- **SteamCMD** — ARM 非対応のため DepotDownloader に統一（メモリ `arm-support-plan`）

### レート制限
- **ログイン (`/auth/login`) のみ** 10req/15min。他は現状の auth lockout と併用。3 段階制限は採用しない。

---

## 2. Pre-Phase 7: バックエンド仕様

### 2.1 認証改修
- **APIKey 撤去**: password を Bearer トークンとして送る（HTTP リクエスト操作用）。秘密は 1 つに統一
- **stateless HMAC cookie**: SessionSecret×adminPasswordHash で署名鍵を導出、絶対失効 30 日。サーバー側セッション状態を持たない（パスワード変更で全トークン無効化）。CLI `mrhc reset-password` で旧PW不要の再設定可
- ブラウザ GUI = cookie、外部 HTTP 操作 = `Authorization: Bearer <password>`
- ログアウト = cookie 失効（クライアント側破棄）

### 2.2 Config schema 改修
- `version` フィールド追加（mrhc.config.json のスキーマ版管理）
- `SessionTTLHours` 追加（既定 30 日 = 720h）/ `Version`（SchemaVersion=1）追加

### 2.3 Headless Config CRUD API（Pre-7b・実装済み）
```
GET    /api/v1/headless-configs            一覧（name + comment + worldCount）
GET    /api/v1/headless-configs/{name}     読込（loginPassword は "" でマスク）
PUT    /api/v1/headless-configs/{name}     保存（新規/上書き = upsert）
DELETE /api/v1/headless-configs/{name}     削除
GET    /api/v1/headless-configs/last-used  前回起動 config 名
GET    /api/v1/headless-credentials        中央既定アカウント {username, hasPassword}（password 非返却）
PUT    /api/v1/headless-credentials        中央既定アカウント登録 {username, password}（password 空=既存保持）
```
**設計（保存型・最小検証）**: 実装は `internal/hlconfig`（HTTP 非依存）+ `internal/server/configs.go`（薄い HTTP 層）。
- **保存型**: フロントが完成 JSON を送り、バックエンドは name サニタイズ・最小検証（有効JSON + startWorlds が配列）・`$schema` 付与・0600 保存
- **name サニタイズ**: `^[A-Za-z0-9_\-]{1,64}$`（`/`・`\`・`.` 不可＝パストラバーサル防止）【必須】
- **保存先**: `headlessConfigDir`（既定固定 `{dataDir}/headless-configs`、Settings で上級者のみ変更）
- **同梱デフォルト**: 起動時に config dir が空なら `default.json`（accessLevel=Private・1ワールド・creds 空）を自動生成（`EnsureDefault`）
- **認証情報（起動時注入）**: config 自身の `loginCredential`/`loginPassword` が空なら、中央既定アカウント（`mrhc.config.json` の `headlessCredentials`）を注入。注入は**起動時**に行い、解決済み config を `{dataDir}/.run/{name}.json`（0600）へ生成して Resonite に渡す。保存済みファイルに password を焼き込まない（平文は中央設定 + 起動用一時のみ）
- **読込マスク**: GET は `loginPassword=""`。PUT は password 空=既存保持・非空=per-config 上書き
- **起動は config 名指定**: `POST /start {config: "<name>"}` → `headlessConfigDir` から解決。`driver.Start(headlessPath, launchPath, configLabel)` で Status には論理名を表示
- **last-used**: 起動成功時に `{dataDir}/runtime-state.json` に記録
- **同期**: credentials PUT（cfg 書込）と起動時読取の競合を `credMu sync.RWMutex` で防止

### 2.4 write API（idx は path、識別子/引数は body）

**設計方針（モデル A 確定・2026-05-30 仕様レビューで詳細確定）**
- idx は path、**ユーザー名等の識別子・引数は body**（任意文字列を安全に扱うため。path 埋め込みは廃止）
- セッション操作=`ExecGroup(focus idx → cmd)`、グローバル操作=`Exec`（focus 不要）、start=`Exec`（長 timeout）
- idx は**信頼**（focus 前の worlds 検証はしない。閉鎖時の index 繰り上がり誤爆は妥協＝狭い窓・read 系と同挙動）
- POST のみ、全て認証必須、ボディは JSON

```
# セッション内ユーザー操作  — ExecGroup(focus idx → cmd)
POST /api/v1/sessions/{idx}/kick            {"user":"..."}                  → kick "<user>"
POST /api/v1/sessions/{idx}/ban             {"user":"..."}                  → ban "<user>"（在席）
POST /api/v1/sessions/{idx}/silence         {"user":"..."}                  → silence "<user>"
POST /api/v1/sessions/{idx}/unsilence       {"user":"..."}                  → unsilence "<user>"
POST /api/v1/sessions/{idx}/respawn         {"user":"..."}                  → respawn "<user>"
POST /api/v1/sessions/{idx}/role            {"user":"...","role":"Admin"}   → role "<user>" "<role>"
POST /api/v1/sessions/{idx}/message         {"user":"...","message":"..."}  → message "<user>" "<text>"
POST /api/v1/sessions/{idx}/invite          {"user":"..."}                  → invite "<user>"（フォーカス中へ）

# セッション設定  — ExecGroup(focus idx → cmd)
POST /api/v1/sessions/{idx}/accesslevel     {"level":"LAN"}                 → accesslevel <Level>
POST /api/v1/sessions/{idx}/maxusers        {"maxUsers":8}                  → maxusers <N>
POST /api/v1/sessions/{idx}/name            {"name":"..."}                  → name "<name>"
POST /api/v1/sessions/{idx}/description     {"description":"..."}           → description "<text>"
POST /api/v1/sessions/{idx}/hidefromlisting {"hide":true}                   → hideFromListing <bool>

# セッションライフサイクル  — ExecGroup(focus idx → cmd)
POST /api/v1/sessions/{idx}/restart                                         → restart
POST /api/v1/sessions/{idx}/save                                           → save
POST /api/v1/sessions/{idx}/close                                          → close

# 新規セッション（稼働中に新ワールド）  — Exec（focus不要・timeout 60s）
POST /api/v1/sessions/start  {"mode":"url","url":"..."}                     → startworldurl "<url>"
                             {"mode":"template","template":"..."}          → startWorldTemplate "<name>"
# ※ /start（プロセス起動）とは別物。search 方式は Phase 9
# ※ template はテンプレ名に空白があり得るため QuoteArg で引用（決定C・引用可否は実機要検証）

# グローバル（フレンド/BAN）  — Exec（focus不要）
POST /api/v1/friendrequests/accept  {"user":"..."}                         → acceptfriendrequest "<user>"
POST /api/v1/friends/add            {"user":"..."}                         → sendFriendRequest "<user>"
POST /api/v1/friends/remove         {"user":"..."}                         → removeFriend "<user>"
POST /api/v1/bans/unban             {"userId":"..."}                       → unbanByID <userId>（listbans の UserID。素の unban は username 用・実機確定2026-05-30）
```

**引数の扱い**
- **enum（accesslevel/role）はサーバーで値リスト検証しない**（サニタイズのみ。値の権威は Resonite、UI が正値を提供）。
  UI ドロップダウン用の値: accesslevel=`{Private, LAN, Contacts, ContactsPlus, RegisteredUsers, Anyone}`（v1 実コードで確認）、role=`{Admin, Builder, Moderator, Guest, Spectator}`
- **maxUsers は正の整数のみ検証**（恣意的な上限は設けない＝Resonite の制限を推測しない）。name/description も恣意的な長さ上限なし
- **文字列サニタイズ（必須・全文字列引数）**: 生 `\r`/`\n` は送らない（injection 防止）。ユーザーの `\`/`"` は strip、その他制御文字は除去、文字列引数は `"..."` で囲む（enum/数値/bool は囲まない）。`<`/`>`/`=`/`/` は保持（リッチテキスト）。
  - **2系統の改行処理（実機確定 2026-05-30）**: `QuoteArg`（user/role/url/template）は実改行→リテラル `\n`。`QuoteRichText`（**name/description/message**）は実改行→ **`<br>`**（Resonite リッチテキストが `<br>` を改行レンダリング・ASCII で Shift_JIS 可。実機確認済）。
  - **status 読み取り注意**: name/description にリッチテキスト（`<color>`/`<s>`/`<br>` 等＝`>` を含む）やセッション名に `:` が入っても、Driver の `stripExactPrompt`（検出した実プロンプトをリテラル剥がし）で正しく読み戻せる（旧 `^([^>]*>)+` 貪欲剥がしは値の `>` を過剰除去するため撤去）。

### 2.5 操作結果の扱い（重要・実機制約）
Resonite の write 出力は **コマンドごとにバラバラで信頼できない**:
- kick/silence/respawn → UniLog スタックトレース混じりの複数行（正常時でも）
- maxusers/name/focus → **silent（出力なし）**
- invite → 出力不確定
- accesslevel/role → きれいな成功書式（パース可）

→ **方針 A 確定**: ノイジー出力をパースせず、「プロンプト `>` 復帰＝コマンド完了＝成功扱い」。`{"executed":true}` を返す。直後に該当データ（users / worlds 等）を再取得して**実状態で結果を見せる**。UI はトースト「実行しました」+ 再取得。

### 2.5.1 timeout / 実装方針
- **timeout**（`WithTimeout` で個別指定）: start 系=60s、restart=180s、**close=180s**、**save=600s**、他=既定5s。
  実機検証(2026-05-30)で restart/save/close は**プロンプトを返す**と確認済（空ワールドで restart≈1s / save・close≈0.2s）。
  save/close は**大規模ワールドのクラウド保存が数分かかり得る**ため長めに取る（タイムアウトは上限・保険であって
  プロンプト復帰で即返る／プロセス死亡時は ErrProcessGone で即中断）。`shutdown` のみ Exec 非対象（`Driver.Stop()`）。
- **反復ハンドラは helper 集約**: 単一引数 session-user 操作（kick/ban/silence/unsilence/respawn/invite）は
  `sessionUserOp` ファクトリ、引数なし lifecycle（restart/save/close）は `sessionCmdOp` ファクトリ、
  グローバル単一引数（accept/add/remove）は `globalUserOp` ファクトリで生成。引数付き（role/message/
  accesslevel/maxusers/name/description/hidefromlisting/start/unban）のみ個別実装
- **fakehl 拡張は最小**: write コマンドを受理し最小限の状態変更（例: kick で users から削除）+ プロンプトのみ。**ノイジー出力は再現しない**
- **エラー**: `writeExecErr` 流用（ErrNotReady→409 / その他→500）。空 user 等の入力検証は 400

### 2.5.2 実機検証バッチ（楽観実装→後でまとめて検証）
1. **コンソールのクォート/エスケープ規則**（`\n`/`\"`/`\\` を解釈するか）← 最優先・全文字列引数の土台
2. ban のフォーカス要否（在席必要か）/ kick が `users` の表示名で効くか
3. restart の所要時間（timeout 調整）/ `startWorldTemplate` の引数クォートと有効テンプレ名
4. **セッション閉鎖時の index 繰り上がり有無**（idx 信頼の前提確認）
5. invite/save/close/description の成功書式 / accesslevel の実有効値

### 2.6 その他の実機ドメイン制約（UI 設計に直結）
- **セッション全体チャット一斉送信コマンドは存在しない**。`message` は個別 DM のみ。「全員へ告知」は `users` 列挙 → 各人へ `message` ループで実現（preRestartActions の chatMessage も同方式）。UI で「ブロードキャスト」と誤認させない
- **無 config 起動はワールドが公開(Anyone)になる** → 起動は **config 必須**（無 config 起動ボタンは出さない）
- **focus は Resonite のグローバル状態** → 複数ブラウザ同時操作では共有される（LAN 単一管理者前提で許容）
- `startworldurl` / `startWorldTemplate` は実在（ランタイム新規ワールド可）。**テンプレート名一覧は実機で要採取**

---

## 3. Phase 7: UI 仕様

### 3.1 技術スタック（確定）
- React 19 + TypeScript + Vite 6
- Mantine v7
- react-i18next（ja + en 両維持）
- 既存 `web/` で継続開発

### 3.2 レイアウト: トップバー主導モデル
- **Mantine AppShell**（PC=サイドバー / モバイル=ハンバーガー overlay、自動切替）
- **レベル2 詳細は全画面 drill-down + 戻るボタン**（PC/モバイル統一、master-detail 分割はしない）

#### トップバー（2 モード）

**稼働中**
```
[PC]     │ MRHC │ 🎯[Hub  present2/users3/max8  Public ▾] │ ⋮ │
[Mobile] │ ☰ │ 🎯[Hub  p2/u3/8  Public ▾] │ ⋮ │
```
- 🎯ドロップダウン = フォーカス切替（選択で `focus N` 送信 + UI focusedIdx 更新）
- 各項目に present / users / max + アクセスレベル（= 全セッション一覧の役割を兼ねる）
- **遅延ロード**: 開いた瞬間キャッシュ表示 → `worlds` 送信 → 応答で最新化（常時 poll しない）
- ⋮ = 強制停止 / Steam 更新確認 / ログアウト（更新ありの時だけ ⋮ 付近にバッジ）
- モバイルは MRHC ロゴ等を消し、`☰ + ドロップダウン + ⋮` のみ
- 稼働中バッジ・稼働時間はトップバーに出さない（状態はトップバーの形が表す）

**停止中（丸ごと切替）**
```
[PC]     │ MRHC │ [起動] [default ▾] │ ⋮ │
[Mobile] │ ☰ │ [起動] [default ▾] │ ⋮ │
```
- [起動] ボタン + 隣にコンフィグ選択プルダウン（**前回起動コンフィグが初期選択** → 押すだけで起動）
- 起動ボタンの存在が「停止中」を表す（「起動していません」テキスト不要）
- config が空 → 「コンフィグタブで作成」へ誘導（起動不可）
- ⋮ = Steam 更新確認 / ログアウト（強制停止は非表示）

### 3.3 タブ構成（7 タブ）

| # | タブ | 階層 | 内容 | 停止中 |
|---|---|---|---|---|
| 1 | **セッション** | 1 | フォーカス中の設定（名前/アクセス/最大/AFK/説明/保存/再起動/閉じる）+ ユーザー一覧（アイコン/AFK/権限 + respawn/kick/ban/silence/role/message） | 「起動してください」 |
| 2 | **フレンド** | 2 | リクエスト一覧 / Ban 一覧 / フォーカス内ユーザー / 検索(P9) → ユーザー操作（承認/申請/解除/invite=フォーカス中へ） | 「起動してください」 |
| 3 | **新規セッション** | — | テンプレート / URL から起動（実装済・§3.12）＋ ワールド検索→起動の枠を予約（将来） | 「起動してください」 |
| 4 | **コンフィグ** | (編集) | v1 同等 CRUD（フォームのみ・タブ式複数ワールド・§3.14） | ✅ 使える |
| 5 | **スケジュール** | — | 状態表示（前回起動/稼働時間/次回再起動）+ 再起動予定（P8、現状「開発中」） | ✅ 使える |
| 6 | **設定** | — | アプリ設定 / パスワード変更 / Steam 設定(P9) | ✅ 使える |
| 7 | **コマンド** | — | SSE ライブログ + コマンド直送（上級者用） | 「起動してください」 |

- 操作の役割分担: **セッションタブ = セッションモデレーション**（respawn/kick/ban/silence/role/message）/ **フレンドタブ = 関係操作**（承認/申請/解除/invite）
- 停止中: セッション/フレンド/コマンド/新規セッションは「起動してください」表示。コンフィグ/スケジュール/設定はファイル編集なので使える

### 3.4 データ鮮度戦略
（セッションタブの自動 poll / Page Visibility は **7-7 残として実装済＝§3.13**。`hooks/useVisiblePolling`。）
- **Page Visibility 連動**: タブ非表示で poll 停止、再表示で即 refetch + 再開
- **コンポーネント unmount で停止**（画面遷移時）
- **アクティブな表示中タブのみ** poll
- セッションタブのフォーカス中詳細 = **イベント駆動（フォーカス変更時/操作後/手動更新ボタン）+ 表示中のみ 10s 自動**。1 回の取得 = `ExecGroup(focus→status→users)`
- フレンド/コンフィグ/設定 = onMount + 手動更新ボタン（自動 poll なし）
- ライブログ・プロセス状態・Steam 進捗 = SSE
- write 成功後 = onSuccess で該当データ再取得（方針 A）

#### SSE イベント
```
{type:"log",     ts, stream:"stdout"|"stderr", line}
{type:"process", ts, state:"started"|"stopping"|"stopped"|"crashed"}   # トップバーのモード切替を駆動
{type:"steam",   ts, phase, percent, log}    # Phase 9
```
- ユーザー参加/退出は SSE に流さない（stdout 解析の脆さ回避、poll で十分）

### 3.5 カラーパレット（Resonite 公式・dark 固定）

| 用途 | hex |
|---|---|
| 背景 (Dark) | `#11151d` |
| カード/サイド (Mid) | `#2b2f35` |
| ラベル/補助 | `#86888b` |
| 本文 (Light) | `#e1e1e0` |
| 主アクション (Cyan) | `#61d1fa` |
| 正常/起動中 (Green) | `#59eb5c` |
| 注意 (Yellow) | `#f8f770` |
| 強調/通知 (Orange) | `#e69e50` |
| 危険/停止 (Red) | `#ff7676` |
| 二次/上級 (Purple) | `#ba64f2` |

### 3.6 状態とフロー
- **ログイン**: シンプルカード（ロゴ + password input + ボタン）、失敗はカード内赤表示、連続失敗 10 回ロック
- **初回セットアップ**: CLI 対話のまま（Web wizard なし）。セットアップ完了後は auto-continue
- **セッション期限**: cookie 30 日（絶対失効）
- **危険操作**（kick/ban/強制停止/close）: 確認モーダル
- **トースト通知**: 操作完了/失敗

### 3.7 Foundation (7-0) で確定した実装事項

7-0（テーマ/AppShell/トップバー2モード/7タブ枠/SSE/プレースホルダ）の実装・反復で確定した決定。以降のタブ実装はこれに従う。

- **ナビ = 状態ベース**（react-router 不採用）。タブ/ドリルダウンは React state、戻るボタン方式。backend の SPA フォールバック不要。
- **配色の適用**（§3.5 の値の「配置」を確定。実装の単一情報源 = `web/src/theme.ts`）:
  - **ボタン全般 = Mid (#2b2f35) 基準・縁取りなし**（Mantine `variant="default"`）。起動/ログイン等もこれ。Cyan は「主アクション色」ではなく**選択/リンク等の控えめなアクセント**に留める（ユーザー決定。Resonite シーンインスペクタ準拠）。
  - **縁取りは TextInput/PasswordInput のみ**。エリア境界・Button・ActionIcon・Select の縁取りは撤去（`AppShell withBorder={false}` 等）。
  - **サイドバー背景 = `#1a2a36`（dark cyan）**、**選択タブ = 文字 yellow(#f8f770) / 背景 `#2b2e26`（dark yellow）**、非選択タブ = 白(Light)。パレット外の暗色2色は `theme.ts` の `SURFACE` に集約。
  - ロゴ = 白(Light)。状態ドット（稼働中トップバー）= running 緑 / 遷移中 黄。
- **i18n** = ブラウザ言語の**自動判定**（`navigator.languages` を prefix 一致）＋**選択式スイッチャ**（ログイン Select・⋮ メニュー）。対応言語の単一情報源 = `LANGUAGES`（言語追加 = locale JSON + resources + 1行）。手動切替は localStorage に保存し自動判定より優先。
- **フォーカス/セッション表示 = 2行**（上=セッション名〔長→自動縮小・`<br>`改行→折返し＋半分サイズ・行数 clamp で頭打ち〕／下=小さく `present/users/max · accessLevel`）。トップバーのフォーカスボタンとプルダウンで共用（`SessionTwoLine`）。§3.2 のモックアップの 🎯/1行表記はこの2行・状態ドット形に置換。
- **モバイル**: 1行トップバーで操作要素（☰/起動/⋮）は `flex-shrink:0`、config Select が `min-width:0` で幅を吸収（起動ボタンの文字が見切れない）。
- **開発支援**: `poc/fakehl` を MRHC の `-HeadlessConfig` で起動可能にし、その config の `startWorlds.sessionName` を世界名に使用 → 実機ヘッドレスなしで稼働中モード/セッション名の UI を確認できる（統合テストは configPath="" で従来通り＝無影響）。

### 3.8 セッションタブ (7-1) で確定した実装事項

7-1 でデザインを反復し確定した「インスペクタ風デザインシステム」。以降のタブ（コンフィグ/設定/フレンド等）はこの部品を流用する。実装の単一情報源 = `web/src/components/inspector/`（バレル `index.ts`）。

- **インスペクタ風部品**（Resonite シーンインスペクタ準拠・参考画像ベース）:
  - `InspectorCard` = カードヘッダ（中央=hero/yellow タイトル）＋**右隣に独立した別ボックスのアクション**（タイトルバーに重ねない）。本体は背景塗りなし（＝全体背景と同色）。
  - `FieldRow` = 1行「項目名（左・色マーカー）｜値/入力欄（右）」。
  - `InspectorTextInput`/`InspectorNumberInput`/`InspectorTextarea`/`InspectorSelect` = 入力ラッパ。スタイル/サイズ/▼アイコンを内蔵。
  - `InspectorButton`（`severity="neutral|warning|danger"` で **gray/yellow/red** に色分け・色の単一情報源）、`RefreshButton`（ヘッダの ⟳）。
- **配色/装飾ルール**: ヘッダ帯のみグレー、入力欄＝グレー fill、**縁取りは「キーボードで文字入力できる欄」のみ**（TextInput/NumberInput/Textarea）。Select は縁取りなし＋**▼ 1つ**（既定 chevron 置換）。読み取り専用はプレーン Text で区別。ボタン主アクション（適用）のみ cyan filled。
- **ユーザー一覧 = 案B 2行コンパクト**: 情報行（状態ドット〔在席=緑/離席=灰〕＋名前／権限プルダウン＋在席離席）／操作行（リスポーン・ミュート・メッセージ＝中立、キック・BAN＝危険・右に分離）。操作行は `wrap` でモバイル折返し（~294px でも崩れない実測）。権限は**選択即適用**。
- **確認ダイアログ**（`components/ConfirmModal`・ラベルは `common.*`）対象 = kick/ban/respawn/silence/unsilence ＋ save/restart/close。危険(kick/ban/close)は確定ボタン赤。メッセージは入力モーダル、適用はバッチ。
- **データ鮮度**: イベント駆動（マウント/フォーカス変更/操作後/手動 ⟳）＋ `useAsyncAction`（操作→完了後 refetch）。**トースト＝7-7 第1層（§3.11）／自動 poll・Page Visibility＝7-7 残として実装済（§3.13）**。**status と users は B1（commit bdb54be）で `GET /sessions/{idx}/detail`（ExecGroup(focus→status→users)）に集約済**（focus 往復半減・一貫スナップショット）。`/status`・`/users` は部分再取得用に残置。
- **レイアウト**: `components/SplitColumns`（再利用）。**xl(1408px) 未満＝1カラム**（max560・中央）、**xl 以上＝2カラム**（左=設定/右=ユーザー、**両パネル560固定**・中央寄せ・ページ全体スクロール）。スクロールバーは `ScrollArea type="hover"`（スマホは hover 無で非表示）。
- **開発支援（7-1 追加）**: fakehl にデモユーザー複数＋ role 反映を追加（スタンドインで一覧/即適用を目視確認）。統合テストは fallback で無影響。
- **対応済**: B1取得集約（commit bdb54be）・`maxUsers`空ガード。レビュー反映: フォーム編集保持(M1=sessionId変化時のみ再同期)・refetch失敗時データ保持(M3=初回のみエラー画面)・UserCard key衝突(L1)・🔇のa11y(L2)。
- **残課題**: ~~write失敗が現状無音（M2/L4）~~ → **✅ 完了（7-7 第1層・§3.11・commit 61af3e2）**。`useAsyncAction`/`useConfirm` で `WriteResult` を拾い失敗を赤トーストで通知。`getData` が 409(not ready) を区別しない点(L5)のみ現状維持（将来整理）。

### 3.9 フレンドタブ (7-2) で確定した実装事項

7-2 は **2セクション構成（v1 master-detail を踏襲・拡張性重視）**＋**クリーンMVP（承認 + unban のみ）**。実装は原則フロントのみ（バックエンドの `/friendrequests`・`/listbans`・`/friendrequests/accept`・`/bans/unban` は実装済）。コンポーネント = `web/src/tabs/friends/`（FriendsTab/SourcePanel/ResultList）。

- **構成の根拠**: v1(main) のフレンド機能は大半が **Resonite クラウドAPI ユーザー検索（=P9）依存**で、検索なしで一覧駆動で確実に成立するのは `承認`（リクエスト一覧）と `unban`（BAN一覧）の2つのみ（`申請`/`解除`/`招待` は検索選択駆動。特に `招待` は在席者では無意味＝実機 ambient のみ）。よって 7-2 は v1 の一覧駆動サブセットに限定。
- **2セクション**: ① `SourcePanel`（何を取得/検索するか）＋ ② `ResultList`（結果を1か所に集約）。`SplitColumns` で広い画面は左右、狭い画面は縦積み。**ソース毎にカードを増やさず②に集約**（検索追加でも増えない）。
- **オンデマンド取得**: タブを開いた時は**何も取得しない**（②は「ソースを選択」）。①のボタンを押した時だけ取得。②の ⟳ は**現ソースのみ**再取得（開く度に全要素を取りに行かない）。現ソースのボタンは filled でハイライト。
- **②=行内ボタン**（7-1 と同方式）: request 行→`[承認]`（neutral・**内向きなので即時**）／ ban 行→2行（名 + `[解除]`(danger) / `userId · Machine` dimmed）。`解除` は外向き/security のため**確認ダイアログ**（`ConfirmModal`・名+userId）。
- **検索の器**: ユーザー名/ID 検索・`[フォーカスセッション内]` は **disabled の「準備中(P9)」枠**として今から配置（将来 ②に検索/在席結果を出すだけ。申請/招待/解除ボタンも P9 で②の行に追加）。
- **流用部品**: `components/inspector`・`hooks/useConfirm`・`hooks/useAsyncAction`・`components/ConfirmModal`・`components/SplitColumns`。`unban` は `unbanByID <userId>`（v1 の素 `unban` 誤りを rewrite で修正済）。
- **開発支援（7-2 追加）**: fakehl スタンドインに `demoRequests()`/`demoBans()` を追加し、`acceptfriendrequest`/`unban(ByID)` で該当を除去（承認/解除→一覧から消えるのを目視確認）。統合テスト（configPath=""）は無影響。Chrome 実機相当で全フロー検証済。
- **対象外（P9 / 7-7）**: `申請`(sendFriendRequest)/`解除`(removeFriend)/`招待`(invite) ＋ ユーザー検索 → P9-A（§3.10 で実装済）。write失敗トースト → 7-7 第1層で実装済（§3.11）。

### 3.10 フレンド検索 (P9-A) で確定した実装事項

§3.9 の「準備中(P9)」枠を解禁。**Resonite 公開API（`api.resonite.com/users`・無認証）** によるユーザー検索＋
申請/解除/招待を実装。world 検索（go.resonite.com）は DESIGN Should だが将来実装のため本 P9-A の対象外。

- **検索 = 無認証プロキシ**: 新パッケージ `internal/resonite`（`Client.SearchUsers`・`User-Agent`・timeout 8s・baseURLテスト差替可）。
  `q` が `U-` 始まり→`/users/<id>`（単一）/ それ以外→`/users/?name=<q>`（配列）。iconUrl は `resdb:///<hash>.<ext>` →
  `https://assets.resonite.com/<hash>` に正規化。ルート `GET /api/v1/resonite/users?q=`（`requireAuth`）。**hermetic ユニット**（httptest stub）。
- **②に行種別を追加**: `search`（検索結果）/`focused`（フォーカス内在席者）。`UsersBody`＝アバター＋名前＋id＋
  `[申請][解除]`＋（search のみ）`[招待]`。招待は在席者では無意味のため focused には出さない。
- **すべて確認ダイアログ**（外向き操作・§3.9 方針）。`解除` は danger。`invite` は `POST /sessions/{idx}/invite`（focus 必要・
  `FriendsTab` が focusedIdx を受け取る）。`申請`/`解除`/`招待` の backend は実装済（api.ts ラッパ追加のみ）。
- **オンデマンド維持**: 検索も押した時だけ取得。`reqId` ガードで search/focused の取得競合も保護。⟳ は現ソース（検索は最後の語）を再取得。
- **既知の限界**: 公開APIは友達関係を返さないため検索行は常に3操作を出す（非該当操作の backend 失敗は **7-7 第1層トーストで通知＝§3.11**。ただし方針A上、意味的失敗で HTTP 200 が返る場合は無音のまま）。`invite` は実機出力未確定（方針A 受理表示）。
- **検証**: ユニット（resonite client + server ハンドラ・httptest）緑。Chrome 実機相当: 名前検索→実 api.resonite.com→結果＋アバター、申請の確認→実行、フォーカス内→申請/解除。**検索のみ外部公開APIに依存**（write は fakehl 経由でローカル完結）。
- **対象外**: world 検索（DESIGN Should・将来実装）。P9-B Steam/DepotDownloader（別計画・DESIGN §5.7）。

### 3.11 write 失敗/成功トースト (7-7 第1層) で確定した実装事項

7-7 第1層。これまで write 操作（承認/unban/申請/解除/招待/設定適用/save/restart/close/kick/ban/silence/respawn/role/message ＋ 起動）の失敗が**完全無音**だった穴を塞ぐ。commit 61af3e2。

- **失敗判定 = HTTP/トランスポートレベルのみ（＝方針A の範囲）**: `WriteResult{ok,error,code}` の `!ok`（停止中=not_ready / timeout / process_gone / 通信不通=network / 入力不正=bad_request）を**赤トースト**で明示。成功は受理ニュアンスの**緑トースト（2s で自動消滅）**。方針A 上トーストは「**届いた**」保証であり「**効いた**」保証ではないため over-claim しない（意味的失敗で HTTP 200 が返るケースは無音のまま＝許容）。
- **設計の肝＝2フック集約**: 失敗/成功を**2つの実行フック（`hooks/useAsyncAction.run(fn, success?)` / `hooks/useConfirm.confirm()`）で1回だけ拾う**。呼び出し側はファクトリ関数が `WriteResult` を return するだけ（低 churn・全 write 操作を最小改修で網羅）。複数 write を束ねる適用系は「最初の失敗」を返す（`results.find(r => !r.ok) ?? {ok:true}`）。
- **UI サイドエフェクトの隔離**: トースト表示は **`web/src/lib/notify.ts`**（`reportWriteResult(result, success?)` / `notifyError(message, title?)`）に閉じ込め、`api.ts` は純データ層のまま（低結合・SOLID）。`reportWriteResult` は型ガードで `WriteResult` のみ反応（無関係な戻り値は無視）。
- **失敗本文 = backend `error.code` を権威に localize**: `post()` が `error.code` も抽出（`WriteResult.code` 追加）。`notify.ts` が code→i18n キー（`toast.errNotReady`/`errTimeout`/`errProcessGone`/`errNetwork`/既定 `errGeneric`）へ写像。コンポーネント外解決のため **singleton `i18n.t`**（`i18n.ts` の `export default i18n`）を使用。
- **マウント**: `@mantine/notifications` を追加し、`main.tsx` に `<Notifications position="bottom-right" />` を**全タブ共通で1つだけ**マウント（`@mantine/notifications/styles.css` も import）。
- **起動失敗**: `onStart` は write 操作とは別系統（`WriteResult` を通さない）。`api.start` の `!ok` と通信不通 throw を `try/catch` で拾い `notifyError(..., t("toast.startFailTitle"))` で赤化（旧 `TODO(7-7)` 解消）。
- **第2層/第3層は不採用（方針A 維持）**: 第2層=出力パース（"Unknown command" 等）は脆く保守コスト高で方針A と矛盾。第3層=再取得差分判定は一覧駆動（承認/unban）のみ成立し競合と区別不可。→ 第1層（HTTP/トランスポート）のみ採用。
- **i18n**: `toast.*`（失敗タイトル2種・エラー本文5種・成功メッセージ16種）を ja/en に追加。
- **検証**: `npx tsc --noEmit` / `npm run build` 緑。Chrome 実機相当で**緑「フレンド申請を承認しました」**（トースト DOM を MutationObserver で捕捉）＋ **赤「サーバーに接続できません」**（backend kill で network 経路）を確認。
- **流用基盤化**: 以降の write 系タブ（7-3 等）は `lib/notify` を追加コストなしで継承（`useAsyncAction`/`useConfirm` 経由なら自動でトースト化）。

### 3.12 新規セッション (7-3) で確定した実装事項

稼働中ヘッドレスに**ランタイムで新ワールドを開始**するタブ。バックエンド `POST /api/v1/sessions/start`（url/template・execGlobal・focus 不要・timeout 60s・commit 7bc6e9b）は実装済のため **フロントのみ**（7-2 / P9-A と同様）。コンポーネント = `web/src/tabs/newsession/`（NewSessionTab/StartPanel/WorldSearchPanel）。

- **2セクション構成（v1 踏襲・レイアウト前方互換が主眼）**: `SplitColumns` で ① `StartPanel`（左=起動方法・機能）＋ ② `WorldSearchPanel`（右=検索して起動・**将来対応の disabled プレースホルダ**）。②を**今から枠だけ予約**することで、将来ワールド検索を実装してもレイアウトが変わらない（7-2 の「準備中(P9)」枠予約と同手法）。
- **起動方法 = URL ＋ テンプレートの2手段**（左カラム）。`InspectorCard` 内に `FieldRow` 2行（各行 値側に `Group([input][起動])`）。
  - **テンプレート = 固定3択** `Grid / Platform / Blank`（既定 Grid・`api.WORLD_TEMPLATES`・`InspectorSelect`）。v1 の `templateSuggestions` 踏襲。**他テンプレ名が現行 Resonite で使えるかは要実機採取**（§4）。
  - **URL** = `InspectorTextInput`（placeholder `resrec://...`）。**scheme をクライアント検証** `^res[-\w]*:\/\//i`（v1 踏襲）。空 or 不一致は [起動] を `disabled`、非空かつ不正時のみ下にヒント文。方針A 上、不正 URL でも backend は HTTP 200 を返し得る（＝無音失敗）ため空振りを減らす狙い。
  - **[起動]は2つともニュートラル灰**（`InspectorButton severity="neutral"`・§3.7「ボタン全般=Mid grey」踏襲。適用のような cyan filled にはしない＝ユーザー決定）。
- **起動前に確認ダイアログあり**（`useConfirm` + `ConfirmModal`）。`confirm.busy` がモーダルの loading を駆動（startworldurl は最大60s かかり得る）。`onConfirm` が `WriteResult` を return → 結果トーストは 7-7 第1層基盤で自動（成功=緑 `toast.newSessionDone`／失敗=赤）。
- **起動成功後はトップバーのセッション一覧を再取得**（`App.tsx` の `refreshSessions` を `onStarted` で渡す）→ 新ワールドがプルダウンに出現＝方針A の「再取得で実状態を見せる」。
- **ワールド検索枠（右・将来実装）**: disabled 検索入力＋「準備中（将来対応）」注記＋グレーのスケルトン結果カード2枚（将来のサムネ＋名前グリッドの形）。**ワールド検索は 2026-05-31 の判断修正で DESIGN §Won't → Should に格上げ（実装は将来）**。移植元 = v1 の `go.resonite.com` HTML スクレイピング（`GET /world-search?term=` → `ol.listing li a.listing-item` から name/画像/`R-`レコードID/`U-`(or `G-`)所有者ID を抽出し `resrec:///<owner>/<record>` を生成）。公式 API にワールド検索は無いため go.resonite.com 依存（HTML 構造変更で壊れ得る点は受容）。**検索結果からの起動は既存 URL モード（`startworldurl "<resrec:// URL>"`）を流用**でき、追加バックエンドは"検索ソース1本"のみ。
- **開発支援**: fakehl は `startworldurl`/`startworldtemplate` を受理し新ワールドを worlds に追加済（スタンドインで一覧出現を確認可能）。
- **既知の制約**: ① 不正 URL/未知テンプレは方針A で無音失敗になり得る（scheme 検証で軽減）。② 起動後の headless 自動 focus 挙動は未確定 → MVP は一覧再取得のみ（自動でセッションタブへ切替しない・将来検討）。

### 3.13 セッションタブの自動 poll / Page Visibility (7-7 残) で確定した実装事項

§3.4 のデータ鮮度戦略の残り。これまで全タブが純イベント駆動で、ユーザーの参加/退出は手動 ⟳ まで反映されなかった。**セッションタブのみ**、表示中に背景 poll を加える。

- **再利用フック `web/src/hooks/useVisiblePolling(fn, intervalMs)`**: 表示中のみ `fn` を一定間隔で実行。**重複起動しない**（再帰 `setTimeout`＝前回完了後に次を予約）／`document.hidden` で停止・再表示で**即時実行＋再開**（`visibilitychange`）／unmount で停止／`fn` は ref 経由で常に最新（依存に入れず interval を再購読させない＝idx 変化でタイマーをリセットしない）。初回 poll は +intervalMs（マウント直後の即時取得と二重化させない）。
- **`SessionTab`**: `refetch` に `{ silent? }` を追加。背景 poll は `silent:true`＝`loading` を触らず **⟳ スピナーを回さない**（画面のチラつき防止）。手動 ⟳ / マウント / focus 変更 / 操作後は従来どおりスピナー表示。`useVisiblePolling(() => refetch({ silent:true }), 10_000)`（§3.4 の 10 秒）。
- **背景 poll の失敗は無音**（既存 M3：表示中データを保持・トーストは出さない＝10 秒ごとの赤通知を防ぐ）。
- **スコープ（§3.4 準拠）**: セッションタブのみ。フレンド/コンフィグ/設定は自動 poll なし。**トップバーの全世界人数（`GET /sessions`＝`worlds`・focus 不要）は poll 対象外** → セッションタブ自身の人数/一覧は 10 秒で追従するが、トップバーのプルダウン人数は手動 ⟳ まで古いまま（既知の制限・要望あれば別途 `worlds` poll を追加可）。
- **focus 競合**: 背景 poll は `focus idx→status→users` を行うが、MRHC の全 write も毎回 focus し直すため実害は軽微（§2.6 の単一管理者前提で許容済）。10 秒間隔で REPL 競合確率も小。

### 3.14 コンフィグタブ (7-4) の確定仕様（実装前・本節がレビュー確定の単一情報源）

Phase 7 最大の未着手機能。headless config（`*.json`）の CRUD エディタを v1 同等で作り直す。**バックエンド CRUD は実装済**（§2.3・`internal/hlconfig`＋`internal/server/configs.go`）のため **フロントのみ**（7-2/7-3/P9-A と同様・改修ゼロ）。決定経緯は 2026-05-31 のレビュー（v1 `main:frontend/src/routes/+page.svelte` 監査含む）。

**前提（バックエンドの性質）**
- config は**不透明な JSON map**。MRHC は name サニタイズ・`loginPassword` マスク/保持・`$schema` 付与・`startWorlds` 配列検証のみ。**未知フィールドは保持**。
- 値の意味検証はしない（accessLevel/preset 名の正当性は Resonite が権威）→ フォームがガードレール役。
- name = ファイル名（`^[A-Za-z0-9_\-]{1,64}$`）。リネーム API 無し。PUT は upsert。GET は `loginPassword=""` マスク。

**(A) エディタ方式＝フォームのみ（生JSON 撤去）**
- 整形フォーム＋複数ワールドは**タブ式**。**生JSON 編集セクションは設けない**（ほぼ全項目をフォーム化するため不要・フォーム↔JSON 同期の複雑さとセキュリティ懸念を回避。v1 の編集可能 JSON プレビューも省く）。
- **未知/レア項目の温存は保存ロジックで invisible に継続（必須）**：保存時に `GET 全文 → フォーム管理項目だけ上書き → 残り（レア/未知）はそのまま → PUT`。これが無いと編集保存で `parentSessionIds`/`*CloudVariable` 等が消失する。
- トレードオフ：UI 非搭載のレア項目（下記）は**温存のみ・UI 編集不可**。必要時に後日フォーム化。

**(D) レイアウト・CRUD**
- 左=config 一覧レール（~260・名前＋worldCount バッジ・選択で右に読込・上部に [＋新規]）／右=エディタカード。狭幅は「config 選択プルダウン＋エディタ」に畳む（`SplitColumns` 流儀）。
- 新規（name 入力→同梱デフォルト雛形）／複製（GET→別名 PUT）／削除（確認）／保存（dirty 追跡＋未保存ガード）。**リネーム非提供**（複製→旧削除で代替）。
- **誤上書き防止**：新規/複製で**既存 name と衝突したら警告**（PUT が黙って上書きするため）。name は即時バリデーション。

**(B) アカウント**
- config 毎に任意の `loginCredential`/`loginPassword` 欄（空=中央アカウント注入）。password マスク・空=変更なし。中央アカウント設定自体は設定タブ（次フェーズ）の領分。

**(C) フィールド構成＝v1 同等（基本的に全フォーム化）**
- config トップ（フォーム）：`comment`・`tickRate`・`maxConcurrentAssetTransfers`・`usernameOverride`・`dataFolder`・`cacheFolder`・`logsFolder`・`allowedUrlHosts`（add/remove リスト）・`autoSpawnItems`（カンマ→配列）＋アカウント欄。
- 各ワールド（startWorlds[]・タブ・フォーム）
  - 基本：`isEnabled`（タブ有効/無効）・`sessionName`・`description`・`accessLevel`・`maxUsers`・`loadWorldPresetName`＋`loadWorldURL`（**両表示**・スキーマ上両立可・どちらが効くかは Resonite 依存＝URL 優先）・`customSessionId`（**prefix/suffix ビルダー**・`:` 分割/結合・自動補完=ボット名依存は将来）。
  - 運用：`tags`（カンマ→配列）・`awayKickMinutes`・`idleRestartInterval`・`forcedRestartInterval`・`autosaveInterval`（各 `-1=無効` 注記）・`saveOnExit`・`autoRecover`・`autoSleep`・`hideFromPublicListing`・`mobileFriendly`。
- **温存のみ（UI 非搭載）**：`universeId`・`useCustomJoinVerifier`・`forcePort`・`enableResoniteLink`・`forceResoniteLinkPort`・`keepOriginalRoles`・`defaultUserRoles`・各 `*CloudVariable`・`parentSessionIds`・`autoInvite*`・`saveAsOwner`・`overrideCorrespondingWorldId` ＋未知フィールド。

**安全/堅牢**
- 新規 config の `accessLevel` 既定は **Private**（v1 の Anyone と違い安全側。no-config/誤 accessLevel は公開事故＝domain-facts §7）。
- **未保存ガード**：dirty 時は **config 切替・新規作成・複製**で破棄確認を挟む（`guardDiscard` で3経路統一）。複製は保存状態（`original`）をクローン。同一 config 内のワールドタブ移動は保存単位が同じため不要。ワールド削除・config 削除は確認ダイアログ。**既知の制限**：コンフィグタブから**他タブへ離脱**すると未保存編集は警告なく失われる（アプリ横断の未保存ガードは未実装＝他タブ方針と整合・MVP 許容）。
- **ワールドは最低1つ**（最後の1枚は削除不可）。
- **稼働中の config 編集は再起動まで未反映**の注記。
- `loginPassword` は常時マスク（タイプした平文を画面に再表示しない）。
- 不正 JSON の config はロード失敗を画面表示（クラッシュしない・一覧には name のみ出る）。

**ワールド検索（将来）**：`loadWorldURL` の検索ピッカーは将来（ワールド検索＝DESIGN Should・将来実装・§3.12）。今は URL プレーンテキスト欄。

**流用部品**：`components/inspector`（InspectorCard/FieldRow/InspectorSelect/InspectorTextInput/InspectorNumberInput/InspectorTextarea/InspectorButton/RefreshButton）・`hooks/useConfirm`＋`ConfirmModal`・`hooks/useAsyncAction`・`lib/notify`（結果トースト自動）・`SplitColumns`。

**バックエンド**：改修ゼロ。GET `/headless-configs`（一覧）・GET `/headless-configs/{name}`（全文・pw マスク）・PUT（upsert）・DELETE。新規雛形はフロントが同梱デフォルト（Private・1ワールド・creds 空）を保持。

---

## 4. 後で詳細協議が必要な領域
- **スケジュールの再起動条件**（モジュール式設計・待機制御 waitControl）← Phase 8 着手前
- **設定タブの中身**
- **新規セッション「テンプレート」の実体**（`startWorldTemplate` の有効テンプレート名）← 実機採取
- **Steam（DepotDownloader）の 2FA UI フロー詳細** ← Phase 9、ARM 実機採取と合わせて（メモリ `arm-support-plan`）

---

## 5. 確定済み設計選択（参考）

### 5.1 採用
- 構造化 Console Driver（executor.go 分離、案C'+案X）
- ExecGroup（原子的グループ、focus 競合防止）
- WorldsService.ForEach（巡回機構、事前アクション等で使用予定）/ List()（userZero 検知）
- Driver 側 `stripExactPrompt`（検出プロンプトをリテラル剥がし。`lastPrompt`＋`cur`、全行適用）
- ParseFriendRequests（v1 互換、実機 format で確証済）
- Encoding 抽象（Win=Shift_JIS / Linux=UTF-8 実機確証済）
- sentinel エラー（ErrNotReady/Timeout/ProcessGone/Canceled）
- fakehl（テスト基盤）

### 5.2 撤去
- HeadlessBackend 抽象（Resonite API 提供予定なしのため概念負債）
- isLikelyUsername（ヒューリスティック、実機データなしで設計すべきでない）
- /command form-urlencoded body（利用者ゼロ）
- writeExecErr 5 区分（2 区分に簡素化）

---

## 6. 実機検証済 fixtures

| 採取日 | 場所 | 内容 |
|---|---|---|
| 2026-05-28 | `fixtures/2026-05-28-windows-multiworld.log` | 複数ワールド・コマンド網羅 |
| 2026-05-28 | `fixtures/2026-05-28-lan-login/01-lan-joined-anonymous/` | LAN join + 別PC参加 |
| 2026-05-28 | `fixtures/2026-05-28-lan-login/02-logged-in/` | login 入り + listbans + help 全コマンド |

friendrequests の実 format は `MARKNPC_SUB2` アカウント手動採取で確証済（匿名化済テスト追加）。
