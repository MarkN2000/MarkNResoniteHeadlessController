package server

// R-C 経路③: handleStart 起点の依存不足ガイド（publishDepGuide）の回帰テスト。
// handleStart は goroutine で包むため、ここでは同期に呼んで sys ログが
// driver の log hub（＝UI コンソール）へ乗ることを検証する。

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

func TestPublishDepGuide_PublishesSysLog(t *testing.T) {
	s, dataDir := newPathServer(t, nil)
	var gotInstall string
	s.checkDeps = func(_, _, installDir string) []platform.DepIssue {
		gotInstall = installDir
		return []platform.DepIssue{
			{Kind: "freetype2", Title: "freetype2（Resonite のネイティブ依存）",
				Commands: []string{"sudo pacman -S freetype2"}},
			{Kind: "dotnet10", Title: ".NET 10 ランタイム（ARM Linux で必要）",
				Fallback: "手動で導入してください。"}, // Commands 空＝Fallback 経路
		}
	}
	ch, _ := s.driver.SubscribeLog(8)
	defer s.driver.UnsubscribeLog(ch)

	s.publishDepGuide()

	// installDir は既定（{dataDir}/resonite）が導出されて渡る。
	if want := filepath.Join(dataDir, "resonite"); gotInstall != want {
		t.Errorf("installDir=%q want %q", gotInstall, want)
	}
	// 2 件の issue がそれぞれ 1 行の sys ログになる（コマンドあり / Fallback）。
	wants := []string{"sudo pacman -S freetype2", "手動で導入してください。"}
	for i, want := range wants {
		var line headless.LogLine
		select {
		case line = <-ch:
		default:
			t.Fatalf("sys ログ %d 行目が発行されていない", i+1)
		}
		if line.Kind != "sys" {
			t.Errorf("kind=%q want sys", line.Kind)
		}
		if !strings.Contains(line.Text, "依存不足") || !strings.Contains(line.Text, want) {
			t.Errorf("text=%q に %q が含まれるべき", line.Text, want)
		}
	}
}

// 不足ゼロなら何も流さない（Windows は checkDeps が常に空＝この経路）。
func TestPublishDepGuide_NoIssuesNoLog(t *testing.T) {
	s, _ := newPathServer(t, nil)
	s.checkDeps = func(_, _, _ string) []platform.DepIssue { return nil }
	ch, _ := s.driver.SubscribeLog(8)
	defer s.driver.UnsubscribeLog(ch)

	s.publishDepGuide()

	select {
	case line := <-ch:
		t.Errorf("不足ゼロなのにログが出た: %+v", line)
	default:
	}
}
