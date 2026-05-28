# 構造化 Console Driver — 詳細設計

> ステータス: **2026-05-28 Windows 実機採取で妥当性検証済**（多ワールド・各コマンド網羅）。承認後に実装着手。
> 親設計: [docs/DESIGN.md](../DESIGN.md) §5 ドメインモデル
> ドメイン事実: [docs/resonite-domain-facts.md](../resonite-domain-facts.md)
> 実機採取fixture: [scripts/empirical-capture/fixtures/2026-05-28-windows-multiworld.log](../../scripts/empirical-capture/fixtures/2026-05-28-windows-multiworld.log)

## 1. 目的・スコープ

公式 Resonite ヘッドレスの stdout/stdin を介して **構造化コマンドの送信＋応答取得＋パース** を行い、ダッシュボードに `worlds` / `status` / `users` / `listbans` / `friendrequests` などの**構造化データ**を供給する。**ライブログ配信は維持**したまま（現行の log SSE と両立）。

スコープ外:
- **ライフサイクル**: 起動/停止（既存 `Driver.Start/Stop`）／プロセスI/Oの低層（既存 `readPipe`／encoding）はそのまま利用。
- ⚠️ **終了系コマンド（`shutdown` / `restart` / `close`）は Exec の対象外**。理由：Exec はプロンプト復帰を完了シグナルにするが、これらはプロセス（or 当該世界）が消えてプロンプトが返らないため必ず timeout になる。`shutdown` は既存 `Driver.Stop()`（fire-and-forget＋プロセス終了監視）に任せる。`restart`/`close` は今後ハンドリング設計時に明示的に「Exec ではない経路」で扱う。

## 2. 配置と既存実装との関係

```
internal/headless/
  driver.go            … 既存：Start/Stop/log配信/SendCommand(fire-and-forget)
                        + 構造化APIメソッド（Exec/ExecGroup）を合流
  executor.go [新規]   … 構造化実行ロジック（直列キュー＋応答コレクタ＋完了検出）
                        ※ Driver 本体にメソッドを生やすが、ロジックの実体は別ファイル
  parser.go   [新規]   … コマンド毎のパース関数（プロンプト接頭辞除去後に適用）
  worlds_service.go [新規] … List() / ForEach() — 巡回の共通化
  hub.go               … 既存：汎用ブロードキャスタ
```
- **方針確定（A）**: `Driver` に Exec/ExecGroup メソッドを**合流**（別オブジェクトに分離しない）。
  - 理由：pipe/Wait/readPipe/状態（ready/exit/encoding）を1箇所で持つほうがシンプル。状態二重管理を避ける。
  - テスト性は内部ロジックを `executor.go` に切り出すことで担保（Driver は薄いラッパー）。
- 既存 `Driver` の `logHub`（ライブ配信）はそのまま利用。
- `readPipe` を拡張し、**「全行を logHub に push」と並行して「アクティブな応答コレクタにも push」**する。コレクタは Exec の入口で set → 出口で unset、`sync/atomic` か mutex で安全に。直列キューにより同時にactiveなコレクタは1つだけ。

## 3. コアモデル

### 3.1 直列キュー（mutex）
構造化コマンドは**1つずつ排他実行**。複数の同時要求はキューで順番待ち。理由：ヘッドレス側が stdin を逐次処理するため、こちら側で直列化するのが最も単純で正しい（並行送信のメリットはほぼ無い）。

### 3.2 ライブログ配信は独立・常時
全 stdout 行は実行中であっても `logHub` に流れ続け、SSE 経由でブラウザのログビューに即反映される。**構造化実行は UI 操作をブロックしない**。

### 3.3 応答コレクタ
構造化実行中だけ有効な「応答収集バッファ」を活性化し、コマンド送信後の出力行を蓄積。完了検出で確定し、プロンプト接頭辞を除去してパーサへ渡す。

## 4. 完了検出（案C': 汎用プロンプト末尾検出）

### 4.1 動機
応答時間は **コマンド種別**（即時〜30秒以上）と **負荷**（賑わう世界はラグる）で大きく変動するため、**固定 settle や固定 timeout だけでは不適**。プロンプト `<world>>` は「次入力待ち」の**確定シグナル**で timing/負荷に非依存。

### 4.2 疑似コード
```
fn Exec(cmd):
    sendBytes = encode(cmd + "\n")
    acquire(mu)
    set activeCollector = new()
    write(stdin, sendBytes)
    started_at = now
    last_change = now
    loop:
        // 読み足し（短い待機）
        chunk = readChunk(timeout=20ms)
        if chunk:
            activeCollector.append(chunk)
            last_change = now
        pending = activeCollector.bytesAfterLastNewline()
        if pending ends with ">" AND (now - last_change) >= 50ms:
            break            // 完了
        if (now - started_at) >= cmdMaxTimeout:
            break            // タイムアウト（フラグ付き）
    clear activeCollector
    release(mu)
    lines = collectedLines (with prompt prefix stripped from first line)
    return lines, timeoutFlag
```

### 4.3 cmd 毎の最大 timeout（既定）
- `worlds`/`status`/`users`/`listbans`/`friendrequests`/`accesslevel`/`role`/`invite`/`kick`/`ban`/`name`/`maxusers` … **3〜5 秒**
- `focus` … **2 秒**
- `startworldurl` … **60 秒**
- 上記は設定可能（config 経由で上書き可）

### 4.4 ambient（無関係ログ）の扱い
応答収集中も ambient 行（入退室等）は届きうるが：
- ambient 行は**必ず `\n` で終わる**ので、`pending`（最後の `\n` より後ろ）には含まれない → **完了検出を構造的に乱さない**（行末に `>` を含む ambient があっても無問題）
- 収集行に混じるが、**パーサが該当行（正規表現に一致するもの）のみ拾う**ので無視される

### 4.5 エッジケース
| ケース | 対処 |
|---|---|
| 改行が来ない長行 | timeout で切上げ＋ログ |
| プロンプトが来ない（コマンドがプロンプトに戻らない） | timeout で切上げ |
| `focus`/`name` でプロンプト自体が変わる | 「世界名」を見ず「`>` で安定」の汎用判定なので問題なし（**実機検証済**） |
| 起動直後の "Unknown command" | 構造化コマンドは **ready 後のみ**実行（呼び出し側でゲート） |
| 確認窓中に ambient が入り pending が変わる | プロンプトは再出現するので少し遅れて再検出されるだけ |
| 応答行末尾が一瞬 `>` で見える誤検出 | ~50ms の安定確認で回避（応答行は通常改行付きで pending にならない） |
| **silent 成功（出力なし）コマンド**（`name`/`maxusers`/`focus`/空 `listbans`/空 `friendrequests`） | プロンプト末尾検出が即発火 → **応答=空行リスト**として返す（実機検証済） |
| **プロンプト累積**（`Renamed>Renamed>Renamed>...`） | (a) Exec 内連射: 直列キューが防止。(b) **ExecGroup 内の連続 Exec**: silent 系後の prompt が lineBuf に残るため発生 → **stripPromptPrefix が `^([^>]*>)+` で全プロンプト剥がし**で対応（§5） |
| **Exec 実行中にヘッドレスプロセスが死亡** | `readPipe` 終了 → コレクタへの新規追加停止 → `readChunk` が空継続 → cmdMaxTimeout で切上げ → `ErrProcessGone` を返す（既存 Wait goroutine が ready フラグを落とすので、以降の Exec は `ErrNotReady`） |

## 5. プロンプト接頭辞除去（案X 拡張版）

実機観測: 応答の最初の行は `<world>>[0] ...` のように **プロンプトが連結**される（プロンプトに改行が無い）。

**ルール**: 応答の最初の行に限り、**行頭の連続プロンプトを全て除去**する。
正規表現: `^([^>]*>)+`

```
単一: "Fake World 0>[0] World A …"              → "[0] World A …"
連続: "Fake World 0>Fake World 1>Name: …"        → "Name: …"
4連:  "R>R>R>R>Unknown command"                 → "Unknown command"
```

世界名の追跡は不要（generic に剥がすだけ）。これによりパーサは `^\[` 等の **`^` アンカー正規表現**を素直に書ける。

### なぜ連続プロンプトが起きるか（実装上の事実）

設計の「直列キューでプロンプト累積を防ぐ」は **単一 Exec の内側では正しい**が、
`ExecGroup` で **silent 系コマンド（focus/name 等）が連続する**と、
前コマンドの prompt が readPipe の `lineBuf` に「`\n` 未確定の状態」で残り、
次コマンドの最初の応答行に連結される。例:

```
[focus 1 完了時点] lineBuf = "Fake World 1>" （未確定）
[status 開始]
[status の Name 行が来る] lineBuf = "Fake World 1>Name: Fake World 1\n"
[行確定] → collector.appendLine("Fake World 1>Name: Fake World 1")
```

ExecGroup 内では前コマンドの prompt が次コマンドの応答に必ず連結するため、
パーサ側で「行頭の連続プロンプト」を **すべて剥がす**のが正解。

### 既知の限界
構造化コマンドの応答1行目に `>` が含まれる場合、過剰に剥がす可能性がある。
現在対応するコマンド（worlds/status/users/listbans/friendrequests/accesslevel/Unknown）の
1行目はいずれも `>` を含まないため実害なし。世界名等に `>` を含めるエッジケースは
ドキュメント明記の限界として受容する。

## 6. 原子的グループ

`focus` は**サーバー全体で共有の状態**。「`focus 2; status`」のような複数手順は、**同じ排他ロックを保持して連続実行**する（他の構造化コマンドが間に割り込まない）。

```go
err := executor.ExecGroup(ctx, func(tx Tx) error {
    if _, err := tx.Exec("focus 2"); err != nil { return err }
    lines, err := tx.Exec("status")
    if err != nil { return err }
    parsed := parseStatus(lines)
    // ...
    return nil
})
```

## 7. WorldsService（巡回の共通化）

```go
type WorldsService interface {
    List(ctx) ([]World, error)        // worlds 一発 → 構造化
    ForEach(ctx, fn func(w World, s Scope) error) error  // 各worldをfocus→fn を原子的に
}
```
- `List()` の合計人数で **userZero 判定**（focus 巡回不要）
- 事前アクション・セッション変更・各world のユーザー一覧取得は `ForEach` を共用

## 8. パーサ（コマンド毎）

| コマンド | 戻り値の主フィールド |
|---|---|
| `worlds`         | `[]World{Index, Name, Users, Present, AccessLevel, MaxUsers}` |
| `status`         | `Status{Name, SessionID, CurrentUsers, PresentUsers, MaxUsers, Uptime, AccessLevel, HiddenFromListing, MobileFriendly, Description, Tags, Users}` |
| `users`          | `[]User{Name, ID, Role, Present, PingMs, FPS, Silenced}` |
| `listbans`       | `[]Ban{Index, Username, UserID, MachineIDs}` |
| `friendrequests` | `[]string`（ユーザー名一覧） |

- 正規表現は `docs/resonite-domain-facts.md` を出典。
- 各 parser は **`stripLineLeadingPrompts` を per-line で適用してから regex 照合**（Phase 6 e2e で発見）。
- ambient/無関係行は正規表現に当たらず自然に無視。

### Phase 6 e2e で発覚した重要事実

Driver の collector は Exec 中に流れる stdout 行を**全て**捕える。
Resonite 起動直後は boot output (BOOTSTRAP, SignalR, 各 world の Opening world など)
が大量に流れているため、Exec("worlds") の応答行は collector の**末尾近く**に
位置し、しかも応答1行目には前 prompt が glue している（例: 107行中 105行目に
`MRHC Test World B>[0] World A ...`）。

旧設計の「lines[0] のみ stripPromptPrefix」では、ambient 末尾の応答行を
strip できず、`^\[(\d+)\]` regex が当たらず World A が parser から消えていた。
修正: parser は per-line で `stripLineLeadingPrompts` を適用 → ambient と
混在しても応答行を正しく抽出できる。ambient 行に `>` があれば過剰剥がしに
なるが、その行は parser regex に当たらず無害。
- **2026-05-28 実機採取での修正点**：
  - `status` パーサは **`ResoniteLink` Key を追加**（旧コードに無い）
  - `users` パーサは **`id` 空文字を許容**（旧 `\S+` → `[^\s]*` 等）
  - その他は旧 regex がそのまま通用（採取で再確認）
- **未知Keyへの寛容性（将来のバージョン変化への耐性）**:
  - `status` パーサは「`<key>: <value>` を全て収集し、**知っている Key だけ構造体に写す**」方針。未知Keyは warning ログを 1 回だけ出して握る（毎回出さない＝spam防止）。これにより新Key追加で**パースが落ちない**。
  - `worlds` / `users` 等の表形式は regex が当たらない行を**無視**するだけで耐性あり（ResoniteLink追加でも壊れなかった実例）。
  - 既知Key全部が一斉に書式変わった場合は明示的な不一致として扱う（フィクスチャの差分で検知）。

## 9. Go 型・インターフェース（概略）

```go
type ExecOption func(*execConfig)
type execConfig struct {
    MaxTimeout       time.Duration
    SettleConfirm    time.Duration // 既定 50ms
    ReadChunkTimeout time.Duration // 既定 20ms
}

type Executor interface {
    Exec(ctx context.Context, cmd string, opts ...ExecOption) ([]string, error)
    ExecGroup(ctx context.Context, fn func(tx Tx) error) error
    // ライブログ購読は既存 Driver.SubscribeLog を継続利用
}

type Tx interface {
    Exec(cmd string, opts ...ExecOption) ([]string, error)
}

type WorldsService interface {
    List(ctx context.Context) ([]World, error)
    ForEach(ctx context.Context, fn func(w World, s Scope) error) error
}

type Scope interface { // ForEach の fn 内で使う
    Exec(cmd string, opts ...ExecOption) ([]string, error) // 既に focus 済み
    World() World
}
```

## 10. エラー処理・タイムアウト

- `Exec` は (lines, err) を返す。err = nil で成功、timeout/EOF/送信失敗で非 nil。
- timeout 時は**収集できた行を含めて**返す（部分パースが可能なように）。呼び出し側は err を見て扱いを決める。
- 標準的なエラー区分（センチネル）:
  - `ErrNotReady`: ヘッドレスが ready でない時の Exec 呼び出し
  - `ErrTimeout`: cmdMaxTimeout 超過（lines は部分結果）
  - `ErrProcessGone`: Exec 実行中にプロセスが死亡（lines は部分結果）
  - `ErrCanceled`: ctx.Done() による中断
- 同期：context キャンセル時は実行を放棄して err 返す。**ロックは必ず `defer Unlock()` で解放**。

## 11. テスト戦略

- **Executor 単体**: 偽 stdin/stdout（バイト列スクリプト）で:
  - 完了検出（プロンプト末尾＋安定窓）
  - 直列化（同時2リクエストで排他確認）
  - 原子的グループ（grouped command 中に他リクエストが割り込まないこと）
  - timeout / readChunk / settle 各境界
- **Parser 単体**: 2026-05-28 実機採取fixture（`scripts/empirical-capture/fixtures/2026-05-28-windows-multiworld.log`）から抽出した worlds/status/users/accesslevel/Unknown command の実行ブロックを入力に期待構造体と比較。プロンプト接頭辞の有無、silent成功(空応答)、空ID も両方テスト。
- **統合**: `poc/fakehl` を拡張して worlds/status/users 風の応答を返せるようにし、Executor→WorldsService→ハンドラまで通す。
- **e2e（任意）**: 実 Resonite ヘッドレスを使った任意検証（ユーザーの 24/7 機で）。

## 12. 将来性: HeadlessBackend 抽象（軽量）

今は stock+stdout 1経路だが、将来 Crystite/mod系API へ拡張可能なよう、**通信境界に薄いインターフェース**を置く：

```go
// 命名と境界だけ意識（今は1実装のみ）
type HeadlessBackend interface {
    Status() Status
    Subscribe(...) (chan ..., ...)
    Execute(cmd) ([]string, error)
    // ...
}
```
今は実装1本だが、`adapter/headless/stdout/` 等のディレクトリ命名で**将来 `adapter/headless/grpc/` を足せる**形にしておく。**今回は実装しない**（命名と境界のみ）。

## 13. 受け入れ基準

- ✅ 直列性: 同時2リクエストが排他されて応答が混ざらない（test）
- ✅ 完了検出: settle のみではなくプロンプト末尾で確定（test）
- ✅ ライブログ非阻害: 構造化実行中も `logHub` は流れる（test）
- ✅ 原子的グループ: focus 変更中に他リクエストが割り込まない（test）
- ✅ パース: 旧書式の代表サンプルに対し期待構造体を返す（test）
- ✅ timeout: 過大時は部分結果＋err を返す（test）

## 14. 未決・後追い

- 実フィクスチャ（status/users/listbans/friendrequests）の採取は**後追い**（パース不一致が出た時点で）。
- HeadlessBackend 抽象の**他実装は未定**（Crystite/mod は採用見送り）。
- 設定での timeout/settle 上書きは v1.x のどこかで設定UIから可能にする（今は config に書ける程度）。
