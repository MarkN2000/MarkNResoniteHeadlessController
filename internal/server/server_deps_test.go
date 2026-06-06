package server

// R-C 経路③: handleStart 起点の依存不足ガイド（publishDepGuide）の回帰テスト。
// handleStart は goroutine で包むため、ここでは同期に呼んで sys ログが
// driver の log hub（＝UI コンソール）へ乗ることを検証する。

import (
	"strings"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

func TestPublishDepGuide_PublishesSysLog(t *testing.T) {
	s, _ := newPathServer(t, nil)
	s.checkDeps = func(_, _ string) []platform.DepIssue {
		return []platform.DepIssue{
			{Kind: "freetype2", Commands: []string{"sudo pacman -S freetype2"}},
			{Kind: "freetype2"}, // Commands 空＝fallback（手動案内）経路
		}
	}
	ch, _ := s.driver.SubscribeLog(8)
	defer s.driver.UnsubscribeLog(ch)

	s.publishDepGuide()

	// 2 件の issue がそれぞれ 1 行の sys ログになる（コマンドあり / fallback=手動案内）。
	wants := []string{"sudo pacman -S freetype2", "libfreetype6"}
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
	s.checkDeps = func(_, _ string) []platform.DepIssue { return nil }
	ch, _ := s.driver.SubscribeLog(8)
	defer s.driver.UnsubscribeLog(ch)

	s.publishDepGuide()

	select {
	case line := <-ch:
		t.Errorf("不足ゼロなのにログが出た: %+v", line)
	default:
	}
}
