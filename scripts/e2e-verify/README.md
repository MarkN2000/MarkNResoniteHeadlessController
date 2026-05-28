# Phase 6: 実 Resonite e2e 検証

実 Resonite ヘッドレスを mrhc 経由で動かし、構造化API（`/api/v1/sessions` 等）の
応答が期待どおりかを検証する。

## 同梱
- `gen-test-config/main.go` — テスト用 `mrhc.config.json` を生成（bcrypt PW + 乱数APIキー）
- `verify.ps1` — オーケストレーション: ビルド → config生成 → mrhc起動 → headless起動 → API叩く → cleanup
- `results/` — 各実行の生成物（gitignore対象。fixture として残したいものだけ別途保存）

## 前提
- Windows 11
- Go 1.26+（PATH or `C:\Program Files\Go\bin`）
- Resonite Headless（既定パス: `C:\Program Files (x86)\Steam\steamapps\common\Resonite\Headless\Resonite.exe`）
- web/dist が既に生成済（`cd web && npm install && npm run build`）
- 本番 mrhc / Resonite が起動していないこと
- ⚠️ PowerShell スクリプトは **UTF-8 BOM 付き**で保存すること（PS 5.1 が日本語コメントを ANSI 誤読すると parser エラー）

## 実行

```powershell
.\scripts\e2e-verify\verify.ps1
```

オプション:
```powershell
.\scripts\e2e-verify\verify.ps1 `
    -Password "my-test-pw" `
    -Port 8081 `
    -ResoniteExe "D:\Steam\..." `
    -TestConfig ".\my-test.json"
```

## 結果

`scripts/e2e-verify/results/<yyyyMMdd-HHmmss>/` に以下が保存される：

| ファイル | 内容 |
|---|---|
| `data/mrhc.config.json` | 生成された設定（bcrypt PW + APIキー） |
| `01-start.json` | `POST /api/v1/start` 応答 |
| `10-sessions.json` | `GET /api/v1/sessions` 応答 |
| `11-session-0-status.json` | World 0 の status |
| `12-session-0-users.json` | World 0 の users |
| `13-session-1-status.json` | World 1 の status |
| `14-session-1-users.json` | World 1 の users |
| `15-listbans.json` | `GET /api/v1/listbans` 応答 |
| (撤去済) | ~~`16-friendrequests.json`~~ → `/api/v1/friendrequests` は撤去（実書式採取不可のため）|
| `90-stop.json` | shutdown 応答 |
| `mrhc.out.log` / `mrhc.err.log` | mrhc 自身のログ |

## 期待される応答（test-multi-world.json 使用時）

- `10-sessions.json`: 2 worlds (`Fake/MRHC Test World A`/`B`)
- `11-session-0-status.json`: `Name: "MRHC Test World A"` + 13 keys（`ResoniteLink: "off"` 含む）
- `12-session-0-users.json`: 1 user (`MARKNPC_MAIN`/`Admin`)
- `15-listbans.json`: `[]`（test config では空）

## Phase 6 で発見された重要バグ（修正済）

旧 `stripPromptPrefix`（応答先頭行のみ剥がす）は、Driver の collector が boot ambient を
大量に含むケースで、応答行が collector 末尾に位置し prompt prefix が付いていると
World 0 が消える問題があった。
→ parser を **per-line `stripLineLeadingPrompts`** に変更して解消（`parser.go`）。
回帰テスト: `parser_test.go::TestParseWorldsHandlesAmbientPlusPromptPrefix`、
`TestParseStatusHandlesAmbientPlusPromptPrefix`。
