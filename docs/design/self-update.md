# MRHC 自己更新（self-update）— 確定仕様

> MRHC 自身を GitHub Releases の最新版へ入れ替える機能。設計レビュー（コードベース整合＋
> 更新メカニズム穴の 2 本・2026-06-07）反映済み・Windows 実機 E2E（CLI / Web UI 全段階）合格。
> 関連: `cli-onboarding.md`（CLI 面）・`.github/workflows/release.yml`（配布規約の正本）。

## 0. 方針（ユーザー裁定 2026-06-07）

1. 入口は2つ: **Web UI のボタン**（遠隔可・ふだん使い）と **`mrhc update` サブコマンド**
   （config 不要＝起動不能な環境からの復旧経路を兼ねる）。コアは `internal/selfupdate` で共通。
2. 適用＝**バイナリ差し替えまで**。自動再起動はしない。適用後は
   「**今すぐ終了する**（確認付き・ワールド graceful 停止込み）／あとで自分で再起動」の2択。
   再「起動」だけは必ず人間が行う（自動起動のポート引き継ぎ・端末切り離し等の難所を持たない）。
3. 自動チェックのポーリング・コード署名検証はしない（個人配布ツールの脅威モデルに過剰）。

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
| ErrUpToDate | `up_to_date` (409) | 「既に最新です」（正常終了） |
| ErrBusy | `update_busy` (409) | 完了待ち案内 |
| ErrNotReleaseBuild | `not_release_build` (409) | 「リリースビルドではない」 |
| fs.ErrPermission | `exe_dir_not_writable` (500) | sudo chown / root 実行を案内 |
| その他 | `update_failed` (502) | 生メッセージ |

## 4. staged（適用済み・再起動待ち）と終了

- 適用成功＝staged。**サーバープロセス内のみ**の状態（メモリ）。再起動後は実体が追いつくので
  永続化しない。CLI で適用した場合、稼働中サーバーの staged 表示には反映されない（仕様）。
- staged 中の再適用: latest が staged と同じなら**再DLせず冪等応答**。より新しい latest が
  出ていれば上書き適用（.old が実行中イメージでも §3-7 の一意名退避で成功する）。
- **POST /api/v1/shutdown**: 応答を返してから main へ終了依頼（チャネル）→ Ctrl+C と同じ
  graceful 経路（ヘッドレス停止を最大 185s 待つ→HTTP shutdown）。UI は依頼成功で
  全画面を静止画面（ShutdownScreen）に差し替え＝Shell unmount で SSE 再接続ループを残さない。
- 副作用: systemd 等の自動再起動付き運用では「終了→新版で自動起動」になる（非公式だが無害）。

## 5. API / CLI / UI

- `GET /api/v1/update/check`（requireAuth）→
  `{current, latest, updateAvailable, currentIsRelease, staged?, goos, checkFailed?, checkError?}`。
  GitHub への問い合わせは呼ばれた時のみ（フロントもポーリングしない）。
  **GitHub 不達でも 200 で返す**: staged/current/goos はローカル情報なので常に有効とし、
  失敗は `checkFailed:true` ＋ `checkError:"<errCode>"` で通知する（エラー応答にすると
  適用済み＝再起動待ちの表示と「今すぐ終了」導線が UI から消えるため）。フロントも
  staged 保持中は null（MRHC 自体に不達）で表示を上書きしない。
- `POST /api/v1/update/apply`（requireAuth）→ `{staged}`。**同期**だが Apply は
  `context.Background()` で実行＝ブラウザ切断でも中断しない（全体上限は DLClient の
  Timeout 15min）。応答を取り逃しても check の staged で回収できる。
- CLI `mrhc update`: **ウィザード分岐より前**に dispatch（新規環境でウィザードを起動させない）・
  config 不要（言語は config があれば LangOrDefault・無ければ OS 検出）。
- UI: ログイン後に**1回だけ**自動チェック → ⋮ に赤丸（Indicator）。メニュー「更新を確認」
  （staged 中は「再起動待ち（vX）」）→ UpdateModal。モーダルは開くたび再チェックし、
  最新/開発ビルド/更新あり/適用済みの4状態を check 結果から導出。適用済みビューは
  「今すぐ終了する」＋OS 別（check の goos）の手動再起動手順を1画面で表示。
  文言の正本は `web/src/locales/{ja,en}.json` の `update.*`/`shutdown.*`・`topbar.checkUpdate`/
  `topbar.updatePending`、CLI は `internal/i18n/catalog.go` の `main.update.*`。

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
- HTTP 層（internal/server/update_test.go）: errCode マップ・staged・shutdown コールバック・401。
- E2E: **`MRHC_UPDATE_BASE`**（main で読んで注入。selfupdate 内では env を読まない）を
  ローカルの偽 GitHub サーバー（`/releases/latest` 302 ＋ `/releases/download/<tag>/` 配信）に
  向け、実バイナリ v0.0.1→v0.0.2 で実施。2026-06-07 Windows 実機で CLI・Web UI とも全段階合格
  （実行中イメージの rename・「今すぐ終了」の graceful exit・起動時ログ・掃除を含む）。

## 8. 手動復旧（README にも記載）

万一 `mrhc(.exe)` が無い/壊れた状態になったら、隣に残っている `mrhc(.exe).old`
（または `.old-<数字>`）を `mrhc(.exe)` に**名前を戻すだけ**で旧版に復旧できる
（電源断が rename 2回の間のマイクロ秒窓に当たった場合も同様。コードでの対策は持たない）。

## 9. やらないこと（裁定済み）

- 常時ポーリング・自動アップデート・自動再起動（§0）
- minisign/cosign 等の署名検証・更新前バイナリの `-version` 実行確認（無意味＋AV 誤検知の火種）
- GitHub API への切り替え（§2）・zip bomb の LimitReader 超の対策
