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

	// ── 依存検出（freetype2 / ARM の .NET 10・R-C）──────────────────
	// Kind が閉じた集合（freetype2/dotnet10）のため "deps.<種別>.<Kind>" 形式で引く。
	"deps.title.freetype2": {
		Ja: "freetype2（Resonite のネイティブ依存）",
		En: "freetype2 (native dependency of Resonite)",
	},
	"deps.title.dotnet10": {
		Ja: ".NET 10 ランタイム（ARM Linux で必要）",
		En: ".NET 10 runtime (required on ARM Linux)",
	},
	"deps.guide.commands": {
		Ja: "導入コマンド: %s",
		En: "Install command: %s",
	},
	"deps.fallback.freetype2": {
		Ja: "お使いのディストリビューションのパッケージマネージャで freetype2（Debian系では libfreetype6）を導入してください。",
		En: "Install freetype2 (libfreetype6 on Debian-based systems) using your distribution's package manager.",
	},
	"deps.fallback.dotnet10": {
		Ja: "ARM Linux には .NET 10 ランタイムが必要です。公式の dotnet-install.sh で導入してください。",
		En: "ARM Linux requires the .NET 10 runtime. Install it with the official dotnet-install.sh.",
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
	// 経路①（ウィザード S4 の [Y/n] 対話・setup）
	"deps.headline.freetype2": {
		Ja: "⚠ Resonite の動作に必要な freetype2 が見つかりません。",
		En: "⚠ freetype2 (required by Resonite) was not found.",
	},
	"deps.headline.dotnet10": {
		Ja: "⚠ ARM Linux では .NET 10 ランタイムが必要ですが、見つかりません。",
		En: "⚠ ARM Linux requires the .NET 10 runtime, but it was not found.",
	},
	"deps.cmdLabel": {
		Ja: "  導入コマンド:",
		En: "  Install command:",
	},
	"deps.cmdLabel.dotnet10": {
		Ja: "  導入コマンド（sudo 不要・~/.dotnet に入ります）:",
		En: "  Install command (no sudo required; installs to ~/.dotnet):",
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
	"deps.runFailed.dotnet10": {
		Ja: "  実行に失敗しました（curl と bash が必要です）: %v",
		En: "  Failed to run (curl and bash are required): %v",
	},
	"deps.runManually": {
		Ja: "  上のコマンドを手動で実行してください。",
		En: "  Please run the command above manually.",
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
		Ja: "ポート %d は既に使用されています。他のソフトが使っていないか確認してください。",
		En: "Port %d is already in use. Check if another program is using it.",
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
}
