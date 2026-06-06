package platform

import "testing"

func TestDetectLangFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"LC_ALL が ja", map[string]string{"LC_ALL": "ja_JP.UTF-8"}, "ja"},
		{"LC_ALL が en", map[string]string{"LC_ALL": "en_US.UTF-8"}, "en"},
		{"LC_ALL が他言語(de)", map[string]string{"LC_ALL": "de_DE.UTF-8"}, "en"},
		{"C ロケール", map[string]string{"LC_ALL": "C"}, "en"},
		{"LC_ALL 空で LANG が ja", map[string]string{"LANG": "ja_JP.UTF-8"}, "ja"},
		{"LC_MESSAGES が LANG より優先", map[string]string{"LC_MESSAGES": "en_US.UTF-8", "LANG": "ja_JP.UTF-8"}, "en"},
		{"LC_ALL が LC_MESSAGES より優先", map[string]string{"LC_ALL": "ja_JP.UTF-8", "LC_MESSAGES": "en_US.UTF-8"}, "ja"},
		{"すべて未設定", map[string]string{}, "en"},
		{"大文字 JA も拾う", map[string]string{"LANG": "JA_JP"}, "ja"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := func(k string) string { return c.env[k] }
			if got := detectLangFromEnv(getenv); got != c.want {
				t.Errorf("detectLangFromEnv = %q, want %q", got, c.want)
			}
		})
	}
}

// TestDetectLang は実 OS での呼び出しが "ja"/"en" のどちらかを返すことだけ確認する
// （値自体は環境依存のため固定しない）。
func TestDetectLang(t *testing.T) {
	got := DetectLang()
	if got != "ja" && got != "en" {
		t.Errorf("DetectLang = %q, want ja or en", got)
	}
}
