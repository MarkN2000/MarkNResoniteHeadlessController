package setup

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/steam"
)

// fakeUpdate は SteamUpdate の偽実装。呼び出しごとに results を順に返し、
// 呼び出し内容（params/toolsDir）を記録する。
type fakeUpdate struct {
	results  []error
	calls    int
	params   []steam.UpdateParams
	toolsDir string
	events   []steam.Event // 各呼び出しで onEvent へ流すイベント
}

func (f *fakeUpdate) fn(_ context.Context, toolsDir string, p steam.UpdateParams, onEvent func(steam.Event)) error {
	f.toolsDir = toolsDir
	f.params = append(f.params, p)
	for _, e := range f.events {
		onEvent(e)
	}
	res := f.results[f.calls]
	f.calls++
	return res
}

// S5 happy path: Y → 資格入力 → DL 成功（進捗イベントが 10% 刻みで描画される）。
func TestWizard_SteamHappy(t *testing.T) {
	fake := &fakeUpdate{
		results: []error{nil},
		events: []steam.Event{
			{Kind: "log", Text: "acquiring"}, // acquire 中の log では「完了」を出さない
			{Kind: "milestone", Text: "Using app branch"},
			{Kind: "progress", Percent: 12.3},
			{Kind: "progress", Percent: 13.0}, // 同じ 10% 帯は再表示しない
			{Kind: "progress", Percent: 47.0},
			{Kind: "progress", Percent: 100},
		},
	}
	// S0空 / pw / pw / port空 / S5空=Y / user / steampw / code / install空=既定 / S6空=Y
	w, out := newTestWizard("\npw\npw\n\n\nuser1\nsteampw\ncode1\n\n\n")
	w.SteamUpdate = fake.fn
	cfgPath := tmpCfgPath(t)
	dataDir := filepath.Dir(cfgPath)

	_, startNow, err := w.Run(cfgPath)
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if !startNow {
		t.Error("startNow=true のはず")
	}
	if fake.calls != 1 {
		t.Fatalf("SteamUpdate 呼び出し回数 = %d, want 1", fake.calls)
	}
	p := fake.params[0]
	if p.Username != "user1" || p.Password != "steampw" || p.BranchCode != "code1" {
		t.Errorf("params 資格が入力と不一致: %+v", p)
	}
	if want := filepath.Join(dataDir, "resonite"); p.InstallDir != want {
		t.Errorf("InstallDir = %q, want %q（既定導出）", p.InstallDir, want)
	}
	if want := filepath.Join("Headless", platform.HeadlessBinaryName()); p.VerifyRelPath != want {
		t.Errorf("VerifyRelPath = %q, want %q", p.VerifyRelPath, want)
	}
	if want := filepath.Join(dataDir, "tools"); fake.toolsDir != want {
		t.Errorf("toolsDir = %q, want %q", fake.toolsDir, want)
	}
	// 資格が config に保存されている（Web UI から再利用できる・M4）
	loaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if loaded.Steam == nil || loaded.Steam.Username != "user1" || loaded.Steam.Password != "steampw" ||
		loaded.Steam.BranchCode != "code1" {
		t.Errorf("config.Steam が保存されていない: %+v", loaded.Steam)
	}
	if loaded.Steam.InstallDir != "" {
		t.Errorf("install 先空Enter は InstallDir 未設定（既定導出）のはず: %q", loaded.Steam.InstallDir)
	}
	// 進捗描画: 準備完了は最初の milestone で・% は 10% 刻み・完了メッセージ
	s := out.String()
	for _, want := range []string{" 完了", "10%", "40%", "100%", "✓ ダウンロード完了"} {
		if !strings.Contains(s, want) {
			t.Errorf("出力に %q が無い:\n%s", want, s)
		}
	}
	if strings.Count(s, "ダウンロード中... 10%") != 1 {
		t.Errorf("同じ 10%% 帯が重複表示されている:\n%s", s)
	}
}

// S5 で n → Steam 設定なし・DL も呼ばれない（newTestWizard の panic 既定で担保）。
func TestWizard_SteamSkip(t *testing.T) {
	w, _ := newTestWizard("\npw\npw\n\nn\n\n")
	cfgPath := tmpCfgPath(t)
	cfg, _, err := w.Run(cfgPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if cfg.Steam != nil {
		t.Errorf("スキップ時は Steam=nil のはず: %+v", cfg.Steam)
	}
}

// S5a ユーザー名で空Enter → セクション中止・続行（S6 へ）。
func TestWizard_SteamCancelEmptyUser(t *testing.T) {
	w, out := newTestWizard("\npw\npw\n\n\n\n\n") // S5=Y → user空=中止 → S6空=Y
	cfg, startNow, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if !startNow {
		t.Error("中止後も S6 まで続行して startNow=true のはず")
	}
	if cfg.Steam != nil {
		t.Errorf("中止時は Steam=nil のはず: %+v", cfg.Steam)
	}
	if !strings.Contains(out.String(), "入力を中止しました") {
		t.Errorf("中止の案内が出ていない:\n%s", out.String())
	}
}

// S5a パスワード不正（非ASCII）→ 再入力。
func TestWizard_SteamPwInvalidThenValid(t *testing.T) {
	fake := &fakeUpdate{results: []error{nil}}
	w, out := newTestWizard("\npw\npw\n\n\nuser\nぱすわーど\ngoodpw\ncode\n\n\n")
	w.SteamUpdate = fake.fn
	_, _, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if !strings.Contains(out.String(), "ASCII") {
		t.Errorf("PW 再入力の案内が出ていない:\n%s", out.String())
	}
	if fake.params[0].Password != "goodpw" {
		t.Errorf("再入力後の PW が使われていない: %q", fake.params[0].Password)
	}
}

// 認証失敗 → 再入力 Y → 2回目の資格で成功。
func TestWizard_SteamAuthRetry(t *testing.T) {
	fake := &fakeUpdate{results: []error{steam.ErrAuthFailed, nil}}
	w, out := newTestWizard("\npw\npw\n\n\nuser1\npw1\ncode1\n\ny\nuser2\npw2\ncode2\n\n\n")
	w.SteamUpdate = fake.fn
	cfgPath := tmpCfgPath(t)
	_, _, err := w.Run(cfgPath)
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if fake.calls != 2 {
		t.Fatalf("SteamUpdate 呼び出し回数 = %d, want 2", fake.calls)
	}
	if fake.params[1].Username != "user2" {
		t.Errorf("2回目は再入力した資格を使うはず: %+v", fake.params[1])
	}
	if !strings.Contains(out.String(), "認証に失敗しました") {
		t.Errorf("認証失敗の案内が出ていない:\n%s", out.String())
	}
	// 最終的に成功した資格が保存されている
	loaded, _ := config.LoadFrom(cfgPath)
	if loaded.Steam.Username != "user2" {
		t.Errorf("保存された資格が古い: %q", loaded.Steam.Username)
	}
}

// 認証失敗 → 再入力 n → 案内を出して続行。
func TestWizard_SteamAuthRetryDeclined(t *testing.T) {
	fake := &fakeUpdate{results: []error{steam.ErrAuthFailed}}
	w, out := newTestWizard("\npw\npw\n\n\nuser\nspw\ncode\n\nn\n\n")
	w.SteamUpdate = fake.fn
	_, startNow, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if !startNow {
		t.Error("拒否後も S6 まで続行するはず")
	}
	if !strings.Contains(out.String(), "あとから Web UI（設定 → Steam）で再試行できます") {
		t.Errorf("再試行の案内が出ていない:\n%s", out.String())
	}
}

// Steam Guard 有効（2FA 要求）→ 再入力に誘導せず専用案内で続行（M3）。
func TestWizard_SteamTwoFactor(t *testing.T) {
	fake := &fakeUpdate{results: []error{steam.ErrTwoFactorRequired}}
	w, out := newTestWizard("\npw\npw\n\n\nuser\nspw\ncode\n\n\n") // 再入力プロンプトは消費しない
	w.SteamUpdate = fake.fn
	_, startNow, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if !startNow {
		t.Error("2FA 検知後も S6 まで続行するはず")
	}
	if fake.calls != 1 {
		t.Errorf("再入力ループに入らないはず: calls=%d", fake.calls)
	}
	s := out.String()
	if !strings.Contains(s, "Steam Guard（二段階認証）が有効") {
		t.Errorf("2FA 専用案内が出ていない:\n%s", s)
	}
	if strings.Contains(s, "もう一度入力しますか") {
		t.Errorf("2FA で再入力に誘導してはいけない:\n%s", s)
	}
}

// その他の失敗（ネットワーク等）→ 失敗表示＋再試行案内で続行。
func TestWizard_SteamGenericFailure(t *testing.T) {
	fake := &fakeUpdate{results: []error{errors.New("network down")}}
	w, out := newTestWizard("\npw\npw\n\n\nuser\nspw\ncode\n\n\n")
	w.SteamUpdate = fake.fn
	_, startNow, err := w.Run(tmpCfgPath(t))
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if !startNow {
		t.Error("DL 失敗後も S6 まで続行するはず")
	}
	s := out.String()
	if !strings.Contains(s, "✗ ダウンロードに失敗しました: network down") {
		t.Errorf("失敗の案内が出ていない:\n%s", s)
	}
}

// インストール先を明示入力 → config と params の両方に反映。
func TestWizard_SteamInstallDirExplicit(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "myresonite")
	fake := &fakeUpdate{results: []error{nil}}
	w, _ := newTestWizard("\npw\npw\n\n\nuser\nspw\ncode\n" + custom + "\n\n")
	w.SteamUpdate = fake.fn
	cfgPath := tmpCfgPath(t)
	_, _, err := w.Run(cfgPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fake.params[0].InstallDir != custom {
		t.Errorf("params.InstallDir = %q, want %q", fake.params[0].InstallDir, custom)
	}
	loaded, _ := config.LoadFrom(cfgPath)
	if loaded.Steam.InstallDir != custom {
		t.Errorf("config.Steam.InstallDir = %q, want %q", loaded.Steam.InstallDir, custom)
	}
}

// 認証失敗→再入力の 2 周目で、1 周目に明示したインストール先が空 Enter で維持される（M2）。
func TestWizard_SteamAuthRetryKeepsCustomInstallDir(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "myresonite")
	fake := &fakeUpdate{results: []error{steam.ErrAuthFailed, nil}}
	// 1周目: installDir=custom → 認証失敗 → 再入力 y → 2周目: installDir 空Enter
	w, out := newTestWizard("\npw\npw\n\n\nuser1\npw1\ncode1\n" + custom + "\ny\nuser2\npw2\ncode2\n\n\n")
	w.SteamUpdate = fake.fn
	cfgPath := tmpCfgPath(t)
	_, _, err := w.Run(cfgPath)
	if err != nil {
		t.Fatalf("Run: %v\nout=%s", err, out.String())
	}
	if fake.params[1].InstallDir != custom {
		t.Errorf("2周目の InstallDir = %q, want %q（空Enter=表示した既定の維持）", fake.params[1].InstallDir, custom)
	}
	loaded, _ := config.LoadFrom(cfgPath)
	if loaded.Steam.InstallDir != custom {
		t.Errorf("config の InstallDir が巻き戻った: %q", loaded.Steam.InstallDir)
	}
	// 2周目のプロンプトにも前回値が既定として表示される
	if !strings.Contains(out.String(), "["+custom+"]") {
		t.Errorf("2周目の既定表示が前回値でない:\n%s", out.String())
	}
}

// S5a 入力中の EOF → ErrAborted。
func TestWizard_SteamEOFDuringCreds(t *testing.T) {
	w, out := newTestWizard("\npw\npw\n\n\nuser1\n") // Steam PW で EOF
	_, _, err := w.Run(tmpCfgPath(t))
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("err = %v, want ErrAborted", err)
	}
	if !strings.Contains(out.String(), "セットアップを中断しました") {
		t.Errorf("中断メッセージが出ていない:\n%s", out.String())
	}
}
