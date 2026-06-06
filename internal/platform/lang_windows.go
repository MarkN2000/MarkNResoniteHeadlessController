//go:build windows

package platform

import "golang.org/x/sys/windows"

var procGetUserDefaultUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// DetectLang は OS の UI 言語から表示言語（"ja"/"en"）を推定する。
// GetUserDefaultUILanguage が返す LANGID の下位 10bit（プライマリ言語）が
// 日本語（LANG_JAPANESE=0x11）なら ja、それ以外は en に倒す。
// ウィザード S0 言語選択の既定値の提案にだけ使う（確定値は config の language）。
func DetectLang() string {
	langid, _, _ := procGetUserDefaultUILanguage.Call()
	if langid&0x3ff == 0x11 { // LANG_JAPANESE
		return "ja"
	}
	return "en"
}
