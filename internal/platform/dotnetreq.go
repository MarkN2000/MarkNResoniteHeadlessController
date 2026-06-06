// dotnetreq.go はヘッドレスが要求する .NET ランタイムの「要求の読み取り」と「充足判定」を担う。
// 要求の正本は <install>/Headless/Resonite.runtimeconfig.json（DD の DL 品に含まれる）。
// channel（例 "10.0"）をここから導出することで、Resonite の .NET 移行に MRHC のコード変更なしで
// 追従する（バージョンのハードコードを持たない）。
// 取得・設置は internal/dotnetruntime が担い、本ファイルは判定のみ（launcher が毎起動使うため
// platform に置く＝dotnetruntime → platform の一方向 import に保つ）。
// 詳細仕様: docs/design/dotnet-runtime.md
package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// runtimeConfigName はヘッドレスのランタイム要求ファイル名（Headless フォルダ直下）。
const runtimeConfigName = "Resonite.runtimeconfig.json"

// netCoreAppFramework はベースランタイムのフレームワーク名。
// ASP.NET 用の Microsoft.AspNetCore.App は見ない（ヘッドレスに必要なのはベースのみ）。
const netCoreAppFramework = "Microsoft.NETCore.App"

// RuntimeRequirement はヘッドレスが要求する .NET ベースランタイムの版。
type RuntimeRequirement struct {
	Major, Minor, Patch int
	Raw                 string // runtimeconfig.json の原文（ログ・エラー文言用）
}

// Channel は dotnet ビルドフィードのチャンネル表記（major.minor。例 "10.0"）。
func (r RuntimeRequirement) Channel() string {
	return fmt.Sprintf("%d.%d", r.Major, r.Minor)
}

// satisfiedBy は版 (maj,min,pat) が要求を満たすか。.NET ホスト既定の roll-forward=Minor の規則:
// major 一致が必須・minor は要求以上（より大きい minor は patch を問わず可）。
func (r RuntimeRequirement) satisfiedBy(maj, min, pat int) bool {
	if maj != r.Major {
		return false
	}
	if min != r.Minor {
		return min > r.Minor
	}
	return pat >= r.Patch
}

// ReadRuntimeRequirement は <headlessDir>/Resonite.runtimeconfig.json から要求ランタイムを読む。
// framework（単数）/ frameworks（配列）の両形式に対応し Microsoft.NETCore.App を探す。
// 読めない・形式不明・版がパースできない場合は ok=false（呼び出し側は楽観＝何もしない。
// R-B の「判定不能は楽観」と同思想。fakehl 等 runtimeconfig の無い環境で挙動を変えないため）。
func ReadRuntimeRequirement(headlessDir string) (RuntimeRequirement, bool) {
	b, err := os.ReadFile(filepath.Join(headlessDir, runtimeConfigName))
	if err != nil {
		return RuntimeRequirement{}, false
	}
	var rc struct {
		RuntimeOptions struct {
			Framework  *runtimeFrameworkRef  `json:"framework"`
			Frameworks []runtimeFrameworkRef `json:"frameworks"`
		} `json:"runtimeOptions"`
	}
	if err := json.Unmarshal(b, &rc); err != nil {
		return RuntimeRequirement{}, false
	}
	refs := rc.RuntimeOptions.Frameworks
	if rc.RuntimeOptions.Framework != nil {
		refs = append([]runtimeFrameworkRef{*rc.RuntimeOptions.Framework}, refs...)
	}
	for _, f := range refs {
		if f.Name != netCoreAppFramework {
			continue
		}
		maj, min, pat, _, ok := parseVersionTriple(f.Version)
		if !ok {
			return RuntimeRequirement{}, false
		}
		return RuntimeRequirement{Major: maj, Minor: min, Patch: pat, Raw: f.Version}, true
	}
	return RuntimeRequirement{}, false
}

type runtimeFrameworkRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// parseVersionTriple は "10.0.8" / "10.0.0-preview.4.26230.115" を数値三つ組へ分解する。
// "-" / "+" 以降（prerelease・build metadata）は pre=true として切り落とす。
// "10.0"（2要素）は patch=0 として受ける。数値でない・要素数不正は ok=false。
func parseVersionTriple(s string) (maj, min, pat int, pre, ok bool) {
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		pre = true
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, 0, 0, pre, false
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, 0, pre, false
		}
		nums[i] = n
	}
	maj, min = nums[0], nums[1]
	if len(nums) == 3 {
		pat = nums[2]
	}
	return maj, min, pat, pre, true
}

// LocalRuntimeSatisfies は <installDir>/dotnet-runtime が要求を満たすかをローカル列挙のみで
// 判定する（ネットへ出ない・ms オーダー。起動時ガードの同期判定にも使うためオフライン安全が必須）。
//  1. dotnet-runtime/dotnet[.exe] が存在し実行 arch で使える（dotnetUsable と同規則）
//  2. shared/Microsoft.NETCore.App/<ver>/ に要求を満たす版がある
//
// prerelease 版ディレクトリ（"-" 含む）は不充足扱い: .NET ホストは release 要求から
// prerelease へ roll-forward しないため、在っても起動には使えない。
func LocalRuntimeSatisfies(installDir string, req RuntimeRequirement, goarch string) bool {
	rtDir := filepath.Join(installDir, "dotnet-runtime")
	host := ""
	for _, name := range []string{"dotnet", "dotnet.exe"} {
		c := filepath.Join(rtDir, name)
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			host = c
			break
		}
	}
	if host == "" || !dotnetUsable(host, goarch) {
		return false
	}
	entries, err := os.ReadDir(filepath.Join(rtDir, "shared", netCoreAppFramework))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if maj, min, pat, pre, ok := parseVersionTriple(e.Name()); ok && !pre && req.satisfiedBy(maj, min, pat) {
			return true
		}
	}
	return false
}

// SystemRuntimeSatisfies はシステムの .NET が要求を満たすと確認できたときだけ true を返す
// （見つからない・確認手段が機能しない、はどちらも false＝設置を試みる側に倒す。
// 設置失敗時は起動を best-effort で続行するため、誤 false でも可用性は現行挙動が下限）。
// 候補は Linux = ~/.dotnet/dotnet → PATH（launcher の systemDotnet と同順）/
// Windows = PATH → %ProgramFiles%\dotnet\dotnet.exe。
// true のときの含意: 設置をスキップし、launcher は env/DOTNET_ROOT を触らない＝挙動不変。
func SystemRuntimeSatisfies(goos, goarch string, req RuntimeRequirement) bool {
	programFiles := ""
	if goos == "windows" {
		programFiles = os.Getenv("ProgramFiles")
	}
	return systemRuntimeSatisfies(realDepProbe(), goos, goarch, req, programFiles)
}

// systemRuntimeSatisfies は probe 注入版（テストで決定的に検証する）。
func systemRuntimeSatisfies(p depProbe, goos, goarch string, req RuntimeRequirement, programFiles string) bool {
	candidate := ""
	if goos == "windows" {
		if found, err := p.lookPath("dotnet"); err == nil {
			candidate = found
		} else if programFiles != "" {
			c := filepath.Join(programFiles, "dotnet", "dotnet.exe")
			if _, err := p.stat(c); err == nil {
				candidate = c
			}
		}
	} else {
		c := path.Join(p.home, ".dotnet", "dotnet")
		if _, err := p.stat(c); p.home != "" && err == nil {
			candidate = c
		} else if found, err := p.lookPath("dotnet"); err == nil {
			candidate = found
		}
	}
	if candidate == "" {
		return false
	}
	// arch が判明して不一致なら使えない（x64 dotnet を ARM に誤導入したケース。
	// Windows の PE は elfArch が "" を返し素通り＝従来どおり楽観）。
	if a := p.elfArch(candidate); a != "" && a != goarch {
		return false
	}
	// ARM SBC は .NET プロセス起動自体が遅い実績があるため 10s の防御値（R-C から踏襲）。
	out, err := p.runCmd(10*time.Second, candidate, "--list-runtimes")
	if err != nil {
		return false
	}
	return listedRuntimeSatisfies(out, req)
}

// listedRuntimeSatisfies は `dotnet --list-runtimes` の出力に要求を満たすベースランタイムが
// あるかを判定する（純関数）。行形式: "Microsoft.NETCore.App 10.0.8 [/path/...]"。
// prerelease 行は不充足扱い（LocalRuntimeSatisfies と同規則）。
func listedRuntimeSatisfies(out string, req RuntimeRequirement) bool {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != netCoreAppFramework {
			continue
		}
		if maj, min, pat, pre, ok := parseVersionTriple(fields[1]); ok && !pre && req.satisfiedBy(maj, min, pat) {
			return true
		}
	}
	return false
}
