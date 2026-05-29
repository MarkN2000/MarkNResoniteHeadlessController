# 2026-05-28 LAN + Logged-in 実機採取

Resonite Beta 2026.5.27.1300, Windows, MarkN_headless アカウントログイン状態, LAN 公開セッション 1 個 (MRHC LAN Capture Session)

## 01-lan-joined-anonymous/
ログインなし状態で別PC(MarkN アカウント)から join した直後のスナップショット。
status の users = ["MARKNPC_MAIN", "MarkN_headless"], users コマンドで実 Guest+Admin の行を確認。

## 02-logged-in/
loginCredential 入りで起動した状態。listbans に実エントリ 2 件 (bannedUserB, bannedUserA)、
help-output.txt に全コマンド + Usage、sse-writeops.log に role/silence/respawn/message/kick の
実レスポンス書式を含む。

friendrequests は両方とも空 (Accepted/Ignored 関係は表示されない仕様)。
