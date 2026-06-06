package setup

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/steam"
)

// newTestWizard は非tty・依存なし・OS検出=ja の決定的なウィザードを作る。
// SteamUpdate は既定で panic（S5 を通らないテストで誤って DL 経路に入った事故を検出）。
// S5 を検証するテストは個別に差し替える（wizard_steam_test.go）。
func newTestWizard(input string) (*Wizard, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &Wizard{
		In:         strings.NewReader(input),
		Out:        out,
		TTY:        false,
		DetectLang: func() string { return "ja" },
		PortInUse:  func(int) bool { return false }, // 既定=空き（実ポートを探りに行かない）
		CheckDeps:  func() []platform.DepIssue { return nil },
		SteamUpdate: func(context.Context, string, steam.UpdateParams, func(steam.Event)) error {
			panic("SteamUpdate が呼ばれるべきでないテストで呼ばれた")
		},
	}, out
}

func tmpCfgPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), config.FileName)
}

// 既定値主体のフロー（S5 のみ n でスキップ）= 言語ja・port既定・起動Y。
func TestWizard_HappyPathDefaults(t *testing.T) {
	w, out := newTestWizard("\npw\npw\n\nn\n\n") // S0空=ja / pw / pw / port空=8080 / S5=n / S6空=Y
	cfgPath := tmpCfgPath(t)

	cfg, startNow, err := w.Run(cfgPath)
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if !startNow {
		t.Error("S6 空Enter は startNow=true のはず")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Language != "ja" {
		t.Errorf("Language = %q, want ja", cfg.Language)
	}
	// 保存内容も確認（ディスク往復・bcrypt 照合）
	loaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(loaded.AdminPasswordHash), []byte("pw")); err != nil {
		t.Errorf("保存されたハッシュが入力PWと一致しない: %v", err)
	}
	if loaded.SessionSecret == "" {
		t.Error("SessionSecret が生成されていない")
	}
	if !strings.Contains(out.String(), "MRHC 初回セットアップ") {
		t.Errorf("S1 導入が表示されていない: %s", out.String())
	}
}

// S6 で n → startNow=false・「あとで起動」案内。
func TestWizard_StartLater(t *testing.T) {
	w, out := newTestWizard("\npw\npw\n\nn\nn\n") // S5=n / S6=n
	_, startNow, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if startNow {
		t.Error("n は startNow=false のはず")
	}
	if !strings.Contains(out.String(), "あとで起動するには") {
		t.Errorf("S8 案内が出ていない: %s", out.String())
	}
}

// S0 不正値 "3" → 再入力 → "1"=英語。config に en が保存され英語文言が出る。
func TestWizard_LangInvalidThenEnglish(t *testing.T) {
	w, out := newTestWizard("3\n1\npw\npw\n\nn\n\n")
	cfg, _, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if cfg.Language != "en" {
		t.Errorf("Language = %q, want en", cfg.Language)
	}
	s := out.String()
	if !strings.Contains(s, "Please enter 1 or 2.") {
		t.Errorf("S0 再入力の案内が出ていない: %s", s)
	}
	if !strings.Contains(s, "First-Time Setup") {
		t.Errorf("英語の導入文言が出ていない: %s", s)
	}
}

// S3 不正値（非数値・範囲外）→ 再入力 → 有効値。
func TestWizard_PortInvalidThenValid(t *testing.T) {
	w, out := newTestWizard("\npw\npw\nabc\n70000\n8081\nn\n\n")
	cfg, _, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if cfg.Port != 8081 {
		t.Errorf("Port = %d, want 8081", cfg.Port)
	}
	if c := strings.Count(out.String(), "1〜65535"); c != 2 {
		t.Errorf("ポート再入力の案内が %d 回（want 2）: %s", c, out.String())
	}
}

// S3 使用中ポート → 警告 [y/N]。空 Enter=N でポート再入力に戻り、別ポートで進める。
func TestWizard_PortInUseRetry(t *testing.T) {
	w, out := newTestWizard("\npw\npw\n\n\n8090\nn\n\n") // port空=8080→使用中→空=N→8090 / S5=n / S6=Y
	w.PortInUse = func(p int) bool { return p == 8080 }
	cfg, _, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if cfg.Port != 8090 {
		t.Errorf("Port = %d, want 8090", cfg.Port)
	}
	if !strings.Contains(out.String(), "ポート 8080 は現在使用中のようです") {
		t.Errorf("使用中の警告が出ていない: %s", out.String())
	}
}

// S3 使用中ポート → y でそのまま採用（あとで空く予定のケース）。
func TestWizard_PortInUseForceUse(t *testing.T) {
	w, _ := newTestWizard("\npw\npw\n\ny\nn\n\n") // port空=8080→使用中→y / S5=n / S6=Y
	w.PortInUse = func(int) bool { return true }
	cfg, _, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
}

// 使用中警告の [y/N] で EOF → 中断（無限ループしない）。
func TestWizard_PortInUseEOFAborts(t *testing.T) {
	w, out := newTestWizard("\npw\npw\n\n") // port空=8080→使用中→EOF
	w.PortInUse = func(int) bool { return true }
	_, _, err := w.Run(tmpCfgPath(t))
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("err = %v, want ErrAborted", err)
	}
	if !strings.Contains(out.String(), "セットアップを中断しました") {
		t.Errorf("中断メッセージが出ていない: %s", out.String())
	}
}

// [Y/n] 不正値 → 再入力（黙って n に倒さない）。S5 で不正→n、S6 で空=Y。
func TestWizard_YNInvalidThenYes(t *testing.T) {
	w, out := newTestWizard("\npw\npw\n\nはい\nn\n\n")
	_, startNow, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !startNow {
		t.Error("S6 空Enter は startNow=true のはず")
	}
	if !strings.Contains(out.String(), "y か n で答えてください。") {
		t.Errorf("[Y/n] 再入力の案内が出ていない: %s", out.String())
	}
}

// PW 不一致 → 再入力 / 空PW → 再入力。
func TestWizard_PasswordRetry(t *testing.T) {
	w, out := newTestWizard("\n\npw1\npw2\npw\npw\n\nn\n\n") // 空PW→不一致→一致
	_, _, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "空のパスワード") || !strings.Contains(s, "一致しません") {
		t.Errorf("PW 再入力の案内が出ていない: %s", s)
	}
}

// 入力が尽きた（EOF）→ ErrAborted・中断メッセージ・無限ループしない。
func TestWizard_EOFAborts(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"S0 で即EOF", ""},
		{"S2 PW入力中にEOF", "\npw\n"},
		{"S3 ポート入力中にEOF", "\npw\npw\n"},
		{"S5 Resonite確認でEOF", "\npw\npw\n\n"},
		{"S6 起動確認でEOF", "\npw\npw\n\nn\n"},
		{"S0 不正値の後にEOF", "3\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, out := newTestWizard(c.input)
			_, _, err := w.Run(tmpCfgPath(t))
			if !errors.Is(err, ErrAborted) {
				t.Fatalf("err = %v, want ErrAborted", err)
			}
			if !strings.Contains(out.String(), "セットアップを中断しました") {
				t.Errorf("中断メッセージが出ていない: %s", out.String())
			}
		})
	}
}

// abortOr: 読取エラー（errInput）だけが中断扱いになり、実エラー（SaveTo 失敗等）は
// そのまま伝播する（M1: EOF と誤報告しない）。
func TestWizard_AbortOrDistinguishesErrors(t *testing.T) {
	w, out := newTestWizard("")
	_, readErr := readLine(bufio.NewReader(strings.NewReader(""))) // readLine 経由の実エラー形
	if err := w.abortOr("ja", readErr); !errors.Is(err, ErrAborted) {
		t.Errorf("読取エラーは ErrAborted になるべき: %v", err)
	}
	if !strings.Contains(out.String(), "セットアップを中断しました") {
		t.Errorf("中断メッセージが出ていない: %s", out.String())
	}
	saveErr := errors.New("disk full")
	if err := w.abortOr("ja", saveErr); !errors.Is(err, saveErr) || errors.Is(err, ErrAborted) {
		t.Errorf("実エラーはそのまま伝播すべき: %v", err)
	}
}

// S3 保存後の中断（S5 EOF）でも config は残る（途中までの入力が無駄にならない）。
func TestWizard_ConfigSurvivesLateAbort(t *testing.T) {
	w, _ := newTestWizard("\npw\npw\n\n") // S5 でEOF
	cfgPath := tmpCfgPath(t)
	_, _, err := w.Run(cfgPath)
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("err = %v, want ErrAborted", err)
	}
	if !config.FileExists(cfgPath) {
		t.Error("S3 直後に保存された config が存在しない")
	}
}
