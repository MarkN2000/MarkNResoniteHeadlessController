# Phase 7+ (フロントエンド統合) 仕様書 — 改訂版

> ステータス: **着手前 / 仕様確定**。v1 全機能監査（2026-05-29）を踏まえ、フェーズ計画・UI を全面再設計。
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
  - Resonite API 連携（ユーザー検索/詳細/ワールド検索）
  - 新規セッション「検索」方式 + フレンド「検索」
  - Steam 更新（DepotDownloader 統一・2FA UI 入力→stdin・進捗 SSE）
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
- **timeout**: start 系=60s、restart=180s（暫定）、他=既定5s（`WithTimeout` で個別指定）。
  ⚠️ restart が実機でプロンプトを返さない場合（driver.go の警告）、成功でも ErrTimeout→500 を誤報し、
  かつ最大 180s 間 execMu を占有して他コマンドを止める。検証バッチ項目3で確認するまでの楽観実装。
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
| 3 | **新規セッション** | — | テンプレート / URL / 検索(P9) | 「起動してください」 |
| 4 | **コンフィグ** | (編集) | v1 同等 CRUD を作り直し | ✅ 使える |
| 5 | **スケジュール** | — | 状態表示（前回起動/稼働時間/次回再起動）+ 再起動予定（P8、現状「開発中」） | ✅ 使える |
| 6 | **設定** | — | アプリ設定 / パスワード変更 / Steam 設定(P9) | ✅ 使える |
| 7 | **コマンド** | — | SSE ライブログ + コマンド直送（上級者用） | 「起動してください」 |

- 操作の役割分担: **セッションタブ = セッションモデレーション**（respawn/kick/ban/silence/role/message）/ **フレンドタブ = 関係操作**（承認/申請/解除/invite）
- 停止中: セッション/フレンド/コマンド/新規セッションは「起動してください」表示。コンフィグ/スケジュール/設定はファイル編集なので使える

### 3.4 データ鮮度戦略
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

---

## 4. 後で詳細協議が必要な領域
- **スケジュールの再起動条件**（モジュール式設計・待機制御 waitControl）← Phase 8 着手前
- **コンフィグエディタの具体的フィールド構成**（v1 のフォーム項目を作り直し）
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
