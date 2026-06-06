// Package i18n は CLI・起動メッセージ・Web コンソールへの sys 案内で使う
// 最小限の文言カタログ（ja/en）を提供する。
// Web UI の表示言語は react-i18next が別管理（API エラーは code をフロントが
// 翻訳する既存方式のため本パッケージの対象外）。
// カタログの完全性（全キーが両言語を持ち fmt 動詞列が一致すること）は単体テストで担保する。
// 設計: docs/design/cli-onboarding.md
package i18n

import "fmt"

// Lang は表示言語。config の language フィールド（"ja"/"en"）に対応する。
type Lang string

const (
	Ja Lang = "ja"
	En Lang = "en"
)

// LangOf は "en" なら En、それ以外（"ja"・空・未知）は Ja を返す。
// platform.DetectLang の戻り値（"ja"/"en"）を Lang へ写すのに使う
// （config.LangOrDefault と同じ「en 以外は ja」規約）。
func LangOf(s string) Lang {
	if s == string(En) {
		return En
	}
	return Ja
}

// T は key の文言を lang で返す。args があれば fmt.Sprintf で埋め込む。
// 未知のキーは key 自体を返す（実行時に panic させない。完全性はテストで担保）。
func T(lang Lang, key string, args ...any) string {
	m, ok := catalog[key]
	if !ok {
		return key
	}
	s, ok := m[lang]
	if !ok {
		s = m[Ja] // 片言語の欠落はテストで弾く。万一の実行時は ja に倒す
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}
