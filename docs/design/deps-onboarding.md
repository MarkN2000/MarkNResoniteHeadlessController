# R-C: 依存検出＋案内（deps onboarding）— 詳細仕様

> P9-B の R-C（旧 Phase3改）。Linux（特に ARM）でヘッドレスの動作に必要な外部依存
> （.NET 10 / freetype2）を検出し、不足時に導入を案内・支援する。
> 2026-06-05 スコープ裁定＋設計レビュー（Plan agent）＋事実検証（Web）反映済み。
> 関連: `steam-depotdownloader.md`、メモリ `arm-support-plan`・`linux-onboarding-ux-backlog`。

## 0. スコープ

**含む:**

- freetype2 検出＋案内（**全 Linux**。Resonite のネイティブ依存。出典は Wiki の Arch 向け注記
  「未導入だと freetype 不在を訴えてクラッシュする」＝挙動からネイティブ依存と判断。
  Wiki に全 distro の依存リストとしての明記があるわけではない）
- .NET 10 検出＋案内（**linux/arm64 のみ**。x64 は Resonite 同梱 dotnet で完結・実機実証済み。
  GOARCH=arm(32bit) は対象外＝Resonite 自体が非対応）
- distro 判定（`/etc/os-release`）→ パッケージマネージャ別の導入コマンド提示
- 依存案内の 3 経路（①ウィザード末尾の対話 ②起動時ログ ③Web UI 起動時の sys ログ）
- FW/ポート開放の案内（起動時メッセージ・Linux のみ)

**含まない（裁定済み）:**

- 既存 Resonite インストールの自動検出（スキップ。R-A のバンドル既定で価値低下・
  既存利用は installDir 手動指定で足りる）
- chmod 案内（R-E の tar.gz 配布で実行権が保持され不要化。DL 後の Resonite chmod は実装済み）
- 検出が「不明」（検出手段自体が機能しない環境）のときの警告（**案A=黙る**。
  誤警告ゼロを優先・不明環境≒上級者環境・R-B の「判定不能=楽観」と一貫）
- **通常起動での [Y/n] 対話**（H-4 裁定: 対話はウィザード末尾のみ。毎起動の対話は
  tmux/screen 放置運用でサーバー起動をブロックするため不採用）
- 依存の自動導入を非対話で走らせること（sudo を勝手に実行しない）
- Windows / macOS（**完全 no-op**・挙動不変）

## 1. 検出仕様

検出結果は 3 値: **Present**（在ると確認）/ **Absent**（検出手段は機能したが無い）/
**Unknown**（検出手段自体が機能しなかった）。案内を出すのは **Absent のときだけ**。

### 1.1 freetype2（全 Linux）

`libfreetype.so.6` の候補を 2 系統で集め、**「使用可能な候補」があるか**で判定する。

**候補収集:**

1. **`ldconfig -p`**: `exec.LookPath("ldconfig")` → `/sbin/ldconfig` → `/usr/sbin/ldconfig`
   の順で実体を探し、**タイムアウト付き（5s）**で実行。出力行から `libfreetype.so.6` を
   含む行の `=> <path>` を候補に採る（先頭のヘッダ行「NNN libs found…」は自然に無視される）。
2. **既知 lib ディレクトリ走査**（fallback 兼追加候補）: 以下を `os.ReadDir` し、
   `libfreetype.so.6` 前方一致のファイルを候補に採る。
   - 共通: `/usr/lib`, `/usr/lib64`, `/lib`, `/lib64`
   - GOARCH=amd64: `/usr/lib/x86_64-linux-gnu`, `/lib/x86_64-linux-gnu`
   - GOARCH=arm64: `/usr/lib/aarch64-linux-gnu`, `/lib/aarch64-linux-gnu`

**使用可能な候補** = `os.Stat` が成功し（dangling symlink 除外・symlink は実体を追う）、
`elfArch(path)` が実行 GOARCH と一致 **または ""（ELF 判定不能）** のもの
（判定不能は楽観側＝警告しない方向に倒す。R-B と同思想）。

**判定:**

- 使用可能な候補が 1 つ以上 → **Present**
- 検出手段が機能した（ldconfig 実行成功 or 既知ディレクトリが 1 つ以上存在）が、
  使用可能な候補ゼロ → **Absent**（候補はあるが**全て他 arch**、も Absent。
  ARM 機に amd64 の libfreetype だけがある multiarch ケースで案内を出すため）
- ldconfig も実行できず既知ディレクトリも 1 つも無い → **Unknown**（黙る）

**`elfArch` の拡張（R-B 修正）**: 現実装は EM_X86_64/EM_AARCH64 のみで、32bit の
EM_386(3)/EM_ARM(40) が ""＝楽観 Present になり「Fedora の `/usr/lib` は 32bit」対策が
機能しない。**EM_386→"386"・EM_ARM→"arm" をマップに追加**する（GOARCH 表記）。
`dotnetUsable` も 32bit dotnet を正しく弾くようになる（望ましい方向）。
launcher_test.go の既存ケース `{"unknown machine (EM_ARM)", …, ""}` の期待値更新が必要。

> 補足: musl/Alpine の `ldconfig` は `-p` 非対応（scanelf ラッパ・出力空）だが、
> その場合も既知ディレクトリ走査が効くため判定は壊れない。apk を distro マップに
> 足さないのは意図的（Resonite は glibc 前提で Alpine では動かない）。

### 1.2 .NET 10（linux/arm64 のみ）

x64 は Resonite 同梱 dotnet（x64）で起動できるため対象外（CachyOS x64 実機で実証済み）。
判定順:

1. **同梱 dotnet**: `<installDir>/dotnet-runtime/dotnet` が存在し `dotnetUsable()`（R-B・
   ELF arch 一致）→ **Present**（将来 Resonite が ARM 同梱 dotnet を配布しても誤案内しない）。
   未 DL なら存在せず単にスキップ。
2. **システム dotnet 候補の解決**: `~/.dotnet/dotnet`（ExpandHome 適用）を `os.Stat` →
   無ければ `exec.LookPath("dotnet")`。**LookPath も失敗（ErrNotFound）→ Absent**
   （`systemDotnet()` と同順序だが、同関数はパス解決専用で「無い」を表現できないため
   流用ではなく手順を並行実装する）。
3. **候補の arch 検証**: 候補に `elfArch` を適用し、**arch が判明して実行 GOARCH と
   不一致 → Absent**（x64 dotnet を ARM に誤導入したケース。実行しても ENOEXEC で
   確実に失敗するため、導入コマンドの案内が正しい）。
4. **ランタイム列挙**: `<dotnet> --list-runtimes` を**タイムアウト付き（10s）**で実行。
   - stdout に `Microsoft.NETCore.App 10.` で始まる行 → **Present**
     （`Microsoft.AspNetCore.App` は見ない）
   - 実行成功したが 10.x 行が無い → **Absent**（dotnet は在るが必要なランタイムが無い。
     案内コマンドはそのまま有効）
   - 実行失敗 / タイムアウト → **Unknown**（黙る）
   - タイムアウトの根拠: ARM SBC では .NET プロセス起動自体が遅い実績がある
     （dotnet/runtime#10070 等）。10s は防御値。

### 1.3 distro 判定（パッケージマネージャ）

`/etc/os-release` を読み、`ID=` → `ID_LIKE=`（スペース区切り・クォート除去）の順で
トークンを既知 distro 系列に照合する（純関数・テスト対象）。

| 系列（ID / ID_LIKE トークン） | pkgmgr | freetype2 導入コマンド |
|---|---|---|
| `arch`, **`cachyos`** | pacman | `sudo pacman -S freetype2` |
| `debian`, `ubuntu` | apt | `sudo apt install libfreetype6` |
| `fedora`, `rhel`, `centos` | dnf | `sudo dnf install freetype` |
| `suse`, `opensuse`（前方一致 `opensuse-*` 含む）, `sles` | zypper | `sudo zypper install libfreetype6` |
| 上記以外 / os-release 読めず | unknown | 汎用文言（§4）。コマンドは出さない |

- **`cachyos` を ID として直接持つ理由**: CachyOS の `ID_LIKE=arch` 自動付与は
  2025-11-14 の cachyos-hooks 修正以降で、未更新システムでは ID_LIKE が欠けうる
  （CachyOS/distribution#177）。ユーザー実機が CachyOS のため実害直結・1 トークンで回避。
- ⚠️ openSUSE のランタイムパッケージは **`libfreetype6`**（soname 命名）。
  計画メモの旧記載「zypper(freetype2)」は誤りで本仕様で訂正（2026-06-05 Web 確認）。

### 1.4 .NET 10 導入コマンド（全 distro 共通・sudo 不要）

```
curl -fsSL https://dot.net/v1/dotnet-install.sh | bash -s -- --channel 10.0 --runtime dotnet
```

- 公式 dotnet-install.sh（`dot.net` 短縮 URL は 301 リダイレクト・`-L` で追従）。
  `~/.dotnet` 配下にランタイムのみ導入・sudo 不要（公式ドキュメント明記・Web 検証済み）。
- launcher（R-B）の `systemDotnet()` が `~/.dotnet/dotnet` を最優先で拾うため、
  導入後は PATH/DOTNET_ROOT 設定なしで起動経路に繋がる。
- `curl` と `bash` が前提。実行失敗時の文言に「curl が必要です」を含める。

## 2. API 設計（`internal/platform/deps.go` 新設）

```go
// DepIssue は不足している外部依存 1 件と導入手段。
type DepIssue struct {
    Kind     string   // "freetype2" | "dotnet10"
    Title    string   // 表示名（例: "freetype2（Resonite のネイティブ依存）"）
    Commands []string // 導入コマンド（[Y/n] 実行・ログ提示の両方で使う。unknown distro は空）
    Sudo     bool     // sudo を伴うか（文言出し分け用）
}

// CheckHeadlessDeps はヘッドレス動作に必要な外部依存の不足を検出する。
// Absent と確認できたものだけ返す（Present/Unknown は返さない＝案A）。
// goos != "linux" は常に nil（Windows/mac 完全 no-op）。
// installDir は内部で platform.ExpandHome を適用する（呼び出し側の展開漏れを防ぐ）。
func CheckHeadlessDeps(goos, goarch, installDir string) []DepIssue
```

- **テスト seam（H-3）**: 実体は I/O を束ねた非公開 probe を取る内部関数にし、公開関数は
  実 probe を渡す薄いラッパとする。

  ```go
  type depProbe struct {
      lookPath func(string) (string, error)
      stat     func(string) (os.FileInfo, error)
      readDir  func(string) ([]os.DirEntry, error)
      readFile func(string) ([]byte, error)         // /etc/os-release
      runCmd   func(timeout time.Duration, name string, args ...string) (string, error)
      elfArch  func(string) string
      home     string                                // ~ 展開用
  }
  func checkHeadlessDeps(p depProbe, goos, goarch, installDir string) []DepIssue
  ```

  マトリクステスト（§5）は probe 注入で決定的に書く（実ホストの /usr/lib 等に依存しない）。
- 純関数（テスト対象）: `pkgManagerFromOSRelease(content)` / `hasDotnet10(listRuntimesOutput)` /
  候補リストからの freetype 3 値判定。
- `elfArch` / `dotnetUsable` / `ExpandHome`（launcher.go・paths.go）を再利用
  （elfArch は §1.1 の 32bit 拡張を先に行う）。

## 3. 結線（3 経路＋FW 案内）

### 3.1 経路② — 通常起動時はログ表示のみ（`cmd/mrhc/main.go`）

config 読込後・サーバー構築前に `CheckHeadlessDeps` を実行（installDir は
`cfg.InstallDirOrDefault(dir)`）。**tty/非 tty を問わず表示のみで続行**（H-4 裁定:
対話はウィザード末尾だけ。毎起動の [Y/n] はサーバー起動をブロックするため出さない）。

- issue ごとに `log.Printf` で「不足: <Title> / 導入コマンド: <cmd>」（1〜2 行）。
- 同期実行で問題ない（amd64 は stat/ReadDir＋ldconfig 数 ms。arm64 の
  `--list-runtimes` も通常 1s 未満・最悪 10s は壊れた dotnet がある稀ケースのみ）。

### 3.2 経路① — ウィザード末尾の [Y/n] 対話（`internal/setup/wizard.go`）

`RunWizard` の**保存成功後**に検出＋対話を実行（途中 Ctrl-C でも config は残る）。
ARM の初回フロー（install.sh → ./mrhc → ウィザード → 依存導入 → 再起動）がここで完結する。

- dataDir は `filepath.Dir(cfgPath)`・installDir は `cfg.InstallDirOrDefault(dataDir)`
  （ウィザード生成 cfg は Steam=nil なので常に既定値）。`RunWizard` のシグネチャ変更は不要。
- ウィザード時点は Resonite 未 DL なので、ARM では system .NET 10 の有無がそのまま効く
  （正しい挙動: DL 後も同梱は x64 のため system が要る）。
- **tty**: issue ごとにタイトル＋コマンドを表示し `[Y/n]`（既定=Y）。同意なら
  **stdio を端末直結**で `bash -c <cmd>` 実行（sudo のパスワード入力がそのまま機能）。
  実行後に該当依存だけ再検出し「確認できました / まだ確認できません（続行します）」。
  **拒否・失敗・bash 不在でも続行**（ブロックしない。以後は経路②が毎起動思い出させる）。
- **非 tty のウィザード**（パイプ実行）: 表示のみ（経路②と同形）。
- 対話部分は `internal/setup` に置く（platform=検出・setup=対話の責務分離）:

  ```go
  // run/recheck が nil なら実実装（bash -c 実行 / CheckHeadlessDeps 再走）。テストで注入。
  func OfferDepInstall(issues []platform.DepIssue, in *bufio.Reader, tty bool,
      run func(cmd string) error, recheck func(kind string) bool)
  ```

  出力は wizard の流儀どおり stdout 直書き。
- 実装注意（L-6）: `in *bufio.Reader` の先読みバッファと bash 子プロセスへの stdin 直結は
  併用に癖がある（先読み済みバイトは子に届かない）。対話入力なら実害なし・コメントで残す。

### 3.3 経路③ — Web UI 起動時の予防ガイド（`internal/server/server.go`）

`handleStart` の `headless_not_installed` チェック通過後・`driver.Start` 直前に
（linux のみ）チェックを仕掛け、不足があれば **sys ログ**で 1 行/件:

> `freetype2 が見つかりません。起動に失敗する場合はサーバー側で導入してください: sudo pacman -S freetype2`

- **cfgMu の扱い（M-1）**: steamParams の流儀（steam.go）を踏襲——RLock で
  `InstallDirOrDefault` だけ読んで即 Unlock、I/O（検出）はロック外で行う。
- **非同期化（M-1）**: 検出＋`PublishSys` は **goroutine で実行**し start をブロックしない
  （server の「即受付」方針と整合。sys ログの順序が起動ログと前後しても実用上問題ない）。
  テストでは検出関数を seam 注入し同期化して検証。
- 起動は**止めない**・コマンドも**実行しない**（sudo を勝手に走らせない）。
- driver に `PublishSys(text string)` を新設（既存 `publishLog("sys", …)` の薄い公開
  ラッパ。停止中でも安全・history リングに乗るため UI コンソールの初期履歴にも出る）。
- orchestrator / crash 復帰経路は対象外（稼働実績ありの前提・確定済み方針)。
- pre-start 方式（クラッシュ検知連動でなく）= 単純・決定的・クラッシュ前に理由が見える。

### 3.4 FW/ポート開放案内（`cmd/mrhc/main.go`・Linux のみ）

起動時メッセージ（`http://localhost:<port> を開いてください`）の直後に追加:

```
LAN内の別PCからは http://<このPCのIP>:<port> でアクセスできます。
（接続できない場合はファイアウォールで TCP <port> の開放が必要です。
 例: sudo firewall-cmd --permanent --add-port=<port>/tcp && sudo firewall-cmd --reload
     / sudo ufw allow <port>/tcp）
```

- IP の列挙はしない（複数 NIC で混乱するため固定文言）。Windows は OS が接続時に
  自動でプロンプトを出すため対象外。

## 4. 文言（案・UI 文言は事実だけ簡潔に）

- freetype2 不足（ウィザード対話・pacman 例）:

  ```
  ⚠ Resonite の動作に必要な freetype2 が見つかりません。
    導入コマンド: sudo pacman -S freetype2
    今すぐ実行しますか? [Y/n]:
  ```

- .NET 10 不足（ARM・ウィザード対話）:

  ```
  ⚠ ARM Linux では .NET 10 ランタイムが必要ですが、見つかりません。
    導入コマンド（sudo 不要・~/.dotnet に入ります）:
    curl -fsSL https://dot.net/v1/dotnet-install.sh | bash -s -- --channel 10.0 --runtime dotnet
    今すぐ実行しますか? [Y/n]:
  ```

- distro 不明（コマンドを出せない）:

  ```
  ⚠ Resonite の動作に必要な freetype2 が見つかりません。
    お使いのディストリビューションのパッケージマネージャで freetype2
    （Debian系では libfreetype6）を導入してください。
  ```

- 経路②（通常起動）は同内容を 1〜2 行のログに圧縮。経路③は §3.3 の 1 行形式。
- 導入コマンド実行失敗時: 「実行に失敗しました（curl と bash が必要です）。
  上のコマンドを手動で実行してください。」

## 5. テスト

- `pkgManagerFromOSRelease`: cachyos（**ID 直接**・ID_LIKE 欠落ケース）/ ubuntu
  （`ID_LIKE=debian`）/ debian / fedora（**ID_LIKE なし**）/ AlmaLinux・Rocky
  （`ID_LIKE="rhel centos fedora"`）/ opensuse-tumbleweed（`ID_LIKE="opensuse suse"`）/
  未知 ID / クォート / 空文字。
- `hasDotnet10`: 実出力形式（`Microsoft.NETCore.App 10.0.x [path]`）・9.x のみ→false・
  `Microsoft.AspNetCore.App 10.x` のみ→false（NETCore.App だけを見る）。
- freetype 判定: probe 注入で Present / Absent / Unknown ＋ **他 arch 候補のみ → Absent**
  （H-1）＋ dangling symlink 除外（elf fixture は launcher_test の既存ヘルパ再利用）。
- `elfArch` 拡張: EM_386→"386"・EM_ARM→"arm"（launcher_test の既存 EM_ARM ケースの
  期待値を "" から "arm" へ更新）。
- `checkHeadlessDeps` マトリクス（probe 注入で決定的に）: windows→nil / linux/amd64→
  freetype のみ / linux/arm64→freetype＋dotnet / 同梱 usable dotnet で dotnet issue 抑制 /
  ~/.dotnet が他 arch → Absent。
- `OfferDepInstall`: run/recheck を偽注入し Y/n/空入力・実行失敗・非 tty 提示のみ。
- `handleStart` pre-start: 検出 seam を注入（テストは同期化）し sys ログが log hub に
  乗ることを確認。

## 6. 実装順

1. `elfArch` 32bit 拡張（launcher.go＋launcher_test 期待値更新）＋
   `internal/platform/deps.go`＋単体テスト（検出・判定・distro マップ）
2. `internal/setup` 対話提示（OfferDepInstall）＋ wizard.go 末尾結線＋
   main.go 経路②結線＋FW 案内文言
3. `driver.PublishSys`＋`handleStart` pre-start チェック（goroutine）＋テスト
4. docs（DESIGN.md・steam-depotdownloader.md スコープ注記）・メモリ消し込み
   （backlog 項2/項5 消化・項1=R-A 済・項3=R-E 委ね・zypper パッケージ名訂正）

## 7. 確定経緯・出典

- スコープ 3 裁定（既存検出スキップ / chmod=R-E / FW=起動時メッセージ）＋
  経路③=pre-start ＋ 不明時=案A（黙る）＋ **H-4=対話はウィザード末尾のみ**:
  2026-06-05 ユーザー裁定。
- 3 経路・[Y/n]・distro マップの大枠: arm-support-plan 改訂2（ユーザー承認済）。
  H-4 裁定により「①対話 CLI(wizard/tty 起動)」は「①ウィザード末尾のみ」に更新。
- 設計レビュー（Plan agent・2026-06-05）: H-1 第4ケース未定義 / H-2 elfArch 32bit /
  H-3 テスト seam / H-4 対話ブロック / M-1〜M-5 / L-1〜L-7 を反映済み。
- 事実検証（Web・2026-06-05）: dotnet-install.sh（URL/フラグ/`~/.dotnet`/sudo 不要）・
  4 distro パッケージ名・ldconfig 挙動（/sbin 配置・musl 非対応）・os-release
  （cachyos/fedora/Alma/Rocky）・multiarch ディレクトリ・`--list-runtimes` 形式を裏取り。
- freetype2 必須・.NET 10・ARM 事情: Resonite Wiki
  [Headless Server Software/Setup](https://wiki.resonite.com/Headless_Server_Software/Setup)・
  [/ARM](https://wiki.resonite.com/Headless_server_software/ARM)（2026-06-05 再確認）。
- openSUSE パッケージ名 `libfreetype6`: software.opensuse.org（2026-06-05 Web 確認）。
- CachyOS `ID_LIKE=arch` 付与時期: CachyOS/cachyos-hooks `b0f179d`（2025-11-14）・
  CachyOS/distribution#177。
- dotnet-install.sh: [Microsoft Learn](https://learn.microsoft.com/en-us/dotnet/core/tools/dotnet-install-script)
  （`--channel 10.0 --runtime dotnet`・非管理者インストール・既定 `$HOME/.dotnet`）。
- ARM の .NET 起動遅延（10s タイムアウト根拠）: dotnet/runtime#10070・dotnet/core#2122。
