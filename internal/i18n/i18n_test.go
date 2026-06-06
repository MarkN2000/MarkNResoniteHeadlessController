package i18n

import (
	"reflect"
	"testing"
)

// verbSeq は文言中の fmt 動詞を出現順に抽出する（例 "x %d y %s" → ["d","s"]）。
// "%%"（リテラル%）は除外。フラグ・幅・精度（%-6.2f 等）は読み飛ばして動詞文字だけを採る。
// 「数の一致」だけだと "%s %d" と "%d %s" の取り違え（実行時の %!d(string=...) 化け）を
// 見逃すため、順序つきの列で比較する（テスト専用ヘルパ）。
func verbSeq(s string) []string {
	var seq []string
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '%' {
			continue
		}
		i++
		if i >= len(rs) {
			break
		}
		if rs[i] == '%' { // "%%" はリテラル
			continue
		}
		// フラグ・幅・精度を読み飛ばす
		for i < len(rs) && (rs[i] == '-' || rs[i] == '+' || rs[i] == ' ' || rs[i] == '#' ||
			rs[i] == '.' || (rs[i] >= '0' && rs[i] <= '9')) {
			i++
		}
		if i < len(rs) {
			seq = append(seq, string(rs[i]))
		}
	}
	return seq
}

// TestCatalogComplete は実カタログの完全性を機械検査する:
// 全キーが ja/en の両方を持ち、fmt 動詞の列（順序つき）が両言語で一致すること。
func TestCatalogComplete(t *testing.T) {
	for key, m := range catalog {
		ja, okJa := m[Ja]
		en, okEn := m[En]
		if !okJa || !okEn {
			t.Errorf("キー %q: ja/en の両方が必要 (ja=%v en=%v)", key, okJa, okEn)
			continue
		}
		if !reflect.DeepEqual(verbSeq(ja), verbSeq(en)) {
			t.Errorf("キー %q: fmt 動詞列が不一致 ja=%v en=%v", key, verbSeq(ja), verbSeq(en))
		}
	}
}

func TestT(t *testing.T) {
	// テスト用キーを一時注入（実カタログを汚さないよう後で戻す）
	const k = "test.greet"
	catalog[k] = map[Lang]string{Ja: "こんにちは %s さん（%d回目）", En: "Hello %s (visit %d)"}
	defer delete(catalog, k)

	if got := T(Ja, k, "太郎", 2); got != "こんにちは 太郎 さん（2回目）" {
		t.Errorf("ja 整形が不正: %q", got)
	}
	if got := T(En, k, "Taro", 2); got != "Hello Taro (visit 2)" {
		t.Errorf("en 整形が不正: %q", got)
	}
	// 未知キーはキー自体を返す（panic しない）
	if got := T(Ja, "no.such.key"); got != "no.such.key" {
		t.Errorf("未知キーはキーを返すべき: %q", got)
	}
	// 片言語欠落は ja に倒す（完全性テストが弾くが実行時の保険）
	const k2 = "test.ja-only"
	catalog[k2] = map[Lang]string{Ja: "日本語のみ"}
	defer delete(catalog, k2)
	if got := T(En, k2); got != "日本語のみ" {
		t.Errorf("en 欠落時は ja に倒すべき: %q", got)
	}
}

func TestVerbSeq(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"動詞なし", nil},
		{"ポート %d を開く", []string{"d"}},
		{"%s と %d", []string{"s", "d"}},
		{"%d と %s", []string{"d", "s"}}, // 順序が区別されること
		{"100%% 完了 %s", []string{"s"}},  // %% は除外
		{"%-6.2f 進捗", []string{"f"}},    // フラグ・幅・精度を跨いで動詞を取る
		{"%v %T %q", []string{"v", "T", "q"}},
		{"末尾%", nil}, // 不完全な % は無視（panic しない）
	}
	for _, c := range cases {
		if got := verbSeq(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("verbSeq(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
