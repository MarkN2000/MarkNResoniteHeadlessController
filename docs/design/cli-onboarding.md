# CLI オンボーディング — 確定仕様（初回ウィザード・起動メッセージ・i18n）

> 2026-06-06 にユーザーと一問一答で全画面の文言・挙動を確定した正本。
> 実装: `internal/setup/wizard.go`・`internal/setup/deps_prompt.go`・`cmd/mrhc/main.go`・
> `internal/i18n/catalog.go`（文言の実体）。文言を変えるときは本ドキュメントとカタログを同時に更新する。
> 関連: `deps-onboarding.md`（S4 の検出仕様）・`steam-depotdownloader.md`（S5b の DL 実体）。

## 0. 設計原則

1. **README を読まなくても画面だけで次の一歩が分かる**（各画面に「これは何のためか」を 1 行）
2. **事実だけ簡潔に**（推測的な警告・脅しは書かない）
3. **ブラウザ操作は別 PC が前提** → URL 案内は LAN を主役、localhost は従
   （IP は複数 NIC で混乱するため列挙しない＝`<このPCのIP>` プレースホルダ固定文言）
4. **空 Enter 連打＝推奨フロー**（全 [Y/n] 既定 Y・入力欄は既定値持ち。
   唯一の例外= S3 の「使用中ポートをこのまま使うか」[y/N] だけ既定 N＝安全側）
5. **何があってもブロックしない**（依存導入・DL は拒否/失敗でも続行。あとから Web UI で
   やり直せる旨を必ず添える）
6. 秘密の扱い: パスワード=伏せ字（tty）／ヘッドレスコード=平文（ウィザードはサーバー起動前で
   Web UI・ログに流れない。保存は 0600 config・API は hasXxx の bool のみ）
7. パイプ実行（自動化）でも同じ質問列で動作し、**EOF は常に安全側**
   （[Y/n]=実行しない・ウィザード全体=中断）

## 1. 入力の契約（全プロンプト共通）

- **不正な値 → 理由を表示して再入力**（黙って既定値や n に倒さない）
- **EOF・読取エラー → `ErrAborted` で中断**（パイプ実行で回答が尽きたとき無限ループしない。
  改行なしの最終行は有効回答として扱う）。S4 の [Y/n] だけは EOF=残り issue の打ち切り
  （直後の S5/S6 プロンプトが中断を扱う）
- **空 Enter → 既定値**（[Y/n] は Y・ポートは 8080・インストール先は既定導出。
  例外= S3 の使用中ポート警告 [y/N] は既定 N＝ポート再入力に戻る）
- S5a の資格欄（ユーザー名/パスワード/コード）の空 Enter は**セクション中止**（脱出ルール）

## 2. フロー

```
mrhc 実行
├─ config あり ──────────────→ S9 通常起動バナー → サーバー稼働
└─ config なし（初回）
     S0 言語選択（両言語併記・既定=OS検出: Windows=GetUserDefaultUILanguage / Linux=LC_ALL→LC_MESSAGES→LANG）
     S1 導入（決める 3 つの提示）
     S2 管理パスワード（×2回）
     S3 Web ポート
     ── config 保存（language 含む。以降で中断しても基本設定は残る）──
     S4 依存チェック  ※Linux のみ・不足時のみ・[Y/n]（R-C 経路①）
     S5 Resonite セットアップ（任意）[Y/n]
     │   ├ Y → S5a Steam資格（空Enterで中止可）→ 資格を config 保存 → S5b DL実行（進捗10%刻み）
     │   │      ├ 成功 → 続行
     │   │      ├ 認証失敗(ErrAuthFailed) → 再入力? [Y/n] → Y で S5a へ戻る
     │   │      ├ ヘッドレスコード誤り(ErrVerifyMissing) → 再入力? [Y/n] → Y で S5a へ戻る
     │   │      │    ※ exit 0 でも headless 実体なし＝public フォールバック検出（H2）。資格の
     │   │      │      入力ミスなので認証失敗と同列・DD は差分再開のため再実行コストほぼゼロ
     │   │      ├ Steam Guard 有効(ErrTwoFactorRequired) → 専用案内（再入力に誘導しない）→ 続行
     │   │      ├ 停滞打切り(ErrStalled) → 専用文言＋Web UI 再試行案内 → 続行（資格ミスではない）
     │   │      ├ Ctrl+C 中止(ErrCancelled/ctx) → 「ダウンロードを中止しました（…再利用されます）」→ 続行
     │   │      └ その他失敗 → 理由表示・Web UI 再試行案内 → 続行（DD は差分再開可）
     │   └ n → 続行
     S6 起動確認 [Y/n]
     │   ├ Y → S7 起動バナー → そのままサーバー稼働（2回起動の廃止）
     │   └ n → S8 あとで起動する案内 → 終了
```

- **途中中断の回収**: S3 保存後はウィザードは二度と出ない（次回=通常起動）。Resonite の準備は
  Web UI（設定→Steam）で完結・依存導入は毎起動ログ（経路②）と起動時 sys 案内（経路③）が
  コマンドを提示し続ける。最初からやり直す最終手段は config 削除（README 記載）。
- **CLI からの DL はウィザード内のみ**。サーバー未起動なので Web UI との衝突は構造的にゼロ。
  独立サブコマンドは作らない（別プロセスは steam.Manager の single-flight を共有できず本当に
  衝突するため）。稼働後の更新は Web UI＋予定再起動時の自動更新。
- **Ctrl+C の捕捉は S5b の DL 実行区間だけ**（`signal.NotifyContext`）。プロンプト読取中に
  張ると Linux で「効いていない」ように見えるため。DD 子プロセスは両 OS ともシグナルが
  直接届くので orphan にならない（調査済み）。

## 3. 画面と文言（正本は `internal/i18n/catalog.go`）

文言の完全な対訳はカタログを参照。ここでは画面例（日本語）と対応キーだけを示す。

### S0 言語選択（カタログ外＝両言語併記の固定文）
```
Language / 言語 [1=English 2=日本語] (2): 
```
不正 → `Please enter 1 or 2. / 1 か 2 を入力してください。`／`(2)` は OS 検出の既定。

### S1 導入 — `wizard.intro`
```
=== MRHC 初回セットアップ ===
Resonite ヘッドレスサーバーの管理ツール MRHC を設定します。
ここで決めるのは次の 3 つです（あとから Web UI で変更できます）:
  1. 管理パスワード   2. Web ポート   3. Resonite 本体の準備（任意）
```

### S2 管理パスワード — `wizard.pw.*`
```
[1/3] 管理パスワード
ブラウザから MRHC にログインするときのパスワードです。
管理パスワード: 
管理パスワード（確認）: 
```

### S3 Web ポート — `wizard.port.*`
```
[2/3] Web ポート
ブラウザから MRHC を開くときのポート番号です。通常はそのままで構いません。
ポート [8080]: 
ポート 8080 は現在使用中のようです。このまま使いますか?（あとで空く予定なら Y） [y/N]: 
```
- 入力直後に空きを事前試験する（`net.Listen`→即 close・`Wizard.PortInUse` が注入点）。
  「使用中」（`platform.IsAddrInUse`）のときだけ警告し、権限不足など他の失敗は黙って通して
  起動時の `main.listenFailed` に任せる（ベストエフォートの早期警告。S5 で 3GB の DL を
  完走した後に「ポート使用中」で死ぬ事故を入力時点で防ぐ）
- 警告は**唯一の既定 N**（空 Enter=ポート再入力に戻る）。Y はあとで空く予定のケース用

### S4 依存チェック — `deps.*`（Linux のみ・不足時のみ・詳細は deps-onboarding.md）
```
⚠ Resonite の動作に必要な freetype2 が見つかりません。
  導入コマンド:
    sudo pacman -S freetype2
  今すぐ実行しますか? [Y/n]: 
```

### S5 Resonite セットアップ — `wizard.resonite.*` / `wizard.steam.*` / `wizard.dl.*`
```
[3/3] Resonite 本体の準備（任意）
Resonite ヘッドレスを Steam からダウンロードします。必要なもの:

  - Steam アカウント
    ⚠ MRHC は二段階認証に対応していないため、Steamガードを
      オフにしたアカウントが必要です。普段使いのアカウントではなく、
      ダウンロード専用の予備アカウントの利用を推奨します。
      オフにする方法: Steam の 設定 → セキュリティ → Steamガードを管理 → Steamガードをオフ

  - ヘッドレスコード
    Resonite 内で bot「Resonite」に /headlessCode と送ると返信されます。

スキップしても、あとから Web UI（設定 → Steam）で実行できます。
今すぐダウンロードしますか? [Y/n]: 

  Steam ユーザー名: 
  Steam パスワード: （伏せ字・ASCII 64 文字以内を検証）
  ヘッドレスコード: （平文）
  インストール先 [<既定>]: （空=既定導出・明示すると config.Steam.InstallDir へ）

ダウンロードを開始します（約 3GB・回線速度により数分かかります）。
  ダウンロードツールを準備中... 完了        ← 「完了」は最初の progress/milestone で
  Resonite をダウンロード中... 10%          ← 10% 刻みの行表示（\r 上書きはパイプを汚すため不使用）
✓ ダウンロード完了
```
- 成否判定は `Manager.Update` の戻り値（result イベントは pubsub 満杯時ドロップがあるため不使用）
- DepotDownloader の利用者向け呼称は「ダウンロードツール」（固有名詞を初回画面に出さない）
- 失敗分岐の文言: 認証失敗=`wizard.dl.authRetry`・ヘッドレスコード誤り=`wizard.dl.verifyRetry`
  （いずれも再入力 [Y/n]）・停滞=`wizard.dl.stalled`・2FA=`wizard.dl.twoFactor`・中止=`wizard.dl.cancelled`・
  その他=`wizard.dl.failed`（err 原文埋め込み。Go エラー文言は ja のため EN CLI では和文混じりが残る
  既知制限＝`steam-depotdownloader.md` §9.1）

### S6 起動確認 — `wizard.saved` / `wizard.start.prompt`
```
設定を保存しました: /home/taro/mrhc-linux-amd64/mrhc.config.json

今すぐサーバーを起動しますか? [Y/n]: 
```

### S7/S9 起動バナー — `banner.*`（S7=ウィザード直後はログイン案内付き）
```
MRHC v2.0.0 を起動しました。
操作する PC のブラウザで http://<このPCのIP>:8080 を開き、管理パスワードでログインしてください。
（この PC で開く場合は http://localhost:8080）
接続できない場合はファイアウォールで TCP 8080 の開放が必要です。例:
  firewalld の場合: sudo firewall-cmd --permanent --add-port=8080/tcp && sudo firewall-cmd --reload
  ufw の場合:       sudo ufw allow 8080/tcp
停止するには Ctrl+C を押してください。
```
- **FW 案内は両 OS**。Windows 版は「初回ダイアログで『アクセスを許可する』・誤ってキャンセル
  した場合は『ファイアウォールによるアプリケーションの許可』で mrhc を許可」（キャンセル時は
  exe 単位のブロックルールが残り、ポート開放コマンドではブロック優先で直らないため、
  コマンド例は意図的に載せない）
- バナー前に `net.Listen` を**同期**で張る。ポート使用中は `main.portInUse` の専用文言
  （判定は `platform.IsAddrInUse`。Windows の実エラーは WSAEADDRINUSE(10048) で
  `syscall.EADDRINUSE` と一致しない）。文言には復旧手段
  「ポートを変えるには mrhc.config.json の "port" を編集」を含む（ウィザードは
  再実行されないため・README「困ったとき」にも同手順を記載）

### S8 あとで起動 — `wizard.start.later`
```
あとで起動するには、もう一度 ./mrhc を実行してください。   （Windows: mrhc.exe）
```

## 4. i18n（多言語化）

- **自前ミニカタログ**（`internal/i18n`・依存ゼロ・ja/en の 2 マップ）。
  `T(lang, key, args...)`。完全性テスト=全キーが両言語を持ち **fmt 動詞の順序つき列**が一致。
- **言語の決定**: S0 で選択（既定=OS 検出）→ `config.language` に保存。
  既存 config（language 無し）の既定は **ja**（既存利用者は全員日本語話者・ユーザー裁定）。
  後から変えるには config を手編集（Web UI に項目は出さない・裁定済み）。
- **適用範囲**: CLI 全文言・起動バナー・経路②（毎起動の依存ログ）・経路③（Web コンソールの
  sys 案内）・reset-password。**対象外**: API エラー（code をフロントが翻訳する既存方式）・
  Web UI 本体（react-i18next が別管理）。
- **シャットダウン系・起動前 fatal・フラグ説明も確定済み（2026-06-06 追補）**:
  シャットダウン3行（config 言語）・fatal 7種＋`-h` フラグ説明（**OS 検出言語**＝config が
  読めない場面のため。日本語 OS 以外はすべて英語）・DL 中の Ctrl+C は専用の中止文言
  （`wizard.dl.cancelled`・steam.ErrCancelled の sentinel 化＋ctx.Err() で acquire 段階も捕捉）。
  カタログの `main.*` を参照。
- ~~残 gap: steam パッケージのエラー文字列（ErrAuthFailed 等）が Web UI の en 表示でも
  日本語のまま出る~~ → **✅ 解消（2026-06-07・sentinel+code 化 d61f44d / Web UI 写像 fb8b567）**。
  正本は `steam-depotdownloader.md` §9.1。

## 5. 検証

- 単体: `wizard_test.go`（happy/再入力統一/EOF 5地点/保存後中断の config 残存）・
  `wizard_steam_test.go`（偽 SteamUpdate で S5 全分岐）・`deps_prompt_test.go`・
  `i18n_test.go`（カタログ完全性）・`lang_test.go`・`addrinuse_test.go`
- 実バイナリスモーク（2026-06-06・Windows）: 日本語/英語フルフロー・EOF 中断 exit 1・
  `-version`・**ウィザード→そのままサーバー起動（S7 バナー）**・同ポート 2 重起動で
  専用文言、をパイプ入力で確認済み
- 確定経緯: 2026-06-06 ユーザーと画面単位で確定（v2 全画面→レビュー指摘→FW 両 OS 化→
  英語版）。実装計画は Plan agent レビュー（H1-3/M1-8 反映）後に承認。
