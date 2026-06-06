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

	// ── main のエラー ───────────────────────────────────────────
	"main.portInUse": {
		Ja: "ポート %d は既に使用されています。他のソフトが使っていないか確認してください。",
		En: "Port %d is already in use. Check if another program is using it.",
	},
	"main.listenFailed": {
		Ja: "サーバー起動に失敗しました: %v",
		En: "Failed to start the server: %v",
	},
}
