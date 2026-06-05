package steam

import (
	"regexp"
	"strconv"
	"strings"
)

// DepotDownloader の出力検出（純関数・テスト対象）。
// 文言は SteamRE/DepotDownloader ソース実読で確定（docs/resonite-domain-facts.md §4.3/§4.4）。
// 最終文言は ARM 実機（Phase8）で再確認する。

// promptKind は DD が stdin 入力を待つプロンプトの種別。
type promptKind int

const (
	promptNone      promptKind = iota
	promptPassword             // Steam パスワード入力待ち
	promptTwoFactor            // 2FA（authenticator/email）入力待ち＝v1 非対応
)

// detectPrompt は DD の「末尾断片」（改行されていないバッファ）からプロンプト種別を判定する。
// DD はプロンプトを改行なし Write で出すため、確定行ではなく tail を見る（§4.3）。
func detectPrompt(tail string) promptKind {
	switch {
	case strings.Contains(tail, "Enter account password for "):
		return promptPassword
	case strings.Contains(tail, "2 factor auth code"),
		strings.Contains(tail, "authentication code sent to your email"):
		return promptTwoFactor
	default:
		return promptNone
	}
}

// progressRe は ` 12.34% <path>` 形式の進捗行を捉える。DD は進捗を WriteLine（行単位）で出し、
// 書式は "{0,6:#00.00}% {1}"（先頭に空白詰め＋% のあとに対象パス）。
var progressRe = regexp.MustCompile(`^\s*(\d{1,3}(?:\.\d+)?)%\s*(.*)$`)

// parseProgress は進捗行から percent(0..100) と対象パスを取り出す。進捗行でなければ ok=false。
func parseProgress(line string) (percent float64, file string, ok bool) {
	m := progressRe.FindStringSubmatch(line)
	if m == nil {
		return 0, "", false
	}
	p, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, "", false
	}
	return p, strings.TrimSpace(m[2]), true
}

// ddMilestones は DD の主要マイルストーン行（前方一致で判定）。確定文言は §4.4。
var ddMilestones = []string{
	"Using app branch",
	"Downloading depot",
	"Pre-allocating",
	"Validating",
	"Total downloaded",
}

// detectMilestone は行が主要マイルストーンならその名称を返す。
func detectMilestone(line string) (string, bool) {
	t := strings.TrimSpace(line)
	for _, m := range ddMilestones {
		if strings.HasPrefix(t, m) {
			return m, true
		}
	}
	return "", false
}
