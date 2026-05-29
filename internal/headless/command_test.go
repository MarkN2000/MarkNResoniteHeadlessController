package headless

import (
	"strings"
	"testing"
)

func TestQuoteArg_WrapsInQuotes(t *testing.T) {
	got := QuoteArg("Alice")
	if got != `"Alice"` {
		t.Fatalf("expected %q, got %q", `"Alice"`, got)
	}
}

// 末尾バックスラッシュでクォートが破壊されない（= 注入されない）こと。
// strip しないと `"path\"` のように閉じクォートが無効化される。
func TestQuoteArg_TrailingBackslashStripped(t *testing.T) {
	got := QuoteArg(`path\`)
	if got != `"path"` {
		t.Fatalf("trailing backslash not stripped: %q", got)
	}
	// 開始 1 個・終了 1 個の計 2 個のクォートのみ（中に裸クォートが無い）
	if strings.Count(got, `"`) != 2 {
		t.Fatalf("unexpected quote count: %q", got)
	}
}

// 埋め込みダブルクォートは strip される（裸の `"` が残らない）。
func TestQuoteArg_EmbeddedQuoteStripped(t *testing.T) {
	got := QuoteArg(`O"Brien`)
	if got != `"OBrien"` {
		t.Fatalf("embedded quote not stripped: %q", got)
	}
}

// 最重要: 生の改行が出力に残らない（残ると 2 行目がコマンドとして注入される）。
func TestQuoteArg_NewlineDoesNotInject(t *testing.T) {
	got := QuoteArg("a\nshutdown")
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("raw newline/CR leaked into output: %q", got)
	}
	if got != `"a\nshutdown"` { // 実改行 → 2 文字エスケープ \n
		t.Fatalf("expected escaped newline, got %q", got)
	}
}

// CRLF は単一の \n エスケープに畳まれる。
func TestQuoteArg_CRLFCollapses(t *testing.T) {
	got := QuoteArg("a\r\nb")
	if got != `"a\nb"` {
		t.Fatalf("CRLF not collapsed to single \\n: %q", got)
	}
}

// その他の制御文字（タブ・NUL・DEL）は除去される。
func TestQuoteArg_ControlCharsStripped(t *testing.T) {
	got := QuoteArg("a\tb\x00c\x7fd")
	if got != `"abcd"` {
		t.Fatalf("control chars not stripped: %q", got)
	}
}

// マルチバイト文字（日本語）は保持される。
func TestQuoteArg_MultibytePreserved(t *testing.T) {
	got := QuoteArg("日本語セッション")
	if got != `"日本語セッション"` {
		t.Fatalf("multibyte mangled: %q", got)
	}
}

func TestQuoteArg_Empty(t *testing.T) {
	if got := QuoteArg(""); got != `""` {
		t.Fatalf("empty should be \"\", got %q", got)
	}
}

func TestSanitizeToken_Valid(t *testing.T) {
	ok := []string{"LAN", "Private", "ContactsPlus", "RegisteredUsers", "U-1NzqeqewOpM", "abc_def-123"}
	for _, s := range ok {
		got, err := SanitizeToken(s)
		if err != nil {
			t.Errorf("valid token %q rejected: %v", s, err)
		}
		if got != s {
			t.Errorf("token mutated: %q → %q", s, got)
		}
	}
}

func TestSanitizeToken_Invalid(t *testing.T) {
	bad := []string{"", "has space", "semi;colon", `quote"`, "back\\slash", "new\nline", "dot.dot", "plus+plus"}
	for _, s := range bad {
		if _, err := SanitizeToken(s); err == nil {
			t.Errorf("invalid token %q accepted", s)
		}
	}
}

// セキュリティ重要: 引用なしトークン経路（accesslevel/unban）は注入を防ぐため、
// 末尾・先頭の改行/CR/タブも必ず拒否されること（Go の `$` は \z 相当だが明示確認）。
func TestSanitizeToken_RejectsNewlinesEverywhere(t *testing.T) {
	bad := []string{"trail\n", "\nlead", "mid\nle", "cr\r", "\rcr", "tab\there", "LAN\nshutdown"}
	for _, s := range bad {
		if _, err := SanitizeToken(s); err == nil {
			t.Errorf("token with newline/CR/tab accepted (injection risk): %q", s)
		}
	}
}
