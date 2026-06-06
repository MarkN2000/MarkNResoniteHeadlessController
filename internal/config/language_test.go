package config

import (
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/i18n"
)

func TestLangOrDefault(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want i18n.Lang
	}{
		{"en は En", "en", i18n.En},
		{"ja は Ja", "ja", i18n.Ja},
		{"空（language 無しの既存 config）は Ja", "", i18n.Ja},
		{"未知の値も Ja に倒す", "fr", i18n.Ja},
		{"大文字 EN は未知扱い＝Ja（ウィザードは小文字を保存する）", "EN", i18n.Ja},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &Config{Language: c.val}
			if got := cfg.LangOrDefault(); got != c.want {
				t.Errorf("LangOrDefault(%q) = %q, want %q", c.val, got, c.want)
			}
		})
	}
}
