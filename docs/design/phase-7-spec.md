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

### 2.3 Headless Config CRUD API（v1 同等・作り直し）
```
GET    /api/v1/headless-configs            一覧
GET    /api/v1/headless-configs/{name}     読み込み
POST   /api/v1/headless-configs/{name}     保存（生成含む）
DELETE /api/v1/headless-configs/{name}     削除
GET    /api/v1/headless-configs/last-used  前回起動コンフィグ
```
- 保存先 = ヘッドレスの Config ディレクトリ配下の `*.json`
- 生成 = HeadlessConfig スキーマ準拠の JSON を programmatic に構築
- 前回起動コンフィグ = 起動時に記録（runtime-state 相当）

### 2.4 write API（RESTish / idx ベース + ExecGroup）

**設計方針（モデル A 確定）**
- URL に idx を持つ idx ベース。各操作は `ExecGroup(focus idx → cmd)` でアトミック実行
- フォーカスは UI 表示・既定対象としてのみ扱い、API はフォーカス状態に依存しない（背景タスクと競合しない）
- POST のみ、全て認証必須、ボディは JSON

```
# セッション内ユーザー操作（セッションモデレーション）
POST /api/v1/sessions/{idx}/users/{user}/kick
POST /api/v1/sessions/{idx}/users/{user}/ban
POST /api/v1/sessions/{idx}/users/{user}/silence
POST /api/v1/sessions/{idx}/users/{user}/unsilence
POST /api/v1/sessions/{idx}/users/{user}/respawn
POST /api/v1/sessions/{idx}/users/{user}/role        {"role":"Admin"}
POST /api/v1/sessions/{idx}/users/{user}/message     {"message":"..."}   # 個別DM

# セッション設定
POST /api/v1/sessions/{idx}/accesslevel              {"level":"LAN"}
POST /api/v1/sessions/{idx}/maxusers                 {"maxUsers":8}
POST /api/v1/sessions/{idx}/name                     {"name":"..."}
POST /api/v1/sessions/{idx}/description              {"description":"..."}
POST /api/v1/sessions/{idx}/hidefromlisting          {"hide":true}

# セッションライフサイクル
POST /api/v1/sessions/{idx}/restart
POST /api/v1/sessions/{idx}/save
POST /api/v1/sessions/{idx}/close

# 新規セッション（ランタイム起動）
POST /api/v1/sessions/start                          {"mode":"url","url":"..."}
                                                     {"mode":"template","template":"..."}
# url      → startworldurl "<url>"
# template → startWorldTemplate <name>
# search 方式は Phase 9（world-search→URL→start）

# Bans / Friends
POST /api/v1/bans/{user}/unban
POST /api/v1/friendrequests/{user}/accept
POST /api/v1/friends/{user}/remove
POST /api/v1/friends                                 {"user":"..."}   # 申請送信
POST /api/v1/sessions/{idx}/invite                   {"user":"..."}   # フォーカス中へ招待
```

### 2.5 操作結果の扱い（重要・実機制約）
Resonite の write 出力は **コマンドごとにバラバラで信頼できない**:
- kick/silence/respawn → UniLog スタックトレース混じりの複数行（正常時でも）
- maxusers/name/focus → **silent（出力なし）**
- invite → 出力不確定
- accesslevel/role → きれいな成功書式（パース可）

→ **方針 A 確定**: ノイジー出力をパースせず、「プロンプト `>` 復帰＝コマンド完了＝成功扱い」。直後に該当データ（users / worlds 等）を再取得して**実状態で結果を見せる**。UI はトースト「実行しました」+ 再取得。

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
- per-line `stripLineLeadingPrompts` / friendrequests 特化 `safeStripLeadingPrompts`
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
