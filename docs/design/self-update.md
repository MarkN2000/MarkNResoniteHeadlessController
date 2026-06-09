# MRHC 自己更新（self-update）— 確定仕様

> MRHC 自身を GitHub Releases の最新版へ入れ替える機能。設計レビュー（コードベース整合＋
> 更新メカニズム穴の 2 本・2026-06-07）反映済み・Windows 実機 E2E（CLI / Web UI 全段階）合格。
> 2026-06-09 改修: Web UI の適用を SSE 進捗ストリーミング化・「今すぐ終了」を **「今すぐ再起動」
> （re-exec で新バイナリ起動）** へ・check に短期キャッシュ追加（H1/H2/M1/M2）。
> 関連: `cli-onboarding.md`（CLI 面）・`.github/workflows/release.yml`（配布規約の正本）。

## 0. 方針（ユーザー裁定 2026-06-07 / 2026-06-09 改修）

1. 入口は2つ: **Web UI のボタン**（遠隔可・ふだん使い）と **`mrhc update` サブコマンド**
   （config 不要＝起動不能な環境からの復旧経路を兼ねる）。コアは `internal/selfupdate` で共通。
2. 適用＝**バイナリ差し替えまで**（稼働中ワールドに無影響）。適用後は
   「**今すぐ再起動する**（ワールド graceful 停止込み→新バイナリで自分自身を起動し直す）／
   あとで自分で再起動」の2択。再起動は Unix=`syscall.Exec` の同一プロセス置換、Windows=新プロセス
   起動＋本プロセス終了で行う（いずれも HTTP リスナーを閉じ・ヘッドレスを停止した後＝ポート解放後）。
   停止モードは通常停止と同じ graceful（`driver.Stop()` の180秒猶予）を維持する。
3. 自動チェックのポーリング・自動 apply・コード署名検証はしない（個人配布ツールの脅威モデルに過剰）。
   apply/再起動はすべてユーザー操作起点（勝手に更新・終了することはない）。

## 1. 配布前提（release.yml が正本）

- アセット名は版番号なしの固定: `mrhc-windows-amd64.zip` / `mrhc-linux-{amd64,arm64}.tar.gz`
  ＋ `SHA256SUMS`。アーカイブ内はトップフォルダ `mrhc-<os>-<arch>/` に `mrhc(.exe)`＋README＋LICENSE。
- プレリリースタグ（`-` 含み）は `releases/latest` を指さない。
- **リリースは draft で作成し全アセット添付後に公開**（`gh release edit --draft=false`）。
  公開直後に latest がアセット欠落を指す窓を消す（自己更新と install.sh の 404 対策）。

## 2. チェック（selfupdate.Check）

- `https://github.com/<repo>/releases/latest` へ**リダイレクト無効** GET →
  `Location: …/releases/tag/<tag>` からタグを抽出。GitHub API は使わない
  （API の未認証 60req/h 制限を受けない。Web エンドポイントは対象外）。**API に切り替えないこと。**
- 404 = リリース未公開（sentinel `ErrNoRelease`・「更新なし」とは区別して表示）。
  タグは `x/mod/semver` の IsValid で厳格検証（リポジトリ改名の 301 等は marker 不一致でエラー）。
- 比較は semver。`UpdateAvailable = IsValid(current) && Compare(latest, current) > 0`
  ＝ latest を過去版へ付け替えてもダウングレードは提示しない。
- **非 semver の焼込版（"dev"・workflow_dispatch のブランチ名）は適用不可ビルド**
  （`CurrentIsRelease=false`）。最新版の表示だけ行う。
- チェック用とDL用の http.Client は**共有しない**（チェック=リダイレクト読む/DL=リダイレクト追う
  — アセットは objects.githubusercontent.com へ 302 するため、共有するとどちらかが壊れる）。

## 3. 適用（selfupdate.Apply）— 既存バイナリに触るのは最後の rename の一瞬だけ

基準パスは常に **`os.Executable()`**（`-data` とは無関係。exe の隣＝同一ボリュームが
atomic rename の前提）。手順:

1. 非 semver 版は即 `ErrNotReleaseBuild`。タグを**自分で解決し直し** `Compare(latest,current)>0`
   を強制（Check との間に latest が動いた場合のダウングレードも防ぐ）。
2. **ロック** `<exe>.update.lock`（O_CREATE|O_EXCL・PID 記録）で UI/CLI の別プロセス間を排他。
   1時間より古いロックは中断の残骸として除去して1回だけ取り直す。多重は `ErrBusy`。
3. **`SHA256SUMS` を本体より先に**同じタグ配下から取得（公開直後等の欠落時に十数MBの
   DL を始める前に失敗させる）→ exe の隣に `.mrhc-update-*.partial`（CreateTemp 一意名）
   として自 GOOS/GOARCH 用アセットを DL（対応表に無いプラットフォームは
   `ErrUnsupportedPlatform`・サイズ上限 200MiB）。一時ファイル作成は通信より先＝
   書込不可（sudo 展開等）はネット前に EACCES で判明。
4. **ディスクから読み直して** SHA-256 照合
   （DLストリームに対するハッシュでは並行書込等によるディスク上の破損を検出できない）。
5. アーカイブから `mrhc-<os>-<arch>/mrhc(.exe)` **だけ**を `<exe>.new` へ抽出
   （エントリ名は path.Clean 正規化で比較・tar=TypeReg 限定・zip=symlink モード拒否・
   実コピー量で上限・0755・rename 前に fsync）。アーカイブ内パスはファイルパス構築に
   使わない＝zip-slip は構造的に不成立。
6. `debug/pe` / `debug/elf` で**形式とアーキテクチャを検査**（リリース作業ミスで別物が
   入った事故を入れ替え前に検出）。
7. **2段 rename**: `os.Remove(<exe>.old)` を試行 → 失敗（=何かの実行中イメージ。
   再起動せず2回目の更新がこれ）なら退避先を `<exe>.old-<unix秒>` に変更 →
   `<exe>` → old / `.new` → `<exe>`。2回目の rename は AV の一時ロックに備え
   200ms×3 リトライ・なお失敗なら **old を戻して復元**（exe 不在の状態を残さない）。
   - 原理: Windows は実行中 exe の削除・上書きが不可でも**リネームは可能**
     （rclone selfupdate 等と同方式）。Linux は rename(2) が原子的に置換。
     実行中プロセスは旧イメージのまま動き続け、**次回起動から新版**。
   - swap 失敗時は検証済み `.new` を**消さない**（三重失敗＝rename×3＋復元失敗で exe 不在に
     なった場合の手動復旧先として残す。残骸は次回起動の掃除が stale 判定で回収）。

### エラー → 利用者表示（sentinel + errCode 方式・steam と同じ）

| sentinel / 状態 | API errCode | CLI |
|---|---|---|
| ErrNoRelease | `no_release` (404) | リリース一覧 URL を案内 |
| ErrUpToDate | `up_to_date` | 「既に最新です」（正常終了） |
| ErrBusy | `update_busy` | 完了待ち案内 |
| ErrNotReleaseBuild | `not_release_build` | 「リリースビルドではない」 |
| fs.ErrPermission | `exe_dir_not_writable` | sudo chown / root 実行を案内 |
| その他 | `update_failed` | 生メッセージ |

> apply は SSE 化したため、ストリーミング時のエラーは HTTP ステータスではなく `update-error`
> イベント（`{code, message}`・HTTP 200）で返す。上表の errCode はその `code` に対応する。
> 非ストリーミング（http.Flusher 非対応）フォールバック時のみ従来の HTTP ステータス＋JSON 封筒。

## 4. staged（適用済み・再起動待ち）と再起動

- 適用成功＝staged。**サーバープロセス内のみ**の状態（メモリ）。再起動後は実体が追いつくので
  永続化しない。CLI で適用した場合、稼働中サーバーの staged 表示には反映されない（仕様）。
- staged 中の再適用: latest が staged と同じなら**再DLせず冪等応答**。より新しい latest が
  出ていれば上書き適用（.old が実行中イメージでも §3-7 の一意名退避で成功する）。
- **POST /api/v1/restart**: 応答を返してから main へ再起動依頼（チャネル）→ Ctrl+C と同じ
  graceful 経路（ヘッドレス停止を最大 185s 待つ→HTTP shutdown）を経た**後**、`relaunch()` で
  新バイナリを起動し直す（Unix=`syscall.Exec`／Windows=新プロセス起動＋本プロセス終了）。
  - **要注意（落とし穴）**: `relaunch` に渡す exe パスは**起動時（swap 前）に捕捉**すること。
    swap は実行中 exe を `<exe>.old` へリネームするため、swap 後に `os.Executable()`
    （Linux=`/proc/self/exe`）を呼ぶと `.old`（旧版）を指し、**旧版を再起動してしまう**。
    main は runServer 冒頭で `selfExe` を捕捉して relaunch に渡す。
  - UI は依頼成功で全画面を再起動中画面（RestartingScreen）に差し替え＝Shell unmount で停止中の
    SSE 再接続エラーループを残さない。
  - **新プロセス検出**: 再起動要求後も旧 HTTP サーバーはヘッドレス停止中（最大 185s）応答し続ける
    ため「応答あり＝復帰」では旧サーバーへ誤って戻る。プロセス毎の **boot 識別子**（無認証
    `GET /api/v1/ping` が `{boot}` を返す・乱数）を使い、再起動直前に捕捉した boot から変化したら
    新プロセスとみなして reload する（タイミング非依存）。boot は再起動を投げる直前
    （旧プロセスが確実に生きている時点）に UpdateModal で捕捉して RestartingScreen へ渡す。
- 停止が重いワールドで遅いと、再起動完了（UI 復帰）まで最大数分かかりうる（停止モードは
  graceful 維持の裁定）。再起動後はセッション（メモリ）が切れるため再ログインになる（許容）。
  Ctrl+C/SIGTERM での終了は従来どおり「ただ終了」（re-exec しない）。再起動中の Ctrl+C は中断。

## 5. API / CLI / UI

- `GET /api/v1/update/check`（requireAuth）→
  `{current, latest, updateAvailable, currentIsRelease, staged?, goos, checkFailed?, checkError?}`。
  GitHub への問い合わせは呼ばれた時のみ（フロントもポーリングしない）。結果は**サーバー側で
  短期キャッシュ**（TTL 10分・表示用途のみ。apply 冪等判定は常に最新を引く・適用成功で破棄）。
  **GitHub 不達でも 200 で返す**: staged/current/goos はローカル情報なので常に有効とし、
  失敗は `checkFailed:true` ＋ `checkError:"<errCode>"` で通知する（エラー応答にすると
  適用済み＝再起動待ちの表示と「今すぐ再起動」導線が UI から消えるため・不達はキャッシュしない）。
  フロントも staged 保持中は null（MRHC 自体に不達）で表示を上書きしない。
- `POST /api/v1/update/apply`（requireAuth）→ **SSE ストリーミング**（`text/event-stream`）。
  イベント: `update-progress`(`{downloaded,total}`) / `update-result`(`{staged}`) /
  `update-error`(`{code,message}`)。Apply は `context.Background()` で実行＝ブラウザ切断でも
  中断しない（全体上限は DLClient の Timeout 15min）。応答を取り逃しても check の staged で回収。
  http.Flusher 非対応の writer では従来の同期 JSON（`{staged}`／エラーは HTTP ステータス）へ
  フォールバック。フロントは fetch + ReadableStream で読む（EventSource は POST 不可）。
- `POST /api/v1/restart`（requireAuth）→ `{accepted}`。§4 参照。
- `GET /api/v1/ping`（**無認証**）→ `{boot}`。プロセス毎の乱数のみ公開（情報漏えいなし）。
  再起動中画面の新プロセス検出専用。§4 参照。
- CLI `mrhc update`: **ウィザード分岐より前**に dispatch（新規環境でウィザードを起動させない）・
  config 不要（言語は config があれば LangOrDefault・無ければ OS 検出）。進捗 SSE は Web 専用で
  CLI は従来どおり（`Apply` を progress=nil で呼ぶ）。
- UI: ログイン後に**1回だけ**自動チェック → ⋮ に赤丸（Indicator）。メニュー「更新を確認」
  （staged 中は「再起動待ち（vX）」）→ UpdateModal。モーダルは開くたび再チェックし、
  最新/開発ビルド/更新あり/適用済みの4状態を check 結果から導出。適用中は進捗バー表示。
  適用済みビューは「今すぐ再起動する」＋OS 別（check の goos）の手動再起動手順を1画面で表示。
  文言の正本は `web/src/locales/{ja,en}.json` の `update.*`/`restarting.*`・`topbar.checkUpdate`/
  `topbar.updatePending`、CLI は `internal/i18n/catalog.go` の `main.update.*`/`main.restart.*`。

## 6. 起動時掃除（selfupdate.CleanupStale・runServer 冒頭）

- `<exe>.old*` を削除（**自分自身と同一ファイルは SameFile ガードでスキップ**＝復旧のため
  .old を直接起動しているケースで自分を消さない）。見つかれば
  「MRHC は vX に更新されました」を1回ログ（更新の痕跡は .old の存在だけ）。
- `<exe>.new`・`.partial`・lock は**1時間より古いものだけ**削除
  （進行中の `mrhc update` 別プロセス＝extract 完了〜swap の間に .new が秒オーダーで
  存在する＝を壊さない）。削除失敗は無視（次回起動で再掃除）。

## 7. テスト・検証

- 単体（internal/selfupdate）: タグ抽出/semver/404/不正リダイレクト・SHA 改竄・symlink 拒否・
  別アーキ/非バイナリ拒否・ロック（busy/stale）・.old 削除不可の一意名退避・掃除の自己ガード等。
- HTTP 層（internal/server/update_test.go）: errCode マップ（SSE `update-error`）・staged の
  SSE `update-result`・check の TTL キャッシュ・restart コールバック・`/ping` 無認証＋boot・401。
- E2E: **`MRHC_UPDATE_BASE`**（main で読んで注入。selfupdate 内では env を読まない）を
  ローカルの偽 GitHub サーバー（`/releases/latest` 302 ＋ `/releases/download/<tag>/` 配信）に
  向け、実バイナリ v0.0.1→v0.0.2 で実施。2026-06-07 Windows 実機で CLI・Web UI とも全段階合格
  （実行中イメージの rename・graceful exit・起動時ログ・掃除を含む）。
  **2026-06-09 改修（re-exec／SSE 進捗）は両 OS の実機 E2E 未実施＝要確認**:
  Windows の `relaunch`（子プロセス起動・コンソール挙動・ポート再bind）と、再起動後の
  RestartingScreen 自動復帰を Linux/Windows 双方で確認すること。

## 8. 手動復旧（README にも記載）

万一 `mrhc(.exe)` が無い/壊れた状態になったら、隣に残っている `mrhc(.exe).old`
（または `.old-<数字>`）を `mrhc(.exe)` に**名前を戻すだけ**で旧版に復旧できる
（電源断が rename 2回の間のマイクロ秒窓に当たった場合も同様。コードでの対策は持たない）。

## 9. やらないこと（裁定済み）

- 常時ポーリング・自動アップデート・自動再起動（§0）
- minisign/cosign 等の署名検証・更新前バイナリの `-version` 実行確認（無意味＋AV 誤検知の火種）
- GitHub API への切り替え（§2）・zip bomb の LimitReader 超の対策
