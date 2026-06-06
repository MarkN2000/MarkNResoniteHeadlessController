# .NET ランタイム自動設置（dotnet-runtime プロビジョニング）— 確定仕様

> Linux amd64 実機テスト（2026-06-07）で「DD の DL 品に .NET ランタイムが含まれず起動不能」が
> 発覚したことへの対処。設計レビュー（Plan agent・アドバーサリアル 2 周）反映済み。
> 関連: `steam-depotdownloader.md`・`deps-onboarding.md`（dotnet10 検出は本仕様へ置換）・
> `resonite-domain-facts.md` §4.6（前提の訂正）。

## 0. 背景（事実・2026-06-07 DL 品実地調査）

- **DD（DepotDownloader）の DL 品に .NET ランタイムは含まれない**。公式は初回のグラフィック
  クライアント起動時に設置する:
  - **Linux**: ルートの `LinuxBootstrap.sh` が**毎起動**、同梱の `dotnet-install.sh` を
    `--channel 10.0 --runtime dotnet --install-dir $PWD/dotnet-runtime` で実行
    （存在チェックなし・冪等はスクリプト任せ・要ネット）。`<install>/dotnet-runtime/` の正体はこれ。
  - **Windows**: `InstallScript.vdf` で Steam クライアントが
    `Tools\Redist\dotnet-desktop-runtime-*.exe /silent` を実行（システム全体）。
    **この exe は DD の DL 品に無い**＝素の Windows でも同じ問題が起きる。
- 旧 R-C の前提「x64 は Resonite 同梱 dotnet で完結」は**誤認**（Steam クライアント＋初回起動済み
  環境を観測していた）。
- 要求ランタイムの正本は `<install>/Headless/Resonite.runtimeconfig.json`
  （`Microsoft.NETCore.App 10.0.0`。デスクトップランタイムは不要）。

## 1. 方針（ユーザー裁定 2026-06-07）

1. **ネイティブ Go 方式**で MRHC が公式ビルドフィードから直接取得し
   `<install>/dotnet-runtime/` へ設置（sudo/管理者権限・curl/bash/ps1 不要・書込は data 配下）。
2. **起動時ガード** = 不足なら「起動」ワンアクションで 受付→自動設置→自動起動。
3. 旧 dotnet10 検出・案内（R-C・arm64 限定）は**完全撤去**して置換（freetype2 の 3 経路は存続）。
4. **channel は runtimeconfig.json から導出**（MRHC にバージョンをハードコードしない＝
   Resonite の .NET 移行に自動追従）。
5. システム .NET が要求を満たす環境では**設置せず・DOTNET_ROOT も触らない**（挙動不変）。

## 2. 取得（internal/dotnetruntime）

フィードは dotnet-install.sh 自身が使う安定 URL（2026-06-07 実測検証済み）:

| 資源 | URL |
|---|---|
| 最新版番号 | `https://builds.dotnet.microsoft.com/dotnet/Runtime/<channel>/latest.version`（**最後の非空行を trim**。1行/2行（hash+版）両形式あり） |
| アーカイブ | `.../Runtime/<ver>/dotnet-runtime-<ver>-<rid>.tar.gz\|.zip`（rid: win-x64=zip / linux-x64・linux-arm64=tar.gz。**全 rid の実在を HTTP 200 で確認済み**） |
| チェックサム | アーカイブ URL + `.sha512`（`<hex128>  <filename>`） |

`Acquirer.Ensure(ctx, installDir, channel, onEvent)`:

1. latest.version で版確定。**prerelease（"-" 含む）は設置せずエラー**
   （実測: GA 前チャンネル `11.0` は `11.0.0-preview.…` を返す。ホストは release 要求から
   prerelease へ roll-forward しないため設置しても使えず、放置すると毎起動フル再DLループになる）。
2. 確定版が `shared/Microsoft.NETCore.App/<ver>/` に在り host も在れば**何もせず成功**（再DL防止）。
3. アーカイブを installDir 直下の一時ファイルへ DL（SHA-512 を同時計算）→ `.sha512` と突合
   （公式スクリプトはサイズ比較のみ＝**公式より強い検証**。同一ホスト由来で TLS が信頼アンカー）。
4. `.dotnet-runtime.new` へ全展開（tar は mode 保持＝実行ビット焼込・Dir/Reg/Symlink のみ許可・
   `filepath.IsLocal` で tar-slip / 不正リンク先を拒否）。
5. スワップ: 既存→`.dotnet-runtime.old` → `.new`→`dotnet-runtime` → `.old` 削除
   （Windows は非空 dir へ rename 不可のため 2 段。Ensure 冒頭で stale `.new`/`.old` を回収）。
   **全置換でマージしない**（旧 minor・他 arch の残骸を残さない。マイグレ嫌い方針と整合）。

- DL のハング対策は本パッケージ内で完結: **無進捗 60s で中断**（進捗があれば何分でも続く）。
  steam.Manager の stall ウォッチドッグは従来どおり DD 区間専用（watchCancel の位置は不変）。
- 進捗は Content-Length 比で 1% 刻みの progress（File=アーカイブ名）・log は MsgKey
  （`dotnetInstall`/`dotnetInstalled`）・milestone は `Installing .NET Runtime`。

## 3. 判定（internal/platform/dotnetreq.go）

- `ReadRuntimeRequirement(headlessDir)`: runtimeconfig.json から Microsoft.NETCore.App の要求版を
  読む（framework 単数/frameworks 配列 両対応）。**読めない＝楽観で何もしない**
  （fakehl 等 runtimeconfig の無い環境の挙動を変えない。R-B と同思想）。
- `LocalRuntimeSatisfies(installDir, req, goarch)`: **ローカル列挙のみ・オフライン安全・ms オーダー**。
  host（dotnet[.exe]・ELF arch 一致 or 判定不能）＋ shared に「major 一致 && minor>=（同 minor なら
  patch>=）」の版（roll-forward=Minor 規則）。**prerelease 版ディレクトリは不充足扱い**。
- `SystemRuntimeSatisfies(goos, goarch, req)`: システム .NET が充足と**確認できたときだけ** true
  （unknown は false＝設置側に倒す）。候補は Linux=`~/.dotnet/dotnet`→PATH /
  Windows=PATH→`%ProgramFiles%\dotnet`。`--list-runtimes` 10s タイムアウト。旧 `dotnet10Status` の一般化。

## 4. フック

### 4.1 DL/更新完了後（steam/manager.go run() 最終段・最重要）

acquire→DD→verify→chmod の後に `runtimeEnsurer.ensure`（steam/dotnet.go）:
要求が読めない/ローカル充足/システム充足 → スキップ（ログなし）。不足 → 設置。
**設置成功後に再判定し、なお不充足（要求 patch > フィード最新等）は明示エラー**
（サイレント再DLループを作らない）。

- wizard S5b・Web UI 手動更新・orchestrator 自動更新の**全経路をカバー**。Resonite 更新で要求版が
  上がっても、新 runtimeconfig が直前に落ちてくるためここで自動追従する。
- 失敗 = **更新失敗扱い**（`ErrDotnetInstallFailed` → errorCode `dotnet_install_failed`・
  errorDetail に内側原文）。DD 成果物はロールバックしない（DD は差分再開・起動時ガードが再試行）。

### 4.2 起動時ガード（server/dotnetguard.go）

`handleStart`（headless_not_installed チェック直後）:

- 稼働中 → 従来どおり同期 409（ガード経路で accepted を作らない）。
- 要求が読めない / ローカル充足 / **システム充足キャッシュ**命中 → 従来の同期起動（挙動不変）。
- 不足 → `{"accepted":true,"runtimePrepare":true}` を返し goroutine（親= bg ctx）で:
  1. システム充足（probe 最大10s）→ **プロセス内キャッシュ**（installDir+要求版。以後は同期経路＝
     Steam クライアント併用 Windows が毎回 async+probe にならない）→ 起動。
  2. 不足 → sys ログ「設置します」→ `steam.InstallRuntime`（進捗は steam SSE）→ 成功 → 起動。
     - `ErrCancelled`（/steam/cancel・shutdown）→ 中立文言で打ち切り（失敗として案内しない）。
     - `ErrUpdateInProgress` → 案内して打ち切り。
     - **その他の失敗 → 失敗+手動導入手段（ManualDotnetInstallHint）を sys ログで案内した上で
       起動を best-effort 実行**（システム .NET があるのに probe が unknown だった環境・
       installDir 書込不可の環境で、従来成功していた起動を壊さない＝**可用性の下限は現行挙動**）。
  3. 起動直前に steam 更新が begin していたら起動見送り（稼働中 DD 書込の防止。sys ログで案内）。
  4. 起動成功時は同期経路と同じ副作用（recordLastUsed / recordLastStart / publishDepGuide）。
     起動失敗は sys ログで明示（accepted 後の失敗を黙殺しない）。
- **設置中の起動取消手段 = `/steam/cancel`**（設定タブの「中止」）。
- orchestrator: scheduled 再起動は 4.1 で設置済み・クラッシュ復帰は直前稼働＝在中前提でガード不要
  （deps 経路③と同じ整理）。

### 4.3 InstallRuntime（steam.Manager・single-flight 共用）

ランタイム単独設置の同期入口（Steam 資格不要・`beginRun(runtime)`）。更新と**同じスロット・
同じ SSE・同じ Status** を使う（書込先が同一資源のため。排他漏れ・配信二重化を防ぐ）。
`Status`/result イベントに **`RunKind: "update"|"runtime"`** を導入し、フロントが
「更新」/「ランタイム設置」を出し分ける（起動しただけで「更新が走った」ように見える混線・
直前の更新失敗が success に上書きされて見える混線への対処）。

## 5. launcher（internal/platform/launcher.go）

- **Windows**: 要求が読めて `LocalRuntimeSatisfies` のときだけ
  `DOTNET_ROOT=<install>\dotnet-runtime` を env で渡す。apphost は DOTNET_ROOT 設定時
  **そこだけを見てシステムへフォールバックしない**ため、「存在する」でなく「充足する」が条件。
  満たさなければ env を触らない＝システム .NET（開発機・Steam クライアント併用環境は挙動不変）。
- **Linux** `resolveDotnet`: ローカル採用条件を「dotnetUsable（ELF arch）かつ（要求が読めれば）
  LocalRuntimeSatisfies」に強化。stale ローカルはシステムへ正しくフォールバック。
  要求が読めない場合は従来動作（楽観採用）。

## 6. 旧 dotnet10 機構の撤去（完全撤去・互換コード残さない）

- deps.go: dotnet10 チェック・`dotnetInstallCmd` 削除（`dotnet10Status`/`hasDotnet10` は §3 へ
  一般化移設）。`CheckHeadlessDeps` の installDir 引数も撤去（freetype2 に不要）。
- deps_prompt.go: ウィザード [Y/n] の dotnet10 特例撤去 → freetype2 専用。
- i18n: `deps.*.dotnet10` 系 5 キー削除。手動 fallback は `ManualDotnetInstallHint`
  （Linux=dotnet-install.sh の channel 埋込コマンド / Windows=公式 DL ページ URL）で継承。
- **freetype2 の 3 経路は現状維持**（sudo 必須＝自動化できない依存は案内が正しい姿）。

## 7. wizard S5b（cli-onboarding 追補）

- milestone `Installing .NET Runtime` 受信で区間ラベル「.NET ランタイムを設置中...」を出し、
  進捗の単調ガード（lastStep）をリセット（DD 100% 後の設置区間が無言にならない・
  設置 DL の % も 10% 刻みで見える）。
- `ErrDotnetInstallFailed` は専用文言（DL 自体は完了・起動時ガードが自動再試行する旨）で
  再入力ループに入れない。

## 8. エラーコード・i18n（追加分）

| code | sentinel | 意味 |
|---|---|---|
| `dotnet_install_failed` | ErrDotnetInstallFailed | ランタイム設置の失敗（detail に内側原文） |

- steam Event MsgKey: `dotnetInstall`（version,file）/ `dotnetInstalled`（version）。
- CLI: `dotnet.sys*`（ガードの sys ログ）・`wizard.dl.dotnetInstalling/dotnetFailed`。
- Web: `settings.steamErrDotnetInstall`・`steamLogDotnetInstall(ed)`・`steamMilestoneDotnet`・
  `steamInstallingRuntime`・`steamRuntimeSuccess`・`toast.startRuntimePrepare`。
  start 応答の `runtimePrepare:true` で情報トースト。

## 9. 既知の edge（許容・docs 化のみ）

- orchestrator の `waitStopped` は 190s で諦めて更新へ進むため、kill 失敗時は理論上
  「稼働中スワップ」が起き得る（既存の DD 書込にも同じ穴があり、本機能で新設したものではない）。
- ユーザーが Steam.InstallDir で**既存 Steam インストールを指す**場合、dotnet-runtime は
  そのフォルダ内へ設置される（data 外だが、DD 書込・chmod -R が既にそこへ走る設計と同等）。
  書込不可（Program Files 等）なら設置は失敗するが、システム .NET 充足なら設置自体がスキップ、
  失敗しても best-effort 起動で従来挙動が下限。

## 10. 出典

- DL 品実地調査（2026-06-07・Windows DD 取得物）: `LinuxBootstrap.sh`（毎起動・
  dotnet-install.sh→`$PWD/dotnet-runtime`）/ `InstallScript.vdf`（Redist exe・DL 品に無し）/
  `Headless/Resonite.runtimeconfig.json`（NETCore.App 10.0.0）/ `Tools\Redist` 不在。
- フィード実測（2026-06-07）: `Runtime/10.0/latest.version`→`10.0.8`・
  `11.0/latest.version`→`11.0.0-preview.4.26230.115`・win-x64/linux-x64/linux-arm64 アーカイブと
  `.sha512` の実在（HTTP 200）・公式スクリプト（dotnet/install-scripts）はハッシュ検証なし。
- apphost の DOTNET_ROOT 挙動・roll-forward 既定 Minor: Microsoft Learn
  （.NET ランタイム選択 / DOTNET_ROOT 環境変数）。
