package platform

import "strings"

// detectLangFromEnv は POSIX ロケール環境変数から表示言語（"ja"/"en"）を推定する。
// LC_ALL → LC_MESSAGES → LANG の優先順（POSIX の解決順）で最初に設定されている値を見て、
// "ja" 接頭辞（ja_JP.UTF-8 等）なら日本語、それ以外（C/POSIX/他言語）は英語に倒す。
// DetectLang（lang_unix.go）の実体。テストから getenv を注入できるよう分離している。
func detectLangFromEnv(getenv func(string) string) string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := getenv(k)
		if v == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(v), "ja") {
			return "ja"
		}
		return "en" // 最初に設定されている変数が勝つ（後続へフォールスルーしない）
	}
	return "en"
}
