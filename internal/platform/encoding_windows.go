//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

var procGetACP = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetACP")

// osConsoleEncoding はWindowsのアクティブコードページ(GetACP)から文字コードを推定する。
// nil は UTF-8 パススルーを意味する。設定の override で上書き可能。
func osConsoleEncoding() encoding.Encoding {
	acp, _, _ := procGetACP.Call()
	switch int(acp) {
	case 932:
		return japanese.ShiftJIS // 日本語（実機確定）
	case 936:
		return simplifiedchinese.GBK
	case 949:
		return korean.EUCKR
	case 950:
		return traditionalchinese.Big5
	case 1250:
		return charmap.Windows1250
	case 1251:
		return charmap.Windows1251
	case 1252:
		return charmap.Windows1252
	case 65001:
		return nil // UTF-8
	default:
		// 未知のコードページはUTF-8パススルー（ASCIIは安全。必要なら設定で上書き）
		return nil
	}
}
