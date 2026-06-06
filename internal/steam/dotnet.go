// dotnet.go は run() の「.NET ランタイム確保」ステップを担う。
// DD の DL 品に .NET ランタイムは含まれない（公式は初回クライアント起動時に
// LinuxBootstrap.sh / InstallScript.vdf で設置する）ため、DL/更新の完了後に
// 要求（runtimeconfig.json）を満たすランタイムが無ければ MRHC が設置する。
// 設計: docs/design/dotnet-runtime.md
package steam

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/dotnetruntime"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

// MilestoneDotnetInstalling は設置区間のマイルストーン名（wizard の区間ラベル判定用に再公開）。
const MilestoneDotnetInstalling = dotnetruntime.MilestoneInstalling

// ErrDotnetInstallFailed は .NET ランタイムの設置に失敗したことを表す
// （文言はプレフィックス部のみ・詳細は "%w: %w" で内側に付加する）。
var ErrDotnetInstallFailed = errors.New(".NET ランタイムの設置に失敗")

// runtimeEnsurer は設置ステップの実装。判定・取得を seam として持ち、テストで差し替える
// （system 判定は実マシンの dotnet に依存し、acquire はネットへ出るため）。
type runtimeEnsurer struct {
	goos, goarch string
	read         func(headlessDir string) (platform.RuntimeRequirement, bool)
	local        func(installDir string, req platform.RuntimeRequirement, goarch string) bool
	system       func(goos, goarch string, req platform.RuntimeRequirement) bool
	acquire      func(ctx context.Context, installDir, channel string, onEvent func(dotnetruntime.Event)) (string, error)
}

// newRuntimeEnsurer は実実装（platform 判定＋公式フィード取得）の ensurer を返す。
func newRuntimeEnsurer() *runtimeEnsurer {
	acq := dotnetruntime.NewAcquirer()
	return &runtimeEnsurer{
		goos:    runtime.GOOS,
		goarch:  runtime.GOARCH,
		read:    platform.ReadRuntimeRequirement,
		local:   platform.LocalRuntimeSatisfies,
		system:  platform.SystemRuntimeSatisfies,
		acquire: acq.Ensure,
	}
}

// ensure は要求を満たす .NET ランタイムが無ければ <installDir>/dotnet-runtime へ設置する。
//   - 要求が読めない（fakehl 等 runtimeconfig 無し）→ 楽観＝何もしない（R-B と同思想）
//   - ローカル設置済み / システム .NET が充足 → 何もしない（ログも出さない・挙動不変）
//   - 設置後になお要求を満たさない（要求 patch > フィード最新等）→ 明示エラー
//     （黙って成功にすると次回も再設置を繰り返すため）
func (r *runtimeEnsurer) ensure(ctx context.Context, installDir string, onEvent func(Event)) error {
	req, ok := r.read(filepath.Join(installDir, "Headless"))
	if !ok {
		return nil
	}
	if r.local(installDir, req, r.goarch) {
		return nil
	}
	if r.system(r.goos, r.goarch, req) {
		return nil
	}
	if _, err := r.acquire(ctx, installDir, req.Channel(), adaptDotnetEvent(onEvent)); err != nil {
		// 中断（ユーザー Cancel / shutdown）は run() の他段階と同様に「中止」へ正規化する。
		if ctx.Err() != nil {
			return ErrCancelled
		}
		return fmt.Errorf("%w: %w", ErrDotnetInstallFailed, err)
	}
	if !r.local(installDir, req, r.goarch) {
		return fmt.Errorf("%w: 設置後も要求 %s を満たしません（フィードの最新版が要求より古い可能性）", ErrDotnetInstallFailed, req.Raw)
	}
	return nil
}

// adaptDotnetEvent は dotnetruntime.Event（import 循環回避の最小同形）を steam.Event へ写す。
func adaptDotnetEvent(onEvent func(Event)) func(dotnetruntime.Event) {
	return func(e dotnetruntime.Event) {
		onEvent(Event{Kind: e.Kind, Text: e.Text, Percent: e.Percent, File: e.File, MsgKey: e.MsgKey, MsgArgs: e.MsgArgs})
	}
}
