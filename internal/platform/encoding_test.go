package platform

import (
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func TestConsoleEncodingOverrides(t *testing.T) {
	if ConsoleEncoding("utf-8") != nil {
		t.Error(`ConsoleEncoding("utf-8") は nil(パススルー) であるべき`)
	}
	if ConsoleEncoding("") != osConsoleEncoding() {
		t.Error(`ConsoleEncoding("") は OS既定 と一致すべき`)
	}
	for _, name := range []string{"shift_jis", "shift-jis", "sjis", "cp932", "932"} {
		if ConsoleEncoding(name) != japanese.ShiftJIS {
			t.Errorf("ConsoleEncoding(%q) は ShiftJIS であるべき", name)
		}
	}
}

// Windows実機で確定した Shift_JIS 経路の往復をユニット検証する（実機不要）。
func TestShiftJISRoundTrip(t *testing.T) {
	enc := ConsoleEncoding("shift_jis")
	if enc == nil {
		t.Fatal("ShiftJIS encoding が nil")
	}
	const s = "日本語テスト 作業セッション グリッドスペース"

	sjis, _, err := transform.String(enc.NewEncoder(), s)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if sjis == s {
		t.Error("Shift_JISバイト列がUTF-8と同一＝変換されていない")
	}

	back, _, err := transform.String(enc.NewDecoder(), sjis)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back != s {
		t.Errorf("往復不一致: %q != %q", back, s)
	}
}
