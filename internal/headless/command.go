package headless

// command.go は write API（Pre-7c）が Resonite コンソールへ渡す引数を安全に整形する
// 純粋関数を提供する。コンソールは行単位（1 行 = 1 コマンド）なので、引数に生の改行が
// 含まれるとコマンド注入になり得る。ここで一元的に無害化する。
//
// 設計確定: docs/design/phase-7-spec.md §2.4（2026-05-30 仕様レビュー）。
//   - QuoteArg     : 自由文字列（user/message/name/description/url/template/role）用。
//   - SanitizeToken: 引用符を付けない単一トークン（accesslevel のレベル名・unban の userId）用。

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidToken は SanitizeToken が許可文字以外を検出したときに返す。
var ErrInvalidToken = errors.New("headless: 不正なトークン（[A-Za-z0-9_-] のみ・1文字以上）")

// tokenRe は引用なしトークンの許可文字。accesslevel のレベル名（ContactsPlus 等）や
// unban の userId（U-xxxx 形式）はすべてこの範囲に収まる。空白・記号・制御文字は不許可。
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// QuoteArg は自由文字列引数を Resonite コンソールへ安全に渡せる形へ無害化し、
// ダブルクォートで囲んで返す。
//
// 規則（エスケープ解釈は実機未検証のため保守的に strip 優先）:
//   - ユーザー由来の `\` と `"` は除去（strip）。エスケープ（`\"`）に頼らないことで、
//     末尾バックスラッシュによるクォート破壊やコマンド注入を構造的に防ぐ。
//   - 実改行（LF）は 2 文字エスケープ `\n` に変換（我々が出力するので注入にならない）。
//     ※ ユーザーの `\` は先に除去済みなので、ここで入る `\` は我々の `\n` のみ。
//   - CR は除去（`\r\n` は上の LF 変換 + CR 除去で単一の `\n` になる）。
//   - その他の制御文字（< 0x20 / DEL 0x7f）は除去。
//   - 最後に "..." で囲む。
//
// 実機でコンソールのエスケープ規則が判明したら（検証バッチ項目1）方針を見直す。
func QuoteArg(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			// strip（注入・クォート破壊防止）
		case r == '\n':
			b.WriteString(`\n`) // 実改行 → 2 文字エスケープ
		case r == '\r':
			// drop
		case r < 0x20 || r == 0x7f:
			// その他制御文字は除去
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// SanitizeToken は引用符を付けない単一トークン引数を検証し、問題なければそのまま返す。
// `^[A-Za-z0-9_-]+$` 以外は ErrInvalidToken を返す（呼び出し側で 400 にマップ）。
func SanitizeToken(s string) (string, error) {
	if !tokenRe.MatchString(s) {
		return "", ErrInvalidToken
	}
	return s, nil
}
