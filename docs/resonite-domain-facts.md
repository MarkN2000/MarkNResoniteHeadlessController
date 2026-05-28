# Resonite ドメイン事実台帳

このドキュメントは、作り直しにあたって**実装の設計判断とは独立に守るべき「外部の事実」**（Resoniteヘッドレス／SteamCMD／OSの仕様）を、現行コードから抽出してまとめたものです。新実装のテストフィクスチャの土台になります。

> ⚠️ **重要**: 以下の出力書式・正規表現は「現行コード＝素人実装が観測した内容」です。**コマンド名は公式Wikiが一次情報源**（下記）。ただし**出力書式（stdoutの実テキスト）はWikiに無い**ため、実機採取で検証・フィクスチャ化が必要です。
>
> **一次情報源（コマンド一覧の権威）**: 公式Wiki <https://wiki.resonite.com/Headless_server_software/Commands>。コマンドは**大文字小文字を区別しない**（現行コードは小文字で動作）。

---

## 1. プロセス起動・I/O・エンコーディング

| 項目 | 事実 | 出典 |
|---|---|---|
| 起動 | `spawn(HEADLESS_EXECUTABLE, ['-HeadlessConfig', <configPath>], { cwd: dirname(exe) })` | processManager.ts:236-250 |
| 実行ファイル | Windows=`Resonite.exe`。**Linux=`dotnet Resonite.dll`（要.NET10 Runtime、`Headless/`フォルダ内）**。両OSとも同一.NETアプリ。公式Wiki Setup参照 | config/index.ts / Wiki |
| 既定パス(Linux) | `~/.local/share/Steam/steamapps/common/Resonite/Headless/Resonite.dll`（実機確認済）。Flatpak版は `~/.var/app/com.valvesoftware.Steam/.local/share/Steam/...` も候補 | 実機 |
| dotnet(Linux) | **Resonite同梱**: `~/.local/share/Steam/steamapps/common/Resonite/dotnet-runtime/dotnet`（実機確認済）。**別途.NET導入は不要**。起動は `<同梱dotnet> Resonite.dll`、cwd=`Headless/` | 実機 |
| 既定パス(Win) | `C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe` | 旧コード |
| stdin送信 | コマンド文字列＋`\n` を **Shift_JISエンコード**して書き込み | processManager.ts:330 |
| stdout復号 | utf8試行 → 失敗時shift_jis → 最終utf8 | processManager.ts:503-513 |
| ⚠️ エンコーディング | stdin固定Shift_JISは**Windows日本語ロケール前提**。Linux/UTF-8ヘッドレスではUTF-8が必要 → **ロケール検出で吸収すべき** | — |
| 正常停止 | `shutdown`送信 → 60秒待ち → SIGTERM → 70秒でSIGKILL | processManager.ts:485+ |

### コマンド完了の判定（重要なドメイン挙動）
Resoniteヘッドレスは**構造化レスポンスを返さない**。コマンド送信後の完了判定は以下のヒューリスティック：
- **タイムアウト**（コマンド毎に既定3000ms、status=4000、invite/role/accesslevel=5000、startworldurl=30000）
- **プロンプト検出**: 末尾が `>` の行＝フォーカス中セッションのプロンプト（例 `<sessionName>>`）。`stopWhen`で早期終了。
- **データ後プロンプト検出**: データ行（例: usersの`... ID: ...`）が出た後のプロンプトで終了（`createPromptAfterDataDetector`）。
- 一致後 `settleDurationMs`（80〜500ms）待って出力を確定。

### 出力の前処理（パース前に共通適用）
`\r`除去 → 各行trimEnd → 空行除去 → **`>`で始まる行（コマンドエコー）除去** → 改行結合。（headlessParsers.ts:6-19 / serverRoutes.ts:372-384）

---

## 2. Resonite コンソールコマンド一覧

### 取得系（read）

| コマンド | 用途 | 出力書式（例） |
|---|---|---|
| `worlds` | セッション一覧 | `[0] セッション名(空白パディング)\tUsers: 2\tPresent: 1\tAccessLevel: LAN\tMaxUsers: 16` ✅ 実機確定 |
| `focus <index>` | セッションにフォーカス（以降の操作対象を切替） | silent（プロンプトのみ変化） |
| `status` | フォーカス中セッションの詳細 | `Key: Value` の複数行（下記、13 keys） ✅ 実機確定 |
| `users` | フォーカス中セッションのユーザー一覧 | `<name>\tID: <id\|空>\tRole: <role>\tPresent: True\tPing: <int> ms\tFPS: <float>\tSilenced: True/False` ✅ 実機確定（**TAB区切り**） |
| `sessionURL` | セッションURL取得 | `res-steam://.../S-xxxx` 等 |
| `listbans` | BAN一覧 | `[N]\tUsername: <X>\tUserID: U-<id>\tMachineIds: <id1> <id2> ...` ✅ 実機確定（**TAB区切り**、`[N]`と`Username:`の間も TAB） |
| `friendrequests` | 受信フレンド申請一覧 | ⚠️ **incoming pending のみ表示**（Accepted/Ignored 関係は出ない、2026-05-28 LAN実機確定）。pending エントリの実書式は **採取できず未確定** |
| `help` | 全コマンドの一覧 + Usage を表示 | 165行（usage 付きリファレンス）⭐ 実機発見 |
| `version` | ヘッドレスバージョン文字列 | — |
| `debugWorldState` | 全ワールドの診断情報 | — |

**`status` のKey一覧（2026-05-28 Windows Beta 2026.5.27.1300 実機採取で再検証）**: `Name`, `SessionID`, `Current Users`, `Present Users`, `Max Users`, `Uptime`, `Access Level`, `Hidden from listing`(bool), `Mobile Friendly`(bool), `Description`, `Tags`(カンマ区切り), `Users`(カンマ区切り), **`ResoniteLink`** ⭐**NEW（旧コードに無い・要追加）**。

### 操作系（write）

| コマンド | 用途 | 成功時出力 |
|---|---|---|
| `invite "<username>"` | フレンドを招待 | 旧コード推測 `Invite sent!` — **実機未確定**（in-session 相手だと ambient のみ。実 friend に未join 状態への送信は未検証） |
| `accesslevel <Private\|LAN\|Friends\|Anyone>` | アクセスレベル変更 | `World <name> now has access level <Level>` ✅ 実機確定 |
| `role "<username>" "<role>"` | ロール変更（Admin/Builder/Moderator/Guest/Spectator） | `<username> now has role <Role>!` ✅ 実機確定（全 5 role） |
| `kick "<username>"` | キック | **複数行**：`KickRequest: KickAndRevokeInvite for User ID... - UserName: X, UserId: U-X, MachineId: ..., Role: Y. Changing User: , ScheduledForValidation: True` + **UniLog stack trace** ⚠️ノイジー |
| `ban "<username>"` | BAN（実機未検証） | — |
| `silence "<username>"` / `unsilence "<username>"` | ミュート/解除 | **複数行**：`Silence: True/False for User ID... - UserName: X, UserId: U-X, MachineId: ..., Role: Y. Changing User: ...` + **UniLog stack trace** ⚠️ノイジー |
| `respawn "<username>"` | リスポーン | **複数行**：`Destroying User: User ID... - UserName: X, ...` + UniLog stack trace ⚠️ノイジー |
| `unban <userId>` | BAN解除（**userIdは引用符なし**） | 実機未検証 |
| `acceptfriendrequest "<username>"` | フレンド申請承認 | 失敗時: `There's no friend request from this user` ✅実機確定 / 成功書式は未確定 |
| `sendFriendRequest "<username>"` | フレンド申請送信 | `Contact request sent to <userId>.` ✅実機確定 |
| `removeFriend "<username>"` | フレンド削除 | `<userId> has been removed from your contacts.` ✅実機確定 |
| `message "<username>" "<text>"` | **個別DM送信**（※セッション全体ブロードキャストではない） | `Message sent!` ✅実機確定 |
| `name "<newName>"` | フォーカス中セッション名変更 | **silent（出力なし）・新プロンプトが新名に変化**で識別 ✅実機 |
| `maxusers <n>` | 最大人数変更 | **silent（出力なし）** ✅実機 |
| `hideFromListing <true\|false>` | listing 表示切替 | `World <name> will now show in listing` / `... will now be hidden from listing` ✅実機確定 |
| `description "<text>"` | ワールド説明変更 | 実機未検証 |
| `startworldurl "<url>"` | URLから新規セッション開始（プロンプトまで待機） | — |
| `save` / `close` / `restart` | フォーカス中セッションの保存/閉じ/再起動 | 実機未検証 |
| `crash` | デバッグ用：プロセスを即クラッシュ | プロセス終了（ProcessGone テストに利用可能）⭐ |
| `shutdown` | ヘッドレス全体の正常終了 | `Exiting. Save Homes: <bool>` 以降の終了ログ多数 |

> ⭐ **silent成功（出力なし）コマンドの存在**（2026-05-28 採取）: `name`/`maxusers`/`focus`/空の `listbans`/空の `friendrequests` などは**応答行をまったく返さず、次のプロンプトのみ出る**。構造化Driver の完了検出は「プロンプト末尾＋安定窓」なので silent でも問題なく検出できる（応答=空行リスト）。パーサ側は空応答=成功として扱う。

> 💡 **「チャット予告」アクションの実体**: Resoniteヘッドレスには「セッション全員へのチャット一斉送信」コマンドが見当たらず、現行は **`users`で全員を列挙 → 各人へ `message "<user>" "<msg>"`** を送るループで実現。新実装の `chatWarning` も同方式になる見込み（要実機確認）。

> 💡 **操作は状態を持つ**: 多くの操作は「先に `focus <index>` してから対象セッションに対して実行」する前提。シーケンス管理が必要。

### コード内不整合 → 公式Wikiで決着済

| 機能 | 正（実機 help コマンド） | 誤 |
|---|---|---|
| アイテムスポーン | **`spawn <url> <active> <persistent>`**（**3 引数**、2026-05-28 help 確定） | 旧コード `spawn <url> <active>` の **2 引数は誤り** |
| ダイナミックインパルス | **`dynamicImpulse <tag>` / `dynamicImpulseString <tag> <value>` / `dynamicImpulseInt` / `dynamicImpulseFloat`**（値付きは`dynamicImpulseString`。restartManager側が正） | frontendの`dynamicimpulse <tag> <text>`（値を送るなら`dynamicImpulseString`を使う） |
| チャット全体送信 | **存在しない**。`message <friend name> <message>` の**個別DMのみ（フレンド宛）**。セッション内告知は実質「全員へDM」か「`dynamicImpulse`系でワールド内の通知機構を起動」 | — |

### 公式Wikiにある追加コマンド（現行コード未使用・新実装で活用可）

- **ワールド**: `description <text>`（説明変更）, `hideFromListing <true/false>`, `awayKickInterval <minutes>`, `startWorldTemplate <template name>`
- **セッション/ID**: `sessionID`, `copySessionURL`, `copySessionID`
- **BAN拡張**: `banByName <username>` / `unbanByName <username>` / `banByID <userId>` / `unbanByID <userId>`
- **フレンド**: `sendFriendRequest <name>`, `removeFriend <name>`, `login`, `logout`
- **アセット**: `import <path|URL>`, `importMinecraft <folder>`
- **システム**: `saveConfig [filename]`, `gc`（GC実行）, `tickRate <tps>`, `version`, `log`, `debugWorldState`

---

## 3. パース正規表現（現行コードの実値）

```
# worlds（API側 headlessParsers.ts:39）
/^\[(?<index>\d+)\]\s+(?<name>.+?)\s+Users:\s+(?<users>\d+)[\s\t]+Present:\s+(?<present>\d+)[\s\t]+AccessLevel:\s+(?<access>\S+)[\s\t]+MaxUsers:\s+(?<max>\d+)/i

# users（serverRoutes.ts:444。⚠️ idは空文字あり=`\S+`では取れないので `(?:\S+)?` 等で修正要）
/^(?<name>\S+)\s+ID:\s+(?<id>\S+)\s+Role:\s+(?<role>\S+)\s+Present:\s+(?<present>True|False)\s+Ping:\s+(?<ping>[0-9.]+)\s+ms\s+FPS:\s+(?<fps>[0-9.]+)\s+Silenced:\s+(?<silenced>True|False)$/i

# status / worlds詳細（Key: Value 行）
/^([^:]+):\s*(.*)$/

# listbans（serverRoutes.ts:742）
/^\[(?<index>\d+)\]\s+Username:\s+(?<username>\S+)\s+UserID:\s+(?<userId>U-[A-Za-z0-9_-]+)\s+MachineIds:\s+(?<machineIds>.*)$/i

# accesslevel成功（headlessParsers.ts:145）
/now has access level\s+(\S+)/i

# role成功（headlessParsers.ts:161）
/(\S+)\s+now has role\s+(\S+)/i      # role側は末尾の "!" "." を除去

# sessionURL（headlessParsers.ts:179,183）
/(res[-\w]*:\/\/[^\s]+)/i  →  /S-([a-f0-9-]+)/i
```

> ⚠️ **既知の脆さ**: `users`/`listbans` のユーザー名が `\S+`＝**空白を含むユーザー名で破綻**。worldsの名前は `.+?` だが**タブ/空白の並びに依存**。出力書式は**Resoniteのバージョン更新で変わりうる**。Go移植時はRE2（先読み・後方参照なし）で問題ないことを確認（現行に先読みは無し）。名前付きグループは `(?P<name>…)`。

---

## 4. SteamCMD（更新まわり）

> v1方針では「更新実行＋ログ表示のみ」に簡素化し、buildid事前チェック機構は廃止予定。ただし更新実行の呼び出し事実は以下を踏襲。

- **App**: Resonite `appId=2519830`、`branch=headless`
- **更新実行**: `+force_install_dir <dir> +login <u> <p> [+set_steam_guard_file <f>] +app_update 2519830 [-beta headless -betapassword <code>] validate +quit`
- ⚠️ **`+force_install_dir` は必ず `+login` より前**（順序ミスで `please use force_install_dir before logon!`）
- **Steam Guard**: `+login <u> <p> <guardCode>` の3引数目。検出キーワード `steam guard` / `two-factor code`
- **エラーキーワード**: `invalid password`, `no subscription`, `no licenses`, `rate limit exceeded`, `failed to install`, `failed to set beta` 等
- 実行: `spawn(steamcmdPath, args, { cwd: dirname, windowsHide: true })`、既定タイムアウト5分
- （事前チェックを残す場合のみ）`+app_status` のローカルbuildid と `+app_info_print` のVDF `depots.branches.<branch>.buildid` を比較。`appcache/appinfo.vdf` を事前削除して古いキャッシュ回避。
- Windows既定パス候補: `C:/steamcmd/steamcmd.exe`, `C:/Program Files (x86)/Steam/steamcmd/steamcmd.exe`。Resonite既定: `C:/Program Files (x86)/Steam/steamapps/common/Resonite`。**Linuxは別パス → 抽象化必須**

---

## 5. ヘッドレス設定ファイル（公式スキーマ）

`$schema = https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json`

- **トップレベル**: `comment`, `universeId`, `tickRate`(60), `maxConcurrentAssetTransfers`(128), `usernameOverride`, `loginCredential`, `loginPassword`, `startWorlds[]`, `autoSpawnItems[]`
- **startWorlds[] の主なキー**: `isEnabled`, `sessionName`, `customSessionId`, `description`, `maxUsers`, `accessLevel`, `useCustomJoinVerifier`, `hideFromPublicListing`, `tags`, `mobileFriendly`, `loadWorldURL`, `loadWorldPresetName`(Grid等), `overrideCorrespondingWorldId`, `forcePort`, `enableResoniteLink`, `forceResoniteLinkPort`, `keepOriginalRoles`, `defaultUserRoles`, `roleCloudVariable`, `allowUserCloudVariable`, `denyUserCloudVariable`, `requiredUserJoinCloudVariable`, `requiredUserJoinCloudVariableDenyMessage`, `awayKickMinutes`(-1), `parentSessionIds`, `autoInviteUsernames`, `autoInviteMessage`, `saveAsOwner`, `autoRecover`

> 公式スキーマが正なので、config生成は**スキーマ準拠を機械的に保証**するのが望ましい（手書きの入れ子構造をやめる）。

---

## 6. 残る「実機採取」項目（コマンド名はWikiで確定済、残るは出力書式）

コマンド名は公式Wikiで確定したので、残る不明点は **stdoutの実出力書式** のみ。24/7ヘッドレスで以下を叩いて生出力を貼れば、台帳が「検証済みフィクスチャ」に格上げできます：

1. `worlds` / `status` / `users` / `listbans` / `friendRequests` の**生出力**（パーサのフィクスチャ）
2. 各操作系コマンドの**成功/失敗時の出力**（kick/ban/silence/respawn/unban/acceptFriendRequest/invite/accessLevel/role）
3. Linuxの**文字コード**（UTF-8想定でよいか）。起動方法は判明＝`dotnet Resonite.dll -HeadlessConfig <f>`（.NET10 Runtime、`Headless/`内）
4. （任意）`spawn` / `dynamicImpulseString` 実行時の出力

---

## 7. 実機確認済（PoC, Resonite Beta 2026.5.20.291 / .NET 10.0.8 / Linux CachyOS）

Go PoCで本物のヘッドレスに対して検証した事実：
- **起動**: 同梱dotnetで `dotnet Resonite.dll`（無config）が起動し "Engine Ready!" → "World running..." まで到達。低スペックCPUで**初期化に約20秒**（途中の "Engine has been unresponsive for over 10.00 seconds" は正常な自己警告）。
- **プロンプト**: フォーカス中セッション名＋`>`。例 `markn-linux-test World 0>`。**改行なしで次出力の行頭に連結**する。
- **`worlds` 実出力（§3のregex一致を確認）**: `[0] markn-linux-test World 0        Users: 1\tPresent: 0\tAccessLevel: Anyone\tMaxUsers: 16`（区切りは空白＋タブ混在）。
- **未知コマンド**: `Unknown command` を返す。
- ⚠️ **起動直後の最初の1コマンドが `Unknown command` になる事象を観測**（同コマンドの2回目は正常）。エンジンが完全に応答可能になる前の入力が無視/誤処理される可能性 → v1では **readiness合図（"Engine Ready!"/"World running..."）を待ってからコマンド受付**、または初回にダミー改行を送る等を検討。
- **`shutdown`**: `Exiting. Save Homes: False` → 設定保存 → 公開セッションは `BroadcastSessionEnded ... to Public` で閉じ → プロセスは**正常終了(exit 0)**。停止時に `UniLog.Log` 由来のスタックトレースが出るが**エラーではない**（RequestShutdown記録ログ）。
- **無config起動はワールドが公開(Private→Anyone・public listing)になる** → v1は必ずconfigで適切なaccessLevelを設定。

### LAN実機・別PC join・logged-in 採取（2026-05-28 後追い, Beta 2026.5.27.1300）

fixture: `scripts/empirical-capture/fixtures/2026-05-28-lan-login/`

**実証された事実**:
1. **ParseListBans 実機確定** — 実 BAN 2 件で書式確認（**TAB 区切り**で `[N]\tUsername:...\tUserID: U-...\tMachineIds: ...`）
2. **ParseUsers 実機確定** — Guest+Admin 両 role、実 UserID `U-1NzqeqewOpM`、実 FPS 値
3. **stripLineLeadingPrompts 実機確定** — **3 連プロンプト累積**（`MRHC LAN Capture Session>MRHC LAN Capture Session>MRHC LAN Capture Session>[0]...`）で per-line strip が機能
4. **status.users カンマ区切り** — `MARKNPC_MAIN, MarkN_headless` 形式確定
5. **write op 実書式採取**: role/silence/respawn/kick/message/removeFriend/sendFriendRequest/acceptFriendRequest（前節の表参照）
6. **help コマンド完全取得** — 165 行のリファレンス（fixture: `02-logged-in/help-output.txt`）

**新発見コマンド**:
- `help` ⭐ — authoritative リファレンス（usage 付き）
- `crash` ⭐ — デバッグ即クラッシュ（ProcessGone テスト用）

**friendrequests の限界 (重要)**:
- Resonite の `friendrequests` は **incoming pending** のみ表示
- Accepted/Ignored 関係は表示されない
- → テスト環境では Accepted 状態への遷移後 pending エントリが残らず**実書式の採取が原理的に不可能**
- 我々の実装では `ParseFriendRequests` および `/api/v1/friendrequests` を**撤去**（実書式に基づかない heuristic 実装を放置しない方針）

**`/command` の API 上の仕様注意**:
- POST body は **JSON のみ** (`{"cmd":"X"}`)
- form-urlencoded body (`cmd=X`) は受け付けない → 400
- 推奨は URL query (`?cmd=X`) または JSON body

### Windows実機・複数ワールド・コマンド網羅採取（2026-05-28, Beta 2026.5.27.1300, .NET 10.0.8）

`scripts/empirical-capture/` のスクリプトで以下を採取（fixture: `scripts/empirical-capture/fixtures/2026-05-28-windows-multiworld.log`）：

- **複数ワールド起動**（test-multi-world.json で `MRHC Test World A/B` 各 Grid プリセット・Private）
- 採取コマンド: `worlds` / `focus 0` / `status` / `users` / `focus 1` / `status` / `users` / `name "Renamed"` / `worlds` / `accesslevel LAN` / `maxusers 8` / `listbans` / `friendrequests` / `dummycommand`（不明）/ `shutdown`

**検証された出力書式**（旧コードregexと突合）：
- `worlds`: `[<idx>] <name padded with spaces>\tUsers: N\tPresent: N\tAccessLevel: <L>\tMaxUsers: N` — name の右側に空白パディング＋TAB区切り。**旧 regex `\s+` で吸収可能**
- `users`: `<Name>\tID: <id>\tRole: <r>\tPresent: <b>\tPing: N ms\tFPS: <f>\tSilenced: <b>` — **ID は無アカウント時 空文字**（旧 regex `\S+` だと空にマッチしない → 修正要）
- `status`: 13行、**新たに `ResoniteLink: off` を末尾に出力**（旧 Key 一覧に無いので追加要）
- `accesslevel <L>`: `World <name> now has access level <L>` — 旧 regex で OK
- `Unknown command`: 不明コマンドで 1 行

**プロンプト挙動の実機確認**：
- 形式: `<world_name>>`（例 `MRHC Test World A>`, `Renamed>`）
- **改行なし**で次の出力行に連結（前から既知）
- ⭐ **プロンプト累積**: 応答ゼロの silent コマンドを**待たずに連射**すると、次に何か出力する行に**プロンプトが N 個連続して現れる**（例 `Renamed>Renamed>Renamed>Renamed>Unknown command`）。これは「応答待ちしない」場合のみ起こる現象で、**構造化Driver は直列＋待機なので発生しない**。

### Windows実機・文字コード（要対応）
- **Windows実機**: Resoniteヘッドレスは既定パス `C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe` に存在（開発PCで確認）。Steamライブラリ: `C:/Program Files (x86)/Steam` と `D:/SteamLibrary`。
- ⚠️ **文字コード（最重要のクロスプラットフォーム課題・実機確認済）**: **両OS実機確定: Windows=Shift_JIS(CP932) / Linux=UTF-8**（OSで異なる）。Linuxは `name "日本語テスト"`→`worlds` で `[0] 日本語テスト ...` が正しく往復（stdin/stdout両方UTF-8）。Windowsは実ヘッドレスの日本語セッション名 `mmc25 SUMA 作業セッション`／`グリッドスペース` がUTF-8読みで `��ƃZ�b�V����` 等に化け、`iconv -f SHIFT_JIS` で正しく復号できた。旧MRHC(Node)がShift_JISを使っていたのと一致。**現Go実装はUTF-8固定なのでJP Windowsで確実に文字化けする** → Encoding抽象が必須。stdin側も同様にコードページでエンコードが必要。
  - 対策: プラットフォームに **Encoding抽象**（Win=コードページ/Linux=UTF-8）を実装。Windowsのコードページは `GetACP`/`GetConsoleOutputCP`(x/sys/windows)で取得し x/text/encoding で変換、または設定で上書き可能に。
  - 検証法: 起動ログは英語(ASCII)で顕在化しないため、`name "日本語テスト"`（stdin検証）→`worlds`/`status`（stdout検証）で日本語が往復するか確認する。
- ⚠️ **`Headless/Config.json` は"ライブ"設定になりうる**: `-HeadlessConfig` を付けずに起動すると Headlessフォルダの `Config.json` を自動ロードする。実Windows機ではこれが**ユーザーの実アカウント認証＋実セッション定義**で、**no-config起動で実アカウントにログインし実セッションを開始してしまった**（**これはOS差ではなくConfig.jsonの内容差**。同一バイナリ・同一挙動で、Linux検証機はたまたまConfig.jsonが空だっただけ。Linuxでも実configを置けば同様にログイン＆実セッション起動する）。→ **MRHCは必ず明示的なconfigで起動し、テストでもno-configに依存しない**。
- **Windows起動コマンド**: `Resonite.exe -HeadlessConfig <f>`（cwd=`Headless/`、直接実行）。.NETランタイムは同梱想定（Linux同様 dotnet-runtime/）。
- ⚠️ **単一インスタンス制約（実機確認）**: 同じ **Data path** で2つ目のヘッドレスを起動すると `DuplicateInstanceException` で**起動側が即終了**する（`A duplicate copy of FrooxEngine is running`）。→ MRHCは同一Data pathで**重複起動しない**こと（start前に稼働中チェック、クラッシュ復帰でも二重起動回避）。また、**コントローラをkillしてもヘッドレス子プロセスは生き残る(orphan)**ので、停止は必ず `shutdown` 送信→終了確認で行う（強制killは最終手段）。
