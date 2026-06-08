package i18n

// catalog は全文言。キーは <領域>.<場面> 形式（例 "wizard.intro", "banner.running"）。
// 文言は docs/design/cli-onboarding.md（ユーザー確定仕様）と一致させること。
// 追加時は ja/en の両方を必ず定義する（欠落・fmt 動詞列の不一致はテストが落とす）。
//
// カタログ外の固定文（意図的）: S0 言語選択（言語確定前なので両言語併記をコードに直書き）。
var catalog = map[string]map[Lang]string{
	// ── ウィザード（S1〜S3・S6・S8）─────────────────────────────
	"wizard.intro": {
		Ja: "=== MRHC 初回セットアップ ===\n" +
			"Resonite ヘッドレスサーバーの管理ツール MRHC を設定します。\n" +
			"ここで決めるのは次の 3 つです（あとから Web UI で変更できます）:\n" +
			"  1. 管理パスワード   2. Web ポート   3. Resonite 本体の準備（任意）",
		En: "=== MRHC First-Time Setup ===\n" +
			"This will set up MRHC, a management tool for Resonite headless servers.\n" +
			"You will decide the following 3 things (all can be changed later in the Web UI):\n" +
			"  1. Admin password   2. Web port   3. Resonite installation (optional)",
	},
	"wizard.pw.header": {
		Ja: "[1/3] 管理パスワード\nブラウザから MRHC にログインするときのパスワードです。",
		En: "[1/3] Admin password\nThe password for logging in to MRHC from a browser.",
	},
	"wizard.pw.prompt": {
		Ja: "管理パスワード: ",
		En: "Admin password: ",
	},
	"wizard.pw.confirm": {
		Ja: "管理パスワード（確認）: ",
		En: "Admin password (confirm): ",
	},
	"wizard.pw.empty": {
		Ja: "空のパスワードは設定できません。",
		En: "Password cannot be empty.",
	},
	"wizard.pw.mismatch": {
		Ja: "一致しません。もう一度入力してください。",
		En: "Passwords do not match. Please try again.",
	},
	"wizard.port.header": {
		Ja: "[2/3] Web ポート\nブラウザから MRHC を開くときのポート番号です。通常はそのままで構いません。",
		En: "[2/3] Web port\nThe port for opening MRHC in a browser. The default is fine in most cases.",
	},
	"wizard.port.prompt": {
		Ja: "ポート [%d]: ",
		En: "Port [%d]: ",
	},
	"wizard.port.invalid": {
		Ja: "1〜65535 の数値を入力してください。",
		En: "Please enter a number between 1 and 65535.",
	},
	// 唯一の既定 N プロンプト（使用中と分かっているポートをうっかり確定しない安全側）
	"wizard.port.inUse": {
		Ja: "ポート %d は現在使用中のようです。このまま使いますか?（あとで空く予定なら Y） [y/N]: ",
		En: "Port %d appears to be in use. Use it anyway? (choose Y if it will be free later) [y/N]: ",
	},
	"wizard.saved": {
		Ja: "設定を保存しました: %s",
		En: "Settings saved to: %s",
	},
	"wizard.start.prompt": {
		Ja: "今すぐサーバーを起動しますか? [Y/n]: ",
		En: "Start the server now? [Y/n]: ",
	},
	"wizard.start.later": {
		Ja: "あとで起動するには、もう一度 %s を実行してください。",
		En: "To start it later, run %s again.",
	},
	"wizard.aborted": {
		Ja: "セットアップを中断しました: 入力が読み取れません（EOF）",
		En: "Setup aborted: no more input (EOF).",
	},
	"common.yn.invalid": {
		Ja: "y か n で答えてください。",
		En: "Please answer y or n.",
	},

	// ── ウィザード S5: Resonite セットアップ ───────────────────────
	"wizard.resonite.header": {
		Ja: "[3/3] Resonite 本体の準備（任意）\n" +
			"Resonite ヘッドレスを Steam からダウンロードします。必要なもの:\n" +
			"\n" +
			"  - Steam アカウント\n" +
			"    ⚠ MRHC は二段階認証に対応していないため、Steamガードを\n" +
			"      オフにしたアカウントが必要です。普段使いのアカウントではなく、\n" +
			"      ダウンロード専用の予備アカウントの利用を推奨します。\n" +
			"      オフにする方法: Steam の 設定 → セキュリティ → Steamガードを管理 → Steamガードをオフ\n" +
			"\n" +
			"  - ヘッドレスコード\n" +
			"    Resonite 内で bot「Resonite」に /headlessCode と送ると返信されます。\n" +
			"\n" +
			"スキップしても、あとから Web UI（設定 → Steam）で実行できます。",
		En: "[3/3] Resonite installation (optional)\n" +
			"Download Resonite headless from Steam. You will need:\n" +
			"\n" +
			"  - A Steam account\n" +
			"    ⚠ MRHC does not support two-factor authentication, so the account\n" +
			"      must have Steam Guard turned off. We recommend a spare account\n" +
			"      dedicated to downloads instead of your main account.\n" +
			"      To turn it off: Steam → Settings → Security → Manage Steam Guard → Turn Steam Guard off\n" +
			"\n" +
			"  - A headless code\n" +
			"    In Resonite, send /headlessCode to the bot \"Resonite\" to receive it.\n" +
			"\n" +
			"You can skip this and do it later in the Web UI (Settings → Steam).",
	},
	"wizard.resonite.prompt": {
		Ja: "今すぐダウンロードしますか? [Y/n]: ",
		En: "Download now? [Y/n]: ",
	},
	"wizard.steam.user": {
		Ja: "  Steam ユーザー名: ",
		En: "  Steam username: ",
	},
	"wizard.steam.pw": {
		Ja: "  Steam パスワード: ",
		En: "  Steam password: ",
	},
	"wizard.steam.code": {
		Ja: "  ヘッドレスコード: ",
		En: "  Headless code: ",
	},
	"wizard.steam.installDir": {
		Ja: "  インストール先 [%s]: ",
		En: "  Install directory [%s]: ",
	},
	"wizard.steam.cancelled": {
		Ja: "入力を中止しました。あとから Web UI（設定 → Steam）で設定できます。",
		En: "Cancelled. You can set this up later in the Web UI (Settings → Steam).",
	},
	"wizard.steam.pwInvalid": {
		Ja: "ASCII（半角英数記号）64 文字以内で入力してください。",
		En: "Use ASCII characters only, up to 64 characters.",
	},
	"wizard.dl.start": {
		Ja: "ダウンロードを開始します（約 3GB・回線速度により数分かかります）。",
		En: "Starting the download (about 3 GB; may take several minutes depending on your connection).",
	},
	"wizard.dl.preparing": {
		Ja: "  ダウンロードツールを準備中...",
		En: "  Preparing the download tool...",
	},
	"wizard.dl.preparingDone": {
		Ja: " 完了",
		En: " done",
	},
	"wizard.dl.downloading": {
		Ja: "  Resonite をダウンロード中... %d%%",
		En: "  Downloading Resonite... %d%%",
	},
	"wizard.dl.done": {
		Ja: "✓ ダウンロード完了",
		En: "✓ Download complete",
	},
	"wizard.dl.twoFactor": {
		Ja: "✗ このアカウントは Steam Guard（二段階認証）が有効です。MRHC は二段階認証に\n" +
			"  対応していないため、Steamガードをオフにしたアカウントを使用してください。\n" +
			"  あとから Web UI（設定 → Steam）で設定できます。",
		En: "✗ This account has Steam Guard (two-factor authentication) enabled. MRHC does\n" +
			"  not support it; use an account with Steam Guard turned off.\n" +
			"  You can set this up later in the Web UI (Settings → Steam).",
	},
	"wizard.dl.authRetry": {
		Ja: "認証に失敗しました（パスワード誤り、または Steamガードが有効です）。Steam の情報をもう一度入力しますか? [Y/n]: ",
		En: "Authentication failed (wrong password, or Steam Guard is enabled). Enter your Steam details again? [Y/n]: ",
	},
	"wizard.dl.verifyRetry": {
		Ja: "ダウンロードは完了しましたが headless 本体を取得できませんでした（ヘッドレスコードが誤っている可能性があります）。Steam の情報をもう一度入力しますか? [Y/n]: ",
		En: "The download finished but the headless binary was not retrieved (the headless code may be wrong). Enter your Steam details again? [Y/n]: ",
	},
	"wizard.dl.stalled": {
		Ja: "✗ ダウンロードが停滞したため中断しました（しばらく進捗がありませんでした）。",
		En: "✗ The download stalled (no progress for a while) and was aborted.",
	},
	"wizard.dl.failed": {
		Ja: "✗ ダウンロードに失敗しました: %v",
		En: "✗ Download failed: %v",
	},
	"wizard.dl.retryLater": {
		Ja: "  あとから Web UI（設定 → Steam）で再試行できます。",
		En: "  You can retry later in the Web UI (Settings → Steam).",
	},
	"wizard.dl.cancelled": {
		Ja: "ダウンロードを中止しました（途中までのデータは次回の更新で再利用されます）。",
		En: "Download cancelled (partially downloaded data will be reused next time).",
	},
	"wizard.dl.dotnetInstalling": {
		Ja: "  .NET ランタイムを設置中...",
		En: "  Installing the .NET runtime...",
	},
	"wizard.dl.dotnetFailed": {
		Ja: "✗ Resonite のダウンロードは完了しましたが、.NET ランタイムの設置に失敗しました: %v\n" +
			"  ヘッドレスの起動時に自動で再試行されます。",
		En: "✗ Resonite was downloaded, but installing the .NET runtime failed: %v\n" +
			"  It will be retried automatically when the headless server starts.",
	},

	// ── 起動バナー（S7=ウィザード直後 / S9=通常起動）────────────────
	"banner.running": {
		Ja: "MRHC %s を起動しました。",
		En: "MRHC %s is running.",
	},
	"banner.openLanLogin": {
		Ja: "操作する PC のブラウザで http://<このPCのIP>:%d を開き、管理パスワードでログインしてください。",
		En: "On the PC you want to control from, open http://<this PC's IP>:%d in a browser and log in with the admin password.",
	},
	"banner.openLan": {
		Ja: "操作する PC のブラウザで http://<このPCのIP>:%d を開いてください。",
		En: "On the PC you want to control from, open http://<this PC's IP>:%d in a browser.",
	},
	"banner.localhost": {
		Ja: "（この PC で開く場合は http://localhost:%d）",
		En: "(On this PC: http://localhost:%d)",
	},
	"banner.fw.linux": {
		Ja: "接続できない場合はファイアウォールで TCP %d の開放が必要です。例:\n" +
			"  firewalld の場合: sudo firewall-cmd --permanent --add-port=%d/tcp && sudo firewall-cmd --reload\n" +
			"  ufw の場合:       sudo ufw allow %d/tcp",
		En: "If you cannot connect, open TCP port %d in your firewall. Examples:\n" +
			"  firewalld: sudo firewall-cmd --permanent --add-port=%d/tcp && sudo firewall-cmd --reload\n" +
			"  ufw:       sudo ufw allow %d/tcp",
	},
	"banner.fw.windows": {
		Ja: "接続できない場合: 初回起動時に表示される Windows ファイアウォールのダイアログで\n" +
			"「アクセスを許可する」を選んでください。誤ってキャンセルした場合は、Windows の\n" +
			"検索で「ファイアウォールによるアプリケーションの許可」を開き、mrhc を許可してください。",
		En: "If you cannot connect: choose \"Allow access\" in the Windows Firewall dialog\n" +
			"shown at first launch. If you cancelled it by mistake, search Windows for\n" +
			"\"Allow an app through firewall\" and allow mrhc.",
	},
	"banner.stop": {
		Ja: "停止するには Ctrl+C を押してください。",
		En: "Press Ctrl+C to stop.",
	},

	// ── 依存検出（freetype2・R-C。dotnet10 系キーは自動設置への置換で撤去済み）──
	// Kind が閉じた集合（freetype2）のため "deps.<種別>.<Kind>" 形式で引く。
	"deps.title.freetype2": {
		Ja: "freetype2（Resonite のネイティブ依存）",
		En: "freetype2 (native dependency of Resonite)",
	},
	"deps.guide.commands": {
		Ja: "導入コマンド: %s",
		En: "Install command: %s",
	},
	"deps.fallback.freetype2": {
		Ja: "お使いのディストリビューションのパッケージマネージャで freetype2（Debian系では libfreetype6）を導入してください。",
		En: "Install freetype2 (libfreetype6 on Debian-based systems) using your distribution's package manager.",
	},
	// 経路②（毎起動のログ案内・main）
	"deps.missingLog": {
		Ja: "依存不足: %s / %s",
		En: "Missing dependency: %s / %s",
	},
	// 経路③（起動時の Web コンソール sys 案内・server）
	"deps.sysGuide": {
		Ja: "依存不足: %s — 起動に失敗する場合はサーバー側での導入が必要です。%s",
		En: "Missing dependency: %s — if startup fails, it must be installed on the server. %s",
	},
	// 停止時の自動キャッシュ削除（server・PublishSys）。引数: 件数(%d) / サイズ(%s) / しきい値日数(%d)
	"cache.autoEvicted": {
		Ja: "キャッシュ自動削除: %d 件 / %s を削除しました（%d 日以上前の未更新ファイル）",
		En: "Auto cache cleanup: removed %d files / %s (not modified in over %d days)",
	},
	// 経路①（ウィザード S4 の [Y/n] 対話・setup）
	"deps.headline.freetype2": {
		Ja: "⚠ Resonite の動作に必要な freetype2 が見つかりません。",
		En: "⚠ freetype2 (required by Resonite) was not found.",
	},
	"deps.cmdLabel": {
		Ja: "  導入コマンド:",
		En: "  Install command:",
	},
	"deps.runNow": {
		Ja: "  今すぐ実行しますか? [Y/n]: ",
		En: "  Run it now? [Y/n]: ",
	},
	"deps.skipped": {
		Ja: "  スキップしました（上のコマンドは後で手動実行できます）。",
		En: "  Skipped (you can run the command above manually later).",
	},
	"deps.verified": {
		Ja: "  ✓ 導入を確認しました。",
		En: "  ✓ Installation verified.",
	},
	"deps.notVerified": {
		Ja: "  まだ確認できません（続行します）。",
		En: "  Still not detected (continuing).",
	},
	"deps.runFailed": {
		Ja: "  実行に失敗しました: %v",
		En: "  Failed to run: %v",
	},
	"deps.runManually": {
		Ja: "  上のコマンドを手動で実行してください。",
		En: "  Please run the command above manually.",
	},

	// ── .NET ランタイム自動設置（起動時ガード・server / docs/design/dotnet-runtime.md）──
	"dotnet.sysInstalling": {
		Ja: ".NET ランタイム（%s）が見つからないため設置します（進捗は設定タブの更新ログ）…",
		En: "Installing the .NET runtime (%s) because it was not found (progress in the Settings tab update log)…",
	},
	"dotnet.sysInstalled": {
		Ja: ".NET ランタイムの設置が完了しました。ヘッドレスを起動します。",
		En: ".NET runtime installed. Starting the headless server.",
	},
	"dotnet.sysCancelled": {
		Ja: ".NET ランタイムの設置を中止しました（起動も中止）。",
		En: ".NET runtime installation cancelled (startup aborted).",
	},
	"dotnet.sysUpdateInProgress": {
		Ja: "Steam 更新が進行中のため起動を見送りました。完了後にもう一度起動してください。",
		En: "Startup deferred because a Steam update is in progress. Start again after it finishes.",
	},
	"dotnet.sysFailed": {
		Ja: ".NET ランタイムの設置に失敗しました: %v — 手動導入: %s。起動を試行します…",
		En: "Failed to install the .NET runtime: %v — manual install: %s. Attempting to start anyway…",
	},
	"dotnet.sysStartDeferred": {
		Ja: "Steam 更新が始まったため起動を見送りました。更新完了後にもう一度起動してください。",
		En: "Startup deferred because a Steam update has started. Start again after it finishes.",
	},
	"dotnet.sysStartFailed": {
		Ja: "起動に失敗しました: %v",
		En: "Failed to start: %v",
	},

	// ── reset-password サブコマンド ─────────────────────────────
	"reset.header": {
		Ja: "=== MRHC パスワード再設定 ===\n新しい管理パスワードを設定します。",
		En: "=== MRHC Password Reset ===\nSet a new admin password.",
	},
	"reset.done": {
		Ja: "パスワードを再設定しました。既存のログインセッションは全て無効になりました。",
		En: "The password has been reset. All existing login sessions are now invalid.",
	},

	// ── main のエラー ───────────────────────────────────────────
	"main.portInUse": {
		Ja: "ポート %d は既に使用されています。他のソフトが使っていないか確認してください。\n" +
			"ポートを変えるには mrhc.config.json の \"port\" を編集してください。",
		En: "Port %d is already in use. Check if another program is using it.\n" +
			"To change the port, edit \"port\" in mrhc.config.json.",
	},
	"main.listenFailed": {
		Ja: "サーバー起動に失敗しました: %v",
		En: "Failed to start the server: %v",
	},
	// 以下は config が読めない場面で出るため OS 検出言語（i18n.LangOf(platform.DetectLang())）で表示
	"main.exePathFailed": {
		Ja: "実行ファイルパスの取得に失敗: %v",
		En: "Failed to determine the executable path: %v",
	},
	"main.dataDirFailed": {
		Ja: "データディレクトリの作成に失敗: %v",
		En: "Failed to create the data directory: %v",
	},
	"main.resetNoConfig": {
		Ja: "設定ファイルがありません: %s（先に通常起動して初回セットアップを完了してください）",
		En: "No config file found: %s (run mrhc normally first to complete the initial setup)",
	},
	"main.resetFailed": {
		Ja: "パスワード再設定に失敗: %v",
		En: "Failed to reset the password: %v",
	},
	"main.setupFailed": {
		Ja: "セットアップに失敗しました: %v",
		En: "Setup failed: %v",
	},
	"main.configLoadFailed": {
		Ja: "設定の読み込みに失敗しました: %v",
		En: "Failed to load the config: %v",
	},
	"main.defaultConfigFailed": {
		Ja: "デフォルトconfigの用意に失敗（続行します）: %v",
		En: "Failed to prepare the default headless config (continuing): %v",
	},

	// ── シャットダウン（Ctrl+C・config 言語）──────────────────────
	"main.shutdown.received": {
		Ja: "終了シグナル受信。ヘッドレスを停止しています...（もう一度 Ctrl+C で即終了）",
		En: "Shutdown signal received. Stopping the headless server... (press Ctrl+C again to force quit)",
	},
	"main.shutdown.force": {
		Ja: "強制終了します。",
		En: "Force quitting.",
	},
	"main.shutdown.requestedWeb": {
		Ja: "Web UI から終了が要求されました。ヘッドレスを停止して終了します...（Ctrl+C で即終了）",
		En: "Shutdown requested from the Web UI. Stopping the headless server and exiting... (press Ctrl+C to force quit)",
	},
	"main.shutdown.done": {
		Ja: "終了しました。",
		En: "Shutdown complete.",
	},

	// ── フラグ説明（mrhc -h・OS 検出言語）──────────────────────────
	"main.flag.data": {
		Ja: "データディレクトリ（config/state置き場。既定: 実行ファイルと同じ場所）",
		En: "data directory (config/state location; default: same folder as the executable)",
	},
	"main.flag.version": {
		Ja: "バージョンを表示して終了",
		En: "print version and exit",
	},

	// ── 自己更新（mrhc update・起動時ログ）─────────────────────────
	"main.update.checking": {
		Ja: "更新を確認しています...",
		En: "Checking for updates...",
	},
	"main.update.upToDate": {
		Ja: "既に最新です（%s）。",
		En: "Already up to date (%s).",
	},
	"main.update.devBuild": {
		Ja: "リリースビルドではないため更新できません（version: %s）。",
		En: "This is not a release build, so it cannot be updated (version: %s).",
	},
	"main.update.noRelease": {
		Ja: "リリースが見つかりません。公開状況を確認してください: %s",
		En: "No release found. Check the releases page: %s",
	},
	"main.update.downloading": {
		Ja: "%s から %s へ更新します。ダウンロードしています...",
		En: "Updating from %s to %s. Downloading...",
	},
	"main.update.done": {
		Ja: "更新が完了しました。次回起動時から %s になります。",
		En: "Update complete. %s takes effect the next time MRHC starts.",
	},
	"main.update.busy": {
		Ja: "別の更新が進行中です。完了を待って再実行してください。",
		En: "Another update is already in progress. Wait for it to finish and try again.",
	},
	"main.update.permission": {
		Ja: "設置ディレクトリに書き込めません（%v）。root で設置した場合は sudo chown -R で所有者を変更するか、root で mrhc update を実行してください。",
		En: "Cannot write to the install directory (%v). If it was installed as root, change the owner with sudo chown -R, or run mrhc update as root.",
	},
	"main.update.failed": {
		Ja: "更新に失敗しました: %v",
		En: "Update failed: %v",
	},
	"main.updated": {
		Ja: "MRHC は %s に更新されました。",
		En: "MRHC was updated to %s.",
	},
}
