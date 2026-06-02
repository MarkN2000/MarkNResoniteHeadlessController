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
	"fmt"
	"regexp"
	"strings"
)

// ErrInvalidToken は SanitizeToken が許可文字以外を検出したときに返す。
var ErrInvalidToken = errors.New("headless: 不正なトークン（[A-Za-z0-9_-] のみ・1文字以上）")

// tokenRe は引用なしトークンの許可文字。accesslevel のレベル名（ContactsPlus 等）や
// unban の userId（U-xxxx 形式）はすべてこの範囲に収まる。空白・記号・制御文字は不許可。
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// quote は自由文字列引数を Resonite コンソールへ安全に渡せる形へ無害化し、
// ダブルクォートで囲んで返す共通実装。newlineRepl は実改行(LF)の置換先。
//
// 規則（実機確認済み 2026-05-30）:
//   - ユーザー由来の `\` と `"` は除去（strip）。エスケープに頼らないことで、
//     末尾バックスラッシュによるクォート破壊やコマンド注入を構造的に防ぐ。
//   - 実改行（LF）は newlineRepl に置換（生改行は送らない＝行ベースのコンソールで注入されない）。
//   - CR は除去（`\r\n` は LF 側で 1 回だけ置換される）。
//   - その他の制御文字（< 0x20 / DEL 0x7f）は除去。
//   - `<` `>` `=` `/` 等はそのまま（Resonite のリッチテキスト指定を維持）。
//   - 最後に "..." で囲む。
func quote(s, newlineRepl string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			// strip（注入・クォート破壊防止）
		case r == '\n':
			b.WriteString(newlineRepl)
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

// QuoteArg は一般の文字列引数（user/role/url/template）を無害化＋引用する。
// 実改行はリテラル `\n`（2文字）にエスケープする（これらのフィールドは複数行表示を
// 想定しないため。万一改行が来ても安全に1コマンドへ収める安全網）。
func QuoteArg(s string) string { return quote(s, `\n`) }

// QuoteRichText はリッチテキスト表示フィールド（name/description/message）用。
// 実改行を `<br>` に変換する。Resonite のリッチテキストが `<br>` を改行として
// レンダリングすることを実機確認済み（2026-05-30）。ASCII なので Shift_JIS
// コンソールも通る。生改行は送らないため注入にはならない。
func QuoteRichText(s string) string { return quote(s, "<br>") }

// SanitizeToken は引用符を付けない単一トークン引数を検証し、問題なければそのまま返す。
// `^[A-Za-z0-9_-]+$` 以外は ErrInvalidToken を返す（呼び出し側で 400 にマップ）。
func SanitizeToken(s string) (string, error) {
	if !tokenRe.MatchString(s) {
		return "", ErrInvalidToken
	}
	return s, nil
}

// --- コマンドビルダー（複数経路で共有する組み立て関数・R14） ---
// セッションの spawn/impulse 書き込み API と orchestrator の告知③（§3.16(2)）が
// 同一のコマンド文字列を組むための単一の真実。引用は command.go の方針（strip 方式）に従う。

// SpawnCmd は `spawn <url> <active> <persistent>`（3引数・help 確定 2026-05-28）を組み立てる。
// url は QuoteArg で無害化＋引用（record URL は空白/引用符を含まないが注入安全網として一貫適用）。
// active=true でアイテムを有効状態で生成、persistent=true でワールド保存に含める。
func SpawnCmd(url string, active, persistent bool) string {
	return fmt.Sprintf("spawn %s %t %t", QuoteArg(url), active, persistent)
}

// DynamicImpulseStringCmd は `dynamicimpulsestring <tag> <value>` を組み立てる。
// tag は "MRHC.play" のように "." を含み得るため SanitizeToken は使えず QuoteArg で引用する。
// value は表示テキストになり得るため QuoteRichText（改行→<br>）で整形する。
// コマンド名は小文字（Resonite はコマンド大小無視・告知③が実機で発火実証済の形を踏襲）。
func DynamicImpulseStringCmd(tag, value string) string {
	return fmt.Sprintf("dynamicimpulsestring %s %s", QuoteArg(tag), QuoteRichText(value))
}
