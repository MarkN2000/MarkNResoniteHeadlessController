# Phase 7+ (フロントエンド統合) 仕様書 — 改訂版

> ステータス: **🎉Phase 7 UI 全7タブ完成＋Phase 8（自動再起動）完了（〜commit 7319fbd）**。実装済タブ: 7-0 Foundation(§3.7)／7-1 セッション(§3.8)／7-2 フレンド(§3.9)＋P9-A 検索(§3.10)／7-7 第1層トースト(§3.11)／7-3 新規セッション(§3.12)／7-7残 自動poll・PageVisibility(§3.13)／7-4 コンフィグ(§3.14)／7-5 設定(§3.15)／スケジュール(§3.16・Phase 8)。仕様は v1 全機能監査（2026-05-29）を踏まえ全面再設計。**Phase 8（自動再起動）= バックエンド（P8-1〜P8-4）＋UI（P8-5a 状態/手動・5b-1 待機/事前/クラッシュ・5b-2 予定リスト/編集モーダル）すべて実装・実機検証済。次は P9-B（Steam/DepotDownloader・ARM）**。
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

Phase 8 (自動再起動): 確定仕様=§3.16（2026-06-01 協議）
  - トリガー=手動通常 + scheduled の2つ（userZero/highLoad/chatMessage は不採用）
  - 統一安全再起動フロー（0人即/①セッション変更→②待機→③dynamicImpulse告知→④停止起動）+ waitControl + クラッシュ自動復帰
  - スケジュールタブ実装: P8-1〜P8-5b 完了（Phase 8 完了・全7タブ完成）

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
- **同梱デフォルト**: 起動時に config dir が空なら `default.json`（accessLevel=Anyone・公式スキーマ全項目を明示・1ワールド・creds 空）を自動生成（`EnsureDefault`）。フロント `defaultConfig()`/`defaultWorld()` と同一方針（UI 表示＝保存値の一致／未設定は null）
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

# セッション内コンテンツ操作  — ExecGroup(focus idx → cmd)・R14
POST /api/v1/sessions/{idx}/spawn   {"url":"...","active":true,"persistent":false} → spawn "<url>" <active> <persistent>（3引数・help 確定）
POST /api/v1/sessions/{idx}/impulse {"tag":"MRHC.play","value":"..."}            → dynamicimpulsestring "<tag>" "<value>"（tag 必須・value 任意）
# ※ コマンド組み立ては headless.SpawnCmd / DynamicImpulseStringCmd（純関数）。告知③(§3.16(2))と共有。

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
POST /api/v1/bans/banByID           {"userId":"..."}                       → banByID <userId>（全セッションから BAN・在席不要・検索結果用・R1。unban と対称）
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
| 5 | **スケジュール** | — | 自動再起動（手動通常/scheduled・安全再起動フロー・事前アクション・クラッシュ復帰）＋予定リスト/編集モーダル＋状態表示（P8・§3.16・**P8-1〜P8-5b 実装済＝Phase 8 完了**） | ✅ 使える |
| 6 | **設定** | — | 管理PW変更 / Resoniteアカウント / アプリ設定 / Steam設定(P9・枠のみ)（実装済・§3.15） | ✅ 使える |
| 7 | **コマンド** | — | SSE ライブログ + コマンド直送（上級者用） | 「起動してください」 |

- 操作の役割分担: **セッションタブ = セッションモデレーション**（respawn/kick/ban/silence/role/message）/ **フレンドタブ = 関係操作**（承認/申請/解除/invite）
- 停止中: セッション/フレンド/コマンド/新規セッションは「起動してください」表示。コンフィグ/スケジュール/設定はファイル編集なので使える

### 3.4 データ鮮度戦略
（セッションタブの自動 poll / Page Visibility は **7-7 残として実装済＝§3.13**。`hooks/useVisiblePolling`。）
- **Page Visibility 連動**: タブ非表示で poll 停止、再表示で即 refetch + 再開
- **コンポーネント unmount で停止**（画面遷移時）
- **アクティブな表示中タブのみ** poll
- セッションタブのフォーカス中詳細 = **イベント駆動（フォーカス変更時/操作後/手動更新ボタン）+ 表示中のみ 10s 自動**。1 回の取得 = `ExecGroup(focus→status→users)`
- フレンド/コンフィグ = onMount + 手動更新ボタン（自動 poll なし）／設定 = onMount（保存後に再評価・自動 poll なし）
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
  - **filled ボタンの文字コントラスト = theme `autoContrast`**（commit 16a4a69）。個別の `styles` で文字色を上書きせず、テーマ一括で背景に応じた可読色を自動付与（保存バー dirty 時の brand filled 等）。グローバル CSS で底上げ。
- **i18n** = ブラウザ言語の**自動判定**（`navigator.languages` を prefix 一致）＋**選択式スイッチャ**（ログイン Select・⋮ メニュー）。対応言語の単一情報源 = `LANGUAGES`（言語追加 = locale JSON + resources + 1行）。手動切替は localStorage に保存し自動判定より優先。
- **フォーカス/セッション表示 = 2行**（上=セッション名〔長→自動縮小・`<br>`改行→折返し＋半分サイズ・行数 clamp で頭打ち〕／下=小さく `present/users/max · accessLevel`）。トップバーのフォーカスボタンとプルダウンで共用（`SessionTwoLine`）。§3.2 のモックアップの 🎯/1行表記はこの2行・状態ドット形に置換。
- **モバイル**: 1行トップバーで操作要素（☰/起動/⋮）は `flex-shrink:0`、config Select が `min-width:0` で幅を吸収（起動ボタンの文字が見切れない）。
- **開発支援**: `poc/fakehl` を MRHC の `-HeadlessConfig` で起動可能にし、その config の `startWorlds.sessionName` を世界名に使用 → 実機ヘッドレスなしで稼働中モード/セッション名の UI を確認できる（統合テストは configPath="" で従来通り＝無影響）。

### 3.8 セッションタブ (7-1) で確定した実装事項

7-1 でデザインを反復し確定した「インスペクタ風デザインシステム」。以降のタブ（コンフィグ/設定/フレンド等）はこの部品を流用する。実装の単一情報源 = `web/src/components/inspector/`（バレル `index.ts`）。

- **インスペクタ風部品**（Resonite シーンインスペクタ準拠・参考画像ベース）:
  - `InspectorCard` = カードヘッダ（中央=hero/yellow タイトル）＋**右隣に独立した別ボックスのアクション**（タイトルバーに重ねない）。本体は背景塗りなし（＝全体背景と同色）。
  - `FieldRow` = 1行「項目名（左・**色マーカー＝ハンドル**）｜値/入力欄（右）」。マーカーは Resonite シーンインスペクタ風ハンドル（種別色の縦バー＋白3本線）。**`onMarkerClick` を渡すとマーカーがボタン化**（hover で明色化〔`index.css` の `.mrhc-field-marker`〕・キーボード操作可）。
  - **マーカー＝「既定値に戻す」**（commit bf61415）: マーカークリック→`ConfirmModal`「『{項目}』を既定値に戻しますか？」→**その項目だけ**既定値へリセット（フィールド単位の取り消し）。採用箇所＝コンフィグ（`GeneralSection`/`WorldsSection`・既定は `configModel.defaultConfig()`）＋スケジュールの設定カード（待機制御/事前アクション/クラッシュ復帰・既定は `scheduleModel.default*()`＝backend `config.DefaultRestart()` ミラー）。予定リスト/状態/手動カードは対象外（フィールド単位の既定が無いため）。i18n=`common.resetToDefault`/`resetConfirmTitle`/`resetConfirmMsg`。
  - `InspectorTextInput`/`InspectorNumberInput`/`InspectorTextarea`/`InspectorSelect` = 入力ラッパ。スタイル/サイズ/▼アイコンを内蔵。
  - `InspectorButton`（`severity="neutral|warning|danger"` で **gray/yellow/red** に色分け・色の単一情報源）、`RefreshButton`（ヘッダの ⟳）。
- **配色/装飾ルール**: ヘッダ帯のみグレー、入力欄＝グレー fill、**縁取りは「キーボードで文字入力できる欄」のみ**（TextInput/NumberInput/Textarea）。Select は縁取りなし＋**▼ 1つ**（既定 chevron 置換）。読み取り専用はプレーン Text で区別。ボタン主アクション（適用）のみ cyan filled。
- **ユーザー一覧 = 案B 2行コンパクト**: 情報行（状態ドット〔在席=緑/離席=灰〕＋名前／権限プルダウン＋在席離席）／操作行（リスポーン・ミュート・メッセージ＝中立、キック・BAN＝危険・右に分離）。操作行は `wrap` でモバイル折返し（~294px でも崩れない実測）。権限は**選択即適用**。
  - **自分（ホスト）への危険操作を無効化（R3）**: `selfUserId`（`status.loginUserId`）と `u.id` が一致する行は **mute/kick/ban/権限プルダウンを `disabled`（グレーアウト）** にし **respawn+message のみ**許可（自 ban 等でセッションを壊す footgun 防止）。バッジ等は付けず**行レイアウトは不変**（disabled で灰色化のみ）。`selfUserId` は App→SessionTab→SessionUsers→UserCard と prop で配線。匿名訪問者(id 空)・匿名起動(loginUserId 空)は非該当＝従来どおり全操作可。
- **確認ダイアログ**（`components/ConfirmModal`・ラベルは `common.*`）対象 = kick/ban/respawn/silence/unsilence ＋ save/restart/close。危険(kick/ban/close)は確定ボタン赤。メッセージは入力モーダル、適用はバッチ。
- **データ鮮度**: イベント駆動（マウント/フォーカス変更/操作後/手動 ⟳）＋ `useAsyncAction`（操作→完了後 refetch）。**トースト＝7-7 第1層（§3.11）／自動 poll・Page Visibility＝7-7 残として実装済（§3.13）**。**status と users は B1（commit bdb54be）で `GET /sessions/{idx}/detail`（ExecGroup(focus→status→users)）に集約済**（focus 往復半減・一貫スナップショット）。`/status`・`/users` は部分再取得用に残置。
- **スポーン / インパルスカード（R14・`SpawnImpulseCard`）**: 左カラムのセッション設定カードの下に配置。①アイテムスポーン（URL〔`^res[-\w]*://` scheme 検証・不正/空は[スポーン]無効＋ヒント〕＋active〔既定ON〕/persistent〔既定OFF〕チェックボックス＋[スポーン]）／②ダイナミックインパルス（タグ〔必須〕＋値〔任意・空可〕＋[送信]）。**非破壊操作なので確認ダイアログなし**＝実行→受理トースト（方針A・respawn/message と同格）。`useAsyncAction` で busy/トースト集約。spawn/impulse は users/status を変えないため refetch しない。backend = `POST /sessions/{idx}/spawn`・`/impulse`（§2.4）。コマンド組み立ては `headless.SpawnCmd`/`DynamicImpulseStringCmd`（告知③と共有）。
- **レイアウト**: `components/SplitColumns`（再利用）。**xl(1408px) 未満＝1カラム**（max560・中央）、**xl 以上＝2カラム**（左=設定〔セッション設定＋スポーン/インパルス〕/右=ユーザー、**両パネル560固定**・中央寄せ・ページ全体スクロール）。スクロールバーは `ScrollArea type="hover"`（スマホは hover 無で非表示）。
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
  - **自分（ホスト）への申請/解除/招待を無効化（R2）**: `selfUserId`（`status.loginUserId`）と `u.id` 一致の行は `[申請][解除][招待]` を `disabled`（グレーアウト）。自分に対しては無意味なため。**`UsersBody` 共通で search/focused 両方をカバー**（検索結果に自分が出た場合も無効）。`selfUserId` は App→FriendsTab→ResultList→UsersBody と prop 配線。R3 とグレーアウト方式で統一。
- **すべて確認ダイアログ**（外向き操作・§3.9 方針）。`解除` は danger。`invite` は `POST /sessions/{idx}/invite`（focus 必要・
  `FriendsTab` が focusedIdx を受け取る）。`申請`/`解除`/`招待` の backend は実装済（api.ts ラッパ追加のみ）。
- **検索結果にモデレーション段を追加（R1）**: `UsersBody` の **search のみ**（`showModeration`）2段目に `[✉ メッセージ][BAN][BAN解除]`。
  - **メッセージ** = 入力モーダル（session タブと同方式・`api.messageUser(idx, username, text)`・フレンド宛のみ届く制約は許容）。username 駆動なので常時有効。
  - **BAN** = `banByID <userId>`（`POST /bans/banByID`・全セッションから・在席不要なので検索した非在席者も BAN 可）／**BAN解除** = `api.unban(u.id)`（unbanByID 再利用）。どちらも確認ダイアログ（赤）。
  - **userId 必須**: id 空の行は BAN/BAN解除 を `disabled`（banByID/unbanByID は ID 必須）。self（ホスト）も R2 同様 `disabled`。
  - **再取得なし**: 検索リストは ban/unban で変化しないためトーストのみ（既存の検索行と同じ）。focused 行は据え置き（在席者のモデレーションはセッションタブ）。
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
- 左=config 一覧レール（名前のみ・各行に複製⧉/削除×・選択で右に読込・上部に [＋新規]）／右=エディタカード。狭幅は「config 選択プルダウン＋エディタ」に畳む（`SplitColumns` 流儀）。一覧/タブ行のアクションアイコンは `ROW_ICON_SIZE`(=Button `xs` と同じ 30px) 共有で高さを揃える（C・`components/inspector`）。
- 新規＝**インラインで空名ドラフト**（同梱デフォルト雛形・名前を入れるまで保存無効）／複製＝**インラインでクローン**（名前 `<元>-copy` を初期表示・編集可）／削除（確認）／保存（upsert・dirty 追跡＋未保存ガード）。**名前は ConfigEditor 先頭の編集欄**（D・識別子なので cfg 本文と別管理＝`draftName`）。**名前入力モーダルは廃止**（item4）。タイトルは「編集・作成」固定。
- **リネーム＝Save As**（リネーム API 無し）：名前を別の新名にして保存すると**新規作成し元は残す**（旧名を指すスケジュール等の dangling 参照を回避）。
- **誤上書き防止**：別の**既存 name へ保存しようとしたら上書き確認ダイアログ**（PUT が黙って上書きするため・Save As と一貫）。name は即時バリデーション（無効名は保存抑止・空欄はエラー文言を出さず抑止のみ）。

**(B) アカウント**
- config 毎に任意の `loginCredential`/`loginPassword` 欄（空=中央アカウント注入）。password マスク・空=変更なし。中央アカウント設定自体は設定タブ（次フェーズ）の領分。

**(C) フィールド構成＝v1 同等（基本的に全フォーム化）**
- config トップ（フォーム）：`comment`・`tickRate`・`maxConcurrentAssetTransfers`・`usernameOverride`・`dataFolder`・`cacheFolder`・`logsFolder`・`allowedUrlHosts`（add/remove リスト）・`autoSpawnItems`（カンマ→配列）＋アカウント欄。**点5：常時表示は `comment`（メモ）のみ**とし、他（`tickRate`〜`autoSpawnItems`＋アカウント）は**上級設定（`CollapsibleSection`・既定閉じ）へ畳む**。コンフィグ名（D）はメモと並ぶ基本項目として ConfigEditor 先頭に常時表示。
- 各ワールド（startWorlds[]・タブ・フォーム）
  - 基本：`isEnabled`（タブ有効/無効）・`sessionName`・`description`・`accessLevel`・`maxUsers`・`loadWorldPresetName`＋`loadWorldURL`（**両表示**・スキーマ上両立可・どちらが効くかは Resonite 依存＝URL 優先）・`customSessionId`（**prefix/suffix ビルダー**・`:` 分割/結合・**prefix は中央アカウントの解決済 UserID を自動入力＝R12**・上書き可）。
  - 上級設定（**折りたたみ・既定=閉じ**・R11→**点5で再振り分け**）：`forcedRestartInterval`・`autosaveInterval`（`-1=無効` 注記）・`saveOnExit`・`autoRecover`・`mobileFriendly` の**5項目のみ**。
  - **点5で基本へ繰り上げ**：`tags`（カンマ→配列）・`awayKickMinutes`・`idleRestartInterval`（`-1=無効` 注記）・`autoSleep`・`hideFromPublicListing`・`enableResoniteLink`/`forceResoniteLinkPort`（R13）。`-1=無効` 注記はセンチネル欄が基本(awayKick/idleRestart)と上級(forcedRestart/autosave)に分かれるため**両方に表示**。`customSessionId` は基本のまま。
- **折りたたみ共通コンポーネント（R11）**: `components/inspector/CollapsibleSection`（`title`＋`defaultOpen?`＋`▾/▴`＋Mantine `Collapse`・`aria-expanded`）。設定タブの上級折りたたみ（`AppSettingsSection`）を本コンポーネントに置換（挙動不変）＋ワールド運用群を折りたたみ既定で包む。**点5でコンフィグタブの `GeneralSection`（メモ以外）にも適用**（基本/上級の最終振り分けは点5＝上記）。
- **ワールド削除＝各タブの×（R5）**: ワールドタブを `Group[選択Button][× ActionIcon]`（`ConfigList` 行と同方式・ネストボタン回避）にし、各タブの×でそのワールドを削除（確認ダイアログ）。**最後の1枚は×非表示**（唯一のワールドは削除不可）。下部の「ワールド削除」ボタンは撤去。削除位置に応じてアクティブ index を補正。
- **-1=無効フィールドを必ず数値に（R6）**: `awayKickMinutes`/`idleRestartInterval`/`forcedRestartInterval`/`autosaveInterval` は **未設定なら既定値を表示**（`asNumOr`・既定=スキーマ値 -1/1800/-1/-1）し、**空欄は -1（無効）へスナップ**（`sentinelW`＝map に `""` を書かない）。UI 方式は「数値入力＋一般ヒント `config.sentinelNote`」（トグルは不採用）。入力欄は `SentinelNumberInput`（`min=-1` を一元化・B）。なお数値欄は既定で**整数のみ＋範囲外を入力時点で抑止**（B・`InspectorNumberInput` 既定 `allowDecimal=false`/`clampBehavior=strict`）。
- **customSessionId prefix 自動入力（R12）**: 中央アカウント保存時に `username→UserID` を解決（backend `resonite.ResolveUserID`・`normalizedUsername` 完全一致・メール/未一致は空・§2.x credentials）し `headless-credentials.userId` に保持。設定タブに **UserID を読み取り表示**＋アカウント名 placeholder から「/ メール」削除（解決成功率↑）。config タブは中央 UserID を取得し `CustomSessionIdInput` の `autoPrefix` に渡す＝**prefix が空なら自動シード（上書き可・表示のみ初期化で未編集なら map 未書込・編集時に commit）**。UserID が後着でも `key` 再シードで反映。
- **ResoniteLink 項目（R13）**: `enableResoniteLink`（Switch）＋`forceResoniteLinkPort`（数値・**空＝自動**＝`undefined` で保存JSONから省く・`portW`）。port は `placeholder` で「空＝自動」を示す。当初は運用折りたたみ内だが**点5で基本へ繰り上げ**。
- **温存のみ（UI 非搭載）**：`universeId`・`useCustomJoinVerifier`・`forcePort`・`keepOriginalRoles`・`defaultUserRoles`・各 `*CloudVariable`・`parentSessionIds`・`autoInvite*`・`saveAsOwner`・`overrideCorrespondingWorldId` ＋未知フィールド。（`enableResoniteLink`/`forceResoniteLinkPort` は R13 でフォーム化＝下記）

**安全/堅牢**
- 新規 config の `accessLevel` 既定は **Anyone**（2026-06-03 変更。旧既定は安全側の Private だったが、ユーザー判断で公開既定に変更）。**雛形は公式スキーマ全項目を明示し、UI 表示＝保存値を一致させる方針**（旧来の「表示専用フォールバックで値を見せるが保存JSONにはキーが無い」ズレを排除。未設定は null）。no-config 起動や誤 accessLevel が公開事故になりうる点は domain-facts §7 のとおりで、**起動は config 必須**（無 config 起動ボタンを出さない）でカバーする。決定値の一覧は §3.14 末尾／`configModel.ts` コメント参照。
- **未保存ガード**：dirty 時は **config 切替・新規作成・複製**で破棄確認を挟む（`guardDiscard` で3経路統一）。複製は保存状態（`original`）をクローン。同一 config 内のワールドタブ移動は保存単位が同じため不要。ワールド削除・config 削除は確認ダイアログ。**既知の制限**：コンフィグタブから**他タブへ離脱**すると未保存編集は警告なく失われる（アプリ横断の未保存ガードは未実装＝他タブ方針と整合・MVP 許容）。
- **ワールドは最低1つ**（最後の1枚は削除不可）。
- **稼働中の config 編集は再起動まで未反映**の注記。
- `loginPassword` は常時マスク（タイプした平文を画面に再表示しない）。
- 不正 JSON の config はロード失敗を画面表示（クラッシュしない・一覧には name のみ出る）。

**ワールド検索（将来）**：`loadWorldURL` の検索ピッカーは将来（ワールド検索＝DESIGN Should・将来実装・§3.12）。今は URL プレーンテキスト欄。

**流用部品**：`components/inspector`（InspectorCard/FieldRow/InspectorSelect/InspectorTextInput/InspectorNumberInput/InspectorTextarea/InspectorButton/RefreshButton）・`hooks/useConfirm`＋`ConfirmModal`・`hooks/useAsyncAction`・`lib/notify`（結果トースト自動）・`SplitColumns`。

**バックエンド**：改修ゼロ。GET `/headless-configs`（一覧）・GET `/headless-configs/{name}`（全文・pw マスク）・PUT（upsert）・DELETE。新規雛形はフロントが同梱デフォルト（Anyone・公式スキーマ全項目明示・1ワールド・creds 空）を保持。

### 3.15 設定タブ (7-5) で確定した実装事項

`mrhc.config.json`（アプリ本体設定）を GUI 化するタブ。停止中でも使える（アプリ/ファイル設定系）。
§3.3 の「アプリ設定 / パスワード変更 / Steam(P9)」を具体化。**バックエンドは小規模に新設**（credentials は既存流用）。

**構成＝縦積み4セクション**（`InspectorCard`・単一中央カラム maw560・`ScrollArea`）
1. **管理パスワード変更**：現PW＋新PW＋確認 → `POST /password`。一致/空はクライアント検証（赤テキスト）、現PW誤り等は 7-7 トースト。
2. **Resonite アカウント**（重要）：username/password（空=変更なし）。`config` で個別指定が無いとき各ワールドに注入される既定アカウント。**初回モーダル＋未設定バナーで設定を促す**（下記）。**稼働中はアカウント欄の下に Resonite ログイン状態を1行表示**（commit 48703b5/9d0e92c・「ログイン失敗が ready=true で隠れる」残課題の解消）: `/status` の `loginState`（ヘッドレス起動ログを Driver が解析＝`Logging in as`/`Logged in successfully`、実機 fixtures 由来）から、**ログイン済み（U- 付き UserID・緑）／ログイン失敗〔匿名で動作中・赤〕／匿名〔未設定・灰〕**。MRHC 保存アカウントとは独立に「実際にログインできているか」を表面化する（停止中は非表示）。UserID 表示は必ず `U-` を含める（UserID とユーザー名は別物）。
3. **アプリ設定**：`port`（普通に表示・**MRHC再起動後反映**）・`resoniteHeadlessPath`（**次回ヘッドレス起動で反映**＝`handleStart` が cfg をライブ参照）＋折りたたみ「上級設定」に `headlessConfigDir`（再起動後反映）。**`encoding` は UI 非搭載**（両OS自動判定が実証済・逃げ道は config 手編集）。
4. **DepotDownloader（Steam）設定**：将来枠の disabled プレースホルダ（7-2/7-3 と同じ予約手法・P9-B）。

**初回オンボーディング（App 全体にはみ出す）**
- ログイン後 `GET /headless-credentials` を取得（`App` の `refreshCred`）。**未設定**（username 空 or password 無し）なら **初回モーダルを1回自動表示**（`setupShown` ref・「後で」で閉じ可・強制ブロックなし）＋**常設バナー**「Resonite アカウントが未設定です [設定する]」（`Alert` orange・全タブ表示）。
- バナー文言は**事実のみ**（「公開になる恐れ」等の帰結文は付けない＝条件依存で不正確になるため）。保存で再評価し両方解消。取得失敗（null）はバナーを出さない（一時エラーで誤って煽らない）。

**確定した設計判断**
- **パスワード変更後＝操作中ブラウザは継続**：`AdminPasswordHash` 変更で署名鍵が変わり全トークン失効するが、応答で**新Cookieを再発行**して操作端末だけログイン維持（他端末は失効）。`setSessionCookie` を login と共用。
- **cfg 同期の一本化**：旧 `credMu` を **`cfgMu`（RWMutex）に格上げ**し、全 cfg 書き換え（credentials/password/app-settings）＋読み取り（auth の署名鍵・`handleStart` の creds/パス）を保護。`auth` は `&cfgMu` を共有（レート制限状態は別ロック `auth.mu`）。ロックは入れ子にしない（RLock→Unlock→Lock の順次）。
- **アカウント入力の共通化＝案A**：`AccountForm`（presentational）＋`useCredentialsForm`（状態/保存）を**設定タブと初回モーダルで共用**。**コンフィグタブは据え置き**（ラベルが config キー名で異なる・出荷直後コードを触らない）。password 欄は config タブと同じ `InspectorTextInput type="password"`。
- 新規 UI プリミティブは**バナー（`Alert`）と初回モーダル（`Modal`）のみ**。他は `inspector`/`useAsyncAction`/`lib/notify` を流用。

**バックエンド（新設）** `internal/server/settings.go`
- `POST /api/v1/password {currentPassword,newPassword}`：現PW bcrypt 検証→新ハッシュ保存（ロールバック付き）→新Cookie再発行。
- `GET/PUT /api/v1/app-settings {port,resoniteHeadlessPath,headlessConfigDir}`：port 範囲(1–65535)検証・秘密/encoding 非露出・SaveTo ロールバック。
- 反映: port・configDir＝MRHC 再起動後 / resoniteHeadlessPath＝次回ヘッドレス起動（cfg ライブ参照）。

**検証**：Go 単体（`settings_test.go`＝PW変更の現PW検証/Cookie再発行/旧PW失効・app-settings GET/PUT/不正port/ファイル永続化）＋`go test ./...` green。`npx tsc --noEmit` / `npm run build` green。

### 3.16 スケジュール（自動再起動）タブ (Phase 8) の確定仕様

> 本節は**スケジュールタブの確定仕様＝単一情報源**（§3.14 と同方針）。2026-06-01 のユーザーとの設計協議で確定し、**P8-1〜P8-5b すべて実装・実機検証済（〜commit 7319fbd）＝Phase 8 完了**（5a 状態/手動・5b-1 待機/事前/クラッシュ＋保存バー・5b-2 予定リスト/編集モーダル）。本節は**実装で確定した差分も反映済**（各項に「実装」注記）。親方針: DESIGN §5.4–§5.6・要件 [[rewrite-plan]]。

`nav.ts` の `schedule` を実体化。**バックエンドは新設**（`config` に `restart` 追加・scheduler / restart-orchestrator / crash-monitor の goroutine・restart API）。土台は実装済（`WorldsService.List/ForEach`・`Driver.Start/Stop`・`d.stopping` 意図的停止判別・`recordLastUsed/loadLastUsed`）。

**(0) 協議で確定したスコープ（v1 から削ぎ落とした点）**
- **トリガーは2つ**: 手動「通常再起動」/ scheduled。**userZero 独立トリガーは作らない**＝「全員退出を待つ」は再起動フローの待機段に内包（v1 の userZeroWatcher 常時監視〔複数→0 検知〕は廃止）。**highLoad 不採用**（要件）。
- **手動は「通常（安全再起動）」1ボタンのみ**。即時(forced)は既存の停止→起動で代替できるため作らない。「告知のみ」も MVP では作らない。
- **告知はチャットDMを使わない**（`message` はフレンド限定で到達不確実）。**dynamicImpulse 告知のみ**（フル設定型）。
- **待機制御はグローバル設定のみ**（予定ごと override は持ち込まない＝YAGNI）。

**(1) 統一フロー「安全再起動」（手動通常・scheduled 共通）**
```
[トリガー] 予定時刻 到達 / 手動「通常再起動」
   ↓
在席者0人？（ΣPresent・ホストは Present:False で自然除外） ──YES──→ 即時再起動（①②③スキップ・告知も待機も無し）
   │ NO
   ↓
① 即発火: セッション変更（Private化 / maxusers=1 / 改名）＝新規参加を静かに止める
   ↓
② 静かに待機（2区間モデル R9・締切 = quietWaitMin + announceWaitMin＝既定 58+2=60分・worlds の在席者(Present)合計を監視）
   │   └─ 待機中に在席0人化 → 即再起動（③告知を待たない）
   ↓ quietWaitMin（既定58分）経過＝告知ライン（締切の announceWaitMin 前）に来てもまだ居る
③ 告知: dynamicImpulse を1回発信「まもなく再起動します」
   ↓ announceWaitMin（既定2分）経過＝締切
④ 強制再起動 → 停止 →（任意Steam=P9-B）→ 選択 config で起動
```
境界条件・決定:
- **人数指標＝在席者(Present)合計**（R0・`orchestrator.presentUserCount`＝`Σ worlds[].Present`）。ホスト（ヘッドレス自身）は実機採取(2026-05-28: ホストのみ=Users:1/Present:0)で `Present:False` のため自然に除外される（"Users−ワールド数" のような補正は不要）。「接続中だが在席でない人（ユーザースペース等）」は在席0扱い＝即時/早期再起動の対象になる点は許容（即時分岐は無告知のため、この指標選択は方針として明示・採用）。表示（TopBar/SessionTab は Current＝ホスト込み）とは指標が異なる。
- **セッション変更（①）はトリガー時に即発火**（新規参加を静かに止める）。**dynamicImpulse 告知（③）だけが締切前**＝静かに待ち、強制が近づいた時だけ告知する運用。
- **二重起動防止**: 再起動進行中フラグでトリガーを排他（手動/scheduled/crash 同時を排他）。
- **再起動の config** = 予定/手動で選択（既定は空文字 `""` = 直近起動と同じ config。`configName` を指定すればその config で起動）。
- 待機は **2区間モデル（R9）**＝`quietWaitMin`（告知前に静かに待つ）＋`announceWaitMin`（告知後に待つ）。締切 = 両者の和。告知は `quietWaitMin` 経過時点に1回。2区間は互いに独立（旧 `forceRestartTimeout`/`actionTiming` の「告知≦強制」相互依存を撤廃＝検証不要）。各 0〜1440 分。
- **キャンセル**: 進行中（①②③＝停止④の前）のみ「中止」可能。中止すると再起動を取りやめ、ヘッドレスは稼働継続。**即発火済みのセッション変更（Private化等）は自動で戻さず、必要なら管理者がセッションタブで手動復元**（UI に「設定は変更されたまま」と添える）。scheduled をキャンセルしても**今回分のみ**＝次回は通常発火（予定は無効化しない）。④停止開始後は中止不可。
- **実装（レビュー反映）**: ②待機中にヘッドレスが落ちた（クラッシュ/外部停止）場合は、締切（最大60分）を待たず**即 ④ で復帰**（Stop は空振り・Start で起動）。この間 crash-monitor は「進行中」を見て手を出さない（orchestrator が所有）。

**(2) 事前アクション**
- **dynamicImpulse 告知（フル設定型）**: 各ワールドに `ForEach` で `spawn "<itemUrl>" true false` → `dynamicimpulsestring "<tag>" "<message>"`（R14 で `headless.SpawnCmd`/`DynamicImpulseStringCmd` 純関数に統一＝セッションの spawn/impulse 書き込み API と同一組み立て。spawn は help 確定の3引数〔旧 `spawn <url> true` の2引数を修正〕・告知アイテムは一時的なので persistent=false）。`itemUrl` / `tag`（例 `MRHC.play`）/ `message` を UI 設定。**固定文**（残り時間差し込み変数なし）。**最終ウィンドウで1回**（カウントダウン繰り返しは将来）。⚠️ dynamicImpulse はワールド側に受け機構が必要＝spawn したアイテムに impulse を送る v1 方式を踏襲（フル設定型ゆえ受け側 tag に合わせられる）。
  - **実装**: **spawn → impulse の間に約10秒待機**（spawn したアイテムがワールド内で実体化してから impulse を送る・v1 `ITEM_SPAWN_DELAY` 踏襲・固定定数）。**2パス**で実行（全ワールド spawn → 10秒 sleep → 全ワールド impulse）。待機中は execMu を解放し他コマンドを妨げない。`itemUrl` 空なら spawn を省略し impulse のみ（常設受け機構前提）。
  - **実装（UI・5b-1 追補）**: `itemUrl` は **テンプレ選択（v1 main 由来の2種＝とらぞセッション閉店アナウンス/テキスト読み上げ）＋手動入力** の `Select`。テンプレ選択で `itemUrl`＋**共通タグ `MRHC.play`**（`ANNOUNCE_COMMON_TAG`）を自動設定し URL/タグ欄を隠す。手動入力時のみ URL/タグ欄を表示。**`tag` は必須・`message` は任意（空可＝受信アイテムが固定内容でメッセージを使わない場合がある。`Restart.Validate` も message を必須にしない）**。告知「有効」OFF 時は配下欄を非表示。
- **セッション変更**: 各ワールドに `ForEach` で `accesslevel Private`（setPrivate）/ `maxusers 1`（setMaxUsersOne）/ `name "<renameTo>"`（rename）。**トリガー時に即発火**。再起動後は config から再ロードされ名前等は戻る。
  - **各項目は独立トグル（全OFF可）**＝「Private化はしたくない」なら setPrivate=OFF のままにできる。既定は **maxusers=1 のみ ON・Private と改名は OFF**。

**(3) scheduled 時刻モデル（独自・once/weekly/daily）**
- `type: "once" | "weekly" | "daily"`。**サーバーローカル時刻**で判定し UI に適用 TZ を明示。
- once = 年月日時分／weekly = 曜日(0=日..6=土)+時分／daily = 時分。各予定に `enabled`・`configName`（**空 `""` = 前回 / 非空 = その config 名**）。※ config 名は `_` を含み得る（`__previous__` すら有効名）ため、前回の番兵は衝突しない**空文字**を使う。
- **次回再起動時刻** = 全有効予定の次回発火時刻のうち最も近いもの（restart-status で公開）。判定はスケジューラ goroutine が自前計算（cron ライブラリ不要）。

**(4) クラッシュ自動復帰（DESIGN §5.6）**
- **既定ON**。意図しないプロセス終了（`d.stopping==false` での終了）を検知 → ヘッドレスを直近 config で自動再起動。
- **ループ保護**: 既定「**10分に3回**」でクラッシュしたら自動復帰を停止し通知（無限再起動ループ防止）。`maxCrashes`/`windowMinutes` を設定可。
- 復帰対象はヘッドレスのみ（MRHC 自身は手動起動）。

**(5) 設定データ構造（`mrhc.config.json` の `restart`）**
```jsonc
"restart": {
  "scheduled": [
    { "id":"...", "enabled":true,  "type":"daily",  "hour":5, "minute":0, "configName":"" },              // ""=前回config
    { "id":"...", "enabled":true,  "type":"weekly", "weekday":1, "hour":4, "minute":0, "configName":"night" },
    { "id":"...", "enabled":false, "type":"once",   "year":2026,"month":6,"day":10,"hour":3,"minute":0, "configName":"" }
  ],
  "waitControl":  { "quietWaitMin": 58, "announceWaitMin": 2 },   // 2区間（R9）: 締切 = quiet+announce
  "preActions": {
    "announce":       { "enabled":true, "itemUrl":"resrec:///...", "impulseTag":"MRHC.play", "message":"まもなく再起動します" },
    "sessionChanges": { "setPrivate":false, "setMaxUsersOne":true, "renameEnabled":false, "renameTo":"" }
  },
  "crashRecovery": { "enabled":true, "maxCrashes":3, "windowMinutes":10 }
}
```
`cfgMu`（既存 RWMutex）で読み書きを保護（設定タブと同じ一本化方針・§3.15）。

**(6) API / SSE**
```
GET  /api/v1/restart-config                  restart 設定を返す
PUT  /api/v1/restart-config                  保存（cfgMu 保護・SaveTo ロールバック）
GET  /api/v1/restart-status                  次回予定 / 最終起動 / 稼働時間 / 進行状態 / クラッシュ復帰状態
POST /api/v1/restart/trigger {configName?}   手動「通常再起動」を即受付（非同期）。空=前回config
POST /api/v1/restart/cancel                  進行中の再起動を中止（①②③のみ・ヘッドレスは継続。通常停止の中止にも共用）
POST /api/v1/stop/graceful                   通常停止を即受付（非同期・R7）。事前アクション→固定2分→停止（起動しない）
```
- **通常停止（R7）**: TopBar ⋮ の「通常停止」（強制停止の隣）から**無確認で即受付**（受理トースト「約2分後に停止・スケジュールタブで中止可」）。強制停止が無確認なのに揃え、かつ通常停止は2分猶予＋中止可で誤操作も復旧可能なため確認ダイアログは挟まない。orchestrator の統一フローを**終端=停止**で流用＝0人なら即停止／居たら ①セッション変更→③告知（即時）→固定2分→④停止（**起動しない・最終起動も記録しない**）。待機制御（②）は再起動の長時間設定ではなく**固定**（告知前0分＋告知後2分）。進行（待機/告知/`停止中`＋残り時間）と中止はスケジュールタブの状態カードに表示（`restartTriggerType="stop"` で終端フェーズを「停止中」表示）。事前アクション（セッション変更・告知）は再起動と同じ `preActions` 設定に従う（告知 OFF なら無告知＝セッション変更＋2分猶予のみ）。
- **進行状態の伝送はポーリング（実装・P8-5 で確定）**: UI が `restart-status` を `useVisiblePolling` で3秒ごと追従（表示中のみ）。restart-status は `inProgress`/`phase`(idle|preparing|waiting|announcing|restarting)/`restartTriggerType`/`restartConfigName`/`deadlineAt`/`lastStartAt`/`lastStartTrigger`/`nextScheduled*` を返す。SSE への進行フェーズ反映は将来拡張（MVP 非採用）。

**(7) UI 構成（スケジュールタブ・インスペクタ風・停止中も編集可）**
`SplitColumns` で配置。**広い画面（xl以上）は2カラム＝左に運用/状態〔①②③〕・右に設定〔④⑤⑥〕／狭い画面は縦1カラム**にカードを積む:
1. **ステータスカード**: 稼働時間・**次回予定再起動**（日時+config）・最終起動（時刻/トリガー種別・手動起動を含む全起動）・現在の進行状態・クラッシュ復帰状態。**再起動進行中は現在フェーズ（待機/告知）＋残り時間＋`[中止]`ボタンを表示**（中止後は「セッション設定は変更されたまま」と一言添える）。稼働時間/進行は稼働中のみ。
2. **手動カード**: `[通常再起動]` ボタン（`ConfirmModal` 確認・config 選択付き〔既定=前回〕・**稼働中のみ有効**・ボタン色は `severity="warning"`＝セッションタブの再起動と同色）。
3. **予定リストカード**: 各行（有効トグル / 種別・時刻 / config / 編集✎ / 削除×〔**直接削除**＝未保存ゆえ保存前は取り消し可〕）＋`[＋新規]`・空時「予定がありません」。編集は**モーダル**（type 選択で日時欄を出し分け〔once=年月日+時分/weekly=曜日+時分/daily=時分〕・config `Select`〔#prev 番兵〕・ドラフト→`[OK]` で working 配列へ反映/`[キャンセル]` 破棄）。
   - **実装**: 新規 id は `scheduleModel.genId()`（`crypto.randomUUID` はセキュアコンテキスト限定＝LAN/HTTP で不可のため `getRandomValues`→`Math.random` にフォールバック・[[lan-http-no-secure-context-apis]]）。**インライン検証**（時/分の範囲＋once 暦実在＝JS Date 往復で 2/30 等を弾き [OK] 無効化、年下限 `MIN_YEAR=2000` は backend 準拠）。daily/weekly へ切替時は once 専用の year/month/day をクリアして保存JSONを汚さない。日付入力は `@mantine/dates` 不使用で `NumberInput`。**once の年月日はスマホ向けに縦並び（R8）**＝full-width 入力を縦 Stack に積み、各入力の右に `年/月/日` 単位ラベル（固定幅で右端整列）。狭幅でも横はみ出しなし。時刻（時:分）は横並びのまま。
4. **待機制御カード**（2区間モデル R9）: `quietWaitMin`（静かに待つ）/ `announceWaitMin`（告知後に待つ）。各 0〜1440・相互依存なし。
5. **事前アクションカード**: 告知（有効 / itemUrl〔テンプレ選択+手動入力〕 / impulseTag / message）＋セッション変更（Private化 / maxusers=1 / 改名+名前）。**告知 OFF 時は配下欄、改名 OFF 時は名前欄を非表示**（条件レンダリング）。message は任意（空可）。詳細は §3.16(2) 実装注記。
6. **クラッシュ復帰カード**: 有効トグル / maxCrashes / windowMinutes。
- 流用部品: `components/inspector`・`useAsyncAction`・`useConfirm`＋`ConfirmModal`・`lib/notify`・`SplitColumns`。
- 停止中: 全設定編集可（`schedule` は `availableWhenStopped`）。手動再起動ボタンのみ停止中は無効。
- **保存モデル（実装・P8-5 で確定）**: 設定群（③④⑤⑥）は**単一 working オブジェクト＋一括保存バー**＝dirty 判定し**完全オブジェクトを `PUT /restart-config`**（backend の pointer 設計に一致・コンフィグタブと同方式）。①状態・②手動は live（poll＋アクション）で保存対象外。手動 config Select は空値を扱えないため番兵 `#prev`（config 名に無効な `#`）を使い送信時に `""`（=前回）へ変換。
- **実装状況**: 全カード実装済（`web/src/tabs/schedule/`）。①②（状態カード・手動再起動）＝P8-5a。③予定リスト＋編集モーダル＝P8-5b-2。④⑤⑥＋保存バー＝P8-5b-1。**＝Phase 8 完了**。

**(8) バックエンドアーキ（実装=案A mutex モデル）**
- **scheduler goroutine**（`scheduler.go`）: 有効予定から次回発火時刻（`NextScheduled`）を算出し待機。**config 変更は `Reload()` シグナル**（バッファ1の非ブロッキング channel・restart-config PUT 後に呼ぶ）で即再計算（ポーリングしない）。strictly-future ゆえ同分の二重発火なし。発火で `orchestrator.Trigger("scheduled", configName)`。非稼働/進行中は Trigger の err を log して継続（停止中 skip）。
- **restart orchestrator**（`orchestrator.go`）: 手動/scheduled のトリガーを受け統一フロー（(1)）を実行。`inProgress` で排他。`WorldsService.List`（人数）・`ForEach`（事前アクション）・`Driver.Stop/Start` を使用。**config 解決（name→launchPath＋認証注入）は `handleStart` と共通化＝`resolveLaunch` に抽出**。
- **crash monitor**（`crashmonitor.go`）: Driver に **`SetOnUnexpectedExit` コールバック**を追加し、`waitExit` の `d.stopping==false`（管理外の異常終了）時のみ**非ブロッキング通知**（誤検知ゼロ）。受信側で enabled 確認→進行中なら skip（orchestrator 所有）→rolling window でループ保護→直近 config で復帰。
- **並行モデル=案A（mutex）**: 進行状態を小さな mutex で保護し、flow は goroutine、cancel は context（既存 cfgMu/driver.mu と同流儀）。仕様当初の「channel 所有の単一 goroutine」は **flow が最大60分のブロッキング I/O を抱えるため見送り**（worker 分離が結局必要で複雑化）。実コマンドの直列化は driver の execMu が担う。`restart-status` は GET（UI はポーリング）で公開。
- **常駐ライフサイクル**: `Server.Start()` が scheduler・crash-monitor を bg ctx で起動し stop 関数を返す。`main` が SIGINT で `driver.Stop()` の前に stop()（予定発火を打ち切り・`orchestrator.setParent(bgCtx)` 経由で進行中の①②③を cancel）。

**(9) 状態永続化**（実装・R10 で「最終起動」に拡張）: **最終起動（`lastStartAt`／`lastStartTrigger`=manual|scheduled|crash）のみ** `runtime-state.json` に追記する。recordLastUsed が消さないよう **read-modify-write で `lastUsedConfig` と共存**させ、`runtimeMu` で直列化（orchestrator/crash-monitor〔goroutine〕と handleStart〔HTTP〕の並行書き込み防止）。**あらゆる起動成功で1回記録**＝手動起動（`handleStart`・trigger=manual）／手動再起動・予定再起動（orchestrator・再起動/予定は `driver.Start` を直接呼ぶため二重記録なし）／クラッシュ復帰（crash-monitor・trigger=crash）。**次回スケジュール再起動は予定から都度算出・稼働時間は driver の `StartedAt` 由来＝どちらも永続化せず導出**。

**(10) 既知の制約 / 将来拡張**
- dynamicImpulse の到達はワールド側受け機構に依存（フル設定型で吸収）。
- 将来候補: 告知カウントダウン繰り返し・「告知のみ」手動ボタン・即時再起動ボタン・残り時間差し込み変数・予定ごと待機 override・Steam 更新を④に挟む（P9-B・DESIGN §5.7）。

---

## 4. 後で詳細協議が必要な領域
- ~~**スケジュールの再起動条件**（モジュール式設計・待機制御 waitControl）← Phase 8 着手前~~ → **確定（§3.16）**
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
