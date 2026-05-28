# Phase 7 (フロントエンド統合) 仕様書

> ステータス: **着手前 / 仕様確定**。Phase 7 着手前に write API 実装を挟む。
> 親設計: [docs/DESIGN.md](../DESIGN.md)
> 関連: [docs/design/structured-driver.md](structured-driver.md), [docs/resonite-domain-facts.md](../resonite-domain-facts.md)

## 1. 着手順序

```
Phase 6 完了 (現在地)
   ↓
Pre-Phase 7: write 系 API 実装
   ↓
Phase 7: UI 実装
```

## 2. Pre-Phase 7: write API 仕様 (RESTish パターン)

### 設計方針
- **P-A RESTish**: URL に動作を明示
- POST のみ（GET は read-only）
- 全て認証必須
- ボディは JSON（form-urlencoded 非対応）

### エンドポイント一覧

#### セッション内のユーザー操作
```
POST /api/v1/sessions/{idx}/users/{user}/kick
POST /api/v1/sessions/{idx}/users/{user}/ban
POST /api/v1/sessions/{idx}/users/{user}/silence
POST /api/v1/sessions/{idx}/users/{user}/unsilence
POST /api/v1/sessions/{idx}/users/{user}/respawn
POST /api/v1/sessions/{idx}/users/{user}/role     body: {"role": "Admin"}
```

#### セッション設定
```
POST /api/v1/sessions/{idx}/accesslevel           body: {"level": "LAN"}
POST /api/v1/sessions/{idx}/maxusers              body: {"maxUsers": 8}
POST /api/v1/sessions/{idx}/name                  body: {"name": "..."}
POST /api/v1/sessions/{idx}/description           body: {"description": "..."}
POST /api/v1/sessions/{idx}/hidefromlisting       body: {"hide": true}
```

#### セッションライフサイクル
```
POST /api/v1/sessions/{idx}/restart
POST /api/v1/sessions/{idx}/save
POST /api/v1/sessions/{idx}/close
```

#### セッション内アクション
```
POST /api/v1/sessions/{idx}/invite                body: {"user": "..."}
POST /api/v1/sessions/{idx}/message               body: {"user": "...", "message": "..."}
POST /api/v1/sessions/{idx}/spawn                 body: {"url": "...", "active": true, "persistent": false}
POST /api/v1/sessions/{idx}/impulse               body: {"type": "string"|"int"|"float"|"bare", "tag": "...", "value": "..."}
```

#### Bans / Friends
```
POST /api/v1/bans/{user}/unban
POST /api/v1/friendrequests/{user}/accept
POST /api/v1/friends/{user}/remove
POST /api/v1/friends                              body: {"user": "..."}  (送信)
```

### 実装方針
- 各ハンドラは `Driver.Exec` または `ExecGroup` を使う
- focus 必要な操作は `ExecGroup` でアトミックに
- レスポンス: `writeOK(w, map[string]any{"sent": true})` または応答が parse 可能なら parse 結果
- エラー: `writeExecErr` で `ErrNotReady→409` / その他→500
- パス引数バリデーション: parseSessionIdx 拡張 + ユーザー名検証

### Pre-Phase 7 ToDo
1. write 系ハンドラ実装 (~20 endpoints)
2. fakehl 拡張 (write 系コマンドのモック応答)
3. 統合テスト追加 (各エンドポイントの正常系 + エラー系)
4. 想定する Resonite レスポンス書式は `docs/resonite-domain-facts.md` 参照

---

## 3. Phase 7 UI 仕様

### 3.1 技術スタック (確定)
- **React 19** + **TypeScript** + **Vite 6**
- **Mantine v7** (UI ライブラリ)
- **react-i18next** (ja + en 両方維持)
- 既存 `web/` ディレクトリで継続開発

### 3.2 レイアウト
- **Mantine AppShell** をベース
- **PC**: 左サイドバー + メインコンテンツ
- **スマホ**: ハンバーガーメニュー (自動切替)
- メインエリアは**縦並べ・折りたたみ可能セクション**

```
[PC 表示]                          [スマホ表示]
┌────┬─────────────────┐          ┌──────────────────┐
│MENU│ MRHC [起動中]   │          │☰ MRHC [起動中]   │
│    │                 │          ├──────────────────┤
│ D  │ Sessions        │          │ Sessions          │
│ B  │  - Sess1        │          │  - Sess1          │
│ F  │                 │          │                   │
│ ⌨  │ Selected:       │          │ Selected:         │
│    │  Status...      │          │  Status...        │
│    │                 │          │                   │
│    │ Log (collapse)  │          │ Log (collapse)    │
└────┴─────────────────┘          └──────────────────┘
```

### 3.3 サイドバーナビ (MVP)
1. **Dashboard** (デフォルト) — sessions 一覧 + 選択中 session 詳細 + 主要 write ボタン
2. **Bans** — listbans + unban
3. **Friends** — friendrequests + accept
4. **Raw Command** — 上級者用 stdin 直送

### 3.4 カラーパレット (Resonite 公式)
**dark 固定**（light は実装しない）

| 用途 | 色 | hex |
|---|---|---|
| 背景 (Dark) | ⬛ | `#11151d` |
| カード/サイド (Mid) | ⬛ | `#2b2f35` |
| ラベル/補助 (Mid-Light) | ⬛ | `#86888b` |
| 本文 (Light) | ⬜ | `#e1e1e0` |
| 主アクション (Cyan) | 🟦 | `#61d1fa` |
| 正常/起動中 (Green) | 🟩 | `#59eb5c` |
| 注意 (Yellow) | 🟨 | `#f8f770` |
| 強調/通知 (Orange) | 🟧 | `#e69e50` |
| 危険/停止 (Red) | 🟥 | `#ff7676` |
| 二次/上級 (Purple) | 🟪 | `#ba64f2` |

### 3.5 状態とフロー

#### ログイン
- **シンプルカード** (中央に MRHC ロゴ + パスワード input + ログインボタン)
- 失敗時はカード内に赤エラー表示
- 連続失敗 10 回でロック (現状の auth ロック仕様)

#### 初回セットアップ
- **CLI 対話のまま** (端末でパスワード/ポート/ヘッドレスパス設定)
- Web ベース wizard は実装しない (admin が一度だけやる作業)

#### セッション期限
- **90日** (Cookie TTL)
- 現状の 24h からの大幅延長。LAN ツールとしての利便性重視

#### ヘッドレス未起動時
- ダッシュボードに **「ヘッドレスを起動」ボタン**を出す
- ボタン押下 → `/api/v1/start` → SSE で進捗確認
- 起動後は通常のダッシュボードに

### 3.6 主要 UI コンポーネント
- **AppShell.Navbar**: ナビアイテム (Dashboard/Bans/Friends/Raw Command)
- **AppShell.Header**: タイトル + 状態バッジ + ログアウト
- **Sessions テーブル**: index, name, users, present, accessLevel, maxUsers (各行クリックで詳細)
- **Session 詳細パネル**: status + users + 操作ボタン (kick/ban/role/accesslevel/maxusers/name/restart)
- **Bans テーブル**: ban entry + unban ボタン
- **Friends リスト**: friend request 名 + accept ボタン
- **Raw Command フォーム**: textarea + 送信ボタン (上級者用)
- **Live Log**: SSE で流れる、折りたたみ可能
- **確認モーダル**: 危険操作 (kick/ban/shutdown) で確認ダイアログ
- **トースト通知**: 操作完了/失敗

---

## 4. 確定済み設計選択（参考）

### 4.1 採用
- **構造化 Console Driver** (executor.go 分離、案C'+案X)
- **ExecGroup** (原子的グループ、focus 競合防止)
- **WorldsService.ForEach** (巡回機構の土台、事前アクション等で使用予定)
- **per-line `stripLineLeadingPrompts`** (prompt-glue 対応)
- **`safeStripLeadingPrompts`** (friendrequests 特化)
- **ParseFriendRequests** (v1 互換、実機 format で確証済)

### 4.2 撤去
- **HeadlessBackend 抽象** (Crystite/mod 不採用 + Resonite API 提供予定なしのため概念負債)
- **isLikelyUsername** (ヒューリスティック判定、実機データなしで設計すべきでない)
- **/command form-urlencoded body** (利用者ゼロ、URL query / JSON で代用)
- **writeExecErr 5区分** (2区分に簡素化、詳細は error.code で識別)

### 4.3 維持
- **WorldsService.List()** (シンプル軽量、userZero 検知で使用)
- **fakehl** (テスト基盤、22 統合テストで稼働)
- **sentinel エラー** (ErrNotReady/Timeout/ProcessGone/Canceled)
- **Encoding 抽象** (Win=Shift_JIS / Linux=UTF-8 実機確証済)

## 5. 実機検証済 fixtures

| 採取日 | 場所 | 内容 |
|---|---|---|
| 2026-05-28 | `fixtures/2026-05-28-windows-multiworld.log` | 複数ワールド・コマンド網羅 |
| 2026-05-28 | `fixtures/2026-05-28-lan-login/01-lan-joined-anonymous/` | LAN join + 別PC参加 |
| 2026-05-28 | `fixtures/2026-05-28-lan-login/02-logged-in/` | login 入り + listbans + help 全コマンド |

friendrequests の実 format も `MARKNPC_SUB2` アカウント手動採取で確証済 (匿名化済テスト追加)。
