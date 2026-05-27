package platform

import (
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// ConsoleEncoding はヘッドレスのコンソール入出力に使う文字コードを返す。
// 返り値 nil は「UTF-8パススルー（変換不要）」を意味する。
//
//   - override（"utf-8" / "shift_jis" / "cp932" など）が指定されればそれを優先
//   - 空ならOS既定: Windows=システムコードページを検出（実機確定: JP=Shift_JIS）、
//     その他のOS=UTF-8（実機確定: Linux=UTF-8）
//
// 実機検証: Windows(JP)=Shift_JIS(CP932) / Linux=UTF-8（docs/resonite-domain-facts.md §7）。
func ConsoleEncoding(override string) encoding.Encoding {
	switch strings.ToLower(strings.TrimSpace(override)) {
	case "":
		return osConsoleEncoding()
	case "utf-8", "utf8":
		return nil
	case "shift_jis", "shift-jis", "sjis", "cp932", "932":
		return japanese.ShiftJIS
	case "euc-jp", "eucjp":
		return japanese.EUCJP
	case "gbk", "cp936", "936":
		return simplifiedchinese.GBK
	case "euc-kr", "euckr", "cp949", "949":
		return korean.EUCKR
	case "big5", "cp950", "950":
		return traditionalchinese.Big5
	case "windows-1252", "cp1252", "1252":
		return charmap.Windows1252
	default:
		// 未知の指定はOS既定にフォールバック
		return osConsoleEncoding()
	}
}
