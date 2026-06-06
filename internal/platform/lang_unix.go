//go:build !windows

package platform

import "os"

// DetectLang は OS のロケールから表示言語（"ja"/"en"）を推定する。
// ウィザード S0 言語選択の既定値の提案にだけ使う（確定値は config の language）。
func DetectLang() string { return detectLangFromEnv(os.Getenv) }
