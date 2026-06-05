// Package steam は DepotDownloader を用いた Resonite ヘッドレスの入手・更新を担う。
// 全OS（Windows/amd64・Linux/amd64・Linux/arm64）共通。SteamCMD は ARM 非対応のため使わない。
// 設計: docs/design/steam-depotdownloader.md
package steam

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ddVersion は固定する DepotDownloader のバージョン。GitHub の tag は "DepotDownloader_"+ddVersion。
// self-contained 配布（.NET 同梱）を使うため利用者は .NET を別途用意しなくてよい。
const ddVersion = "3.4.0"

// ddReleaseBase は self-contained アセットのダウンロード元（GitHub Releases）。
const ddReleaseBase = "https://github.com/SteamRE/DepotDownloader/releases/download/DepotDownloader_" + ddVersion

// ddAsset は OS/arch ごとの DepotDownloader self-contained 配布物。
type ddAsset struct {
	file   string // アセット zip 名
	sha256 string // zip の SHA-256（小文字hex・固定）
	exe    string // zip 内の実行ファイル名（root 直下）
}

// ddAssets は runtime.GOOS+"/"+runtime.GOARCH をキーにした対応表。
// self-contained 版＝.NET 不要。SHA-256 は DepotDownloader 3.4.0 の実アセットから算出（2026-06-05）。
// 版を上げる際は ddVersion と各 sha256 を必ず同時に更新する（実アセットをDLして Get-FileHash/sha256sum）。
var ddAssets = map[string]ddAsset{
	"windows/amd64": {file: "DepotDownloader-windows-x64.zip", sha256: "41c9e9f0df54b3ad02e67a11726756e5c73283bd7c2e1b04acfa5ae4c2ed3767", exe: "DepotDownloader.exe"},
	"linux/amd64":   {file: "DepotDownloader-linux-x64.zip", sha256: "a999dec66b4850fc961bd50366696d23c2d0fad7b18790e6a5647b2f19097a53", exe: "DepotDownloader"},
	"linux/arm64":   {file: "DepotDownloader-linux-arm64.zip", sha256: "d9fb612ccebc1db8eeea3b4045d2221ec70431381393ce908fb72f01d4f9c812", exe: "DepotDownloader"},
}

// ErrUnsupportedPlatform は現在の OS/arch 用の DepotDownloader 配布物が無いことを表す。
var ErrUnsupportedPlatform = errors.New("この OS/アーキテクチャ用の DepotDownloader 配布物がありません")

// platformKey は現在の実行プラットフォーム。テストで差し替えられるよう変数にする。
var platformKey = runtime.GOOS + "/" + runtime.GOARCH

// assetForPlatform は指定キーの ddAsset を返す。未対応なら ErrUnsupportedPlatform。
func assetForPlatform(key string) (ddAsset, error) {
	a, ok := ddAssets[key]
	if !ok {
		return ddAsset{}, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, key)
	}
	return a, nil
}

// Acquirer は DepotDownloader 本体の取得器。テストで BaseURL/Client を差し替えられる。
type Acquirer struct {
	BaseURL string       // self-contained アセットの取得元
	Client  *http.Client // HTTP クライアント
}

// NewAcquirer は既定（GitHub Releases・既定HTTPクライアント）の取得器を返す。
func NewAcquirer() *Acquirer {
	return &Acquirer{BaseURL: ddReleaseBase, Client: http.DefaultClient}
}

// Ensure は DepotDownloader 実行ファイルを用意してそのパスを返す（冪等）。
// 確定パス {toolsDir}/depotdownloader/{version}/<exe> に既に在ればDLせずスキップ。
// 無ければ固定版 self-contained を BaseURL からDL→SHA-256検証→展開→chmod→原子的rename する。
// 進捗・ログは logf（nil 可）に通知する。
func (a *Acquirer) Ensure(ctx context.Context, toolsDir string, logf func(string)) (string, error) {
	asset, err := assetForPlatform(platformKey)
	if err != nil {
		return "", err
	}
	destDir := filepath.Join(toolsDir, "depotdownloader", ddVersion)
	destExe := filepath.Join(destDir, asset.exe)
	if fi, err := os.Stat(destExe); err == nil && !fi.IsDir() {
		return destExe, nil // 取得済み（冪等スキップ）
	}
	url := a.BaseURL + "/" + asset.file
	logmsg(logf, fmt.Sprintf("DepotDownloader %s を取得します（%s）", ddVersion, asset.file))
	if err := a.downloadVerifyExtract(ctx, url, asset.sha256, asset.exe, destExe, logf); err != nil {
		return "", err
	}
	logmsg(logf, "DepotDownloader の取得が完了しました")
	return destExe, nil
}

// downloadVerifyExtract は url の zip をDLし、SHA-256 を検証し、zip 内 entryName を destExe へ
// 原子的に展開する（非windows は chmod 0755）。検証失敗・DL失敗時は確定ファイルも一時ファイルも残さない。
func (a *Acquirer) downloadVerifyExtract(ctx context.Context, url, expectedSHA, entryName, destExe string, logf func(string)) error {
	destDir := filepath.Dir(destExe)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("ディレクトリ作成に失敗: %w", err)
	}

	// 一時zipへDL（destDir と同一FSに置く＝後段の rename を原子的にできるよう近接させる）。
	tmpZip, err := os.CreateTemp(destDir, ".dd-*.zip")
	if err != nil {
		return fmt.Errorf("一時ファイル作成に失敗: %w", err)
	}
	tmpZipPath := tmpZip.Name()
	defer os.Remove(tmpZipPath) // 成否にかかわらず zip は残さない

	sum, dlErr := a.download(ctx, url, tmpZip)
	closeErr := tmpZip.Close()
	if dlErr != nil {
		return dlErr
	}
	if closeErr != nil {
		return fmt.Errorf("一時ファイルのクローズに失敗: %w", closeErr)
	}
	if !strings.EqualFold(sum, expectedSHA) {
		return fmt.Errorf("SHA-256 が一致しません（期待=%s 実際=%s）", expectedSHA, sum)
	}
	logmsg(logf, "SHA-256 検証 OK")

	// zip から実行ファイルを destExe と同一ディレクトリの一時名へ展開→原子的 rename で確定。
	tmpExe := destExe + ".tmp"
	if err := extractEntry(tmpZipPath, entryName, tmpExe); err != nil {
		os.Remove(tmpExe)
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpExe, 0o755); err != nil {
			os.Remove(tmpExe)
			return fmt.Errorf("実行権の付与に失敗: %w", err)
		}
	}
	if err := os.Rename(tmpExe, destExe); err != nil {
		os.Remove(tmpExe)
		return fmt.Errorf("確定パスへの配置に失敗: %w", err)
	}
	return nil
}

// download は url を w へストリームコピーしつつ SHA-256（小文字hex）を計算して返す。
func (a *Acquirer) download(ctx context.Context, url string, w io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "MRHC")
	resp, err := a.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ダウンロードに失敗: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ダウンロードに失敗: HTTP %d", resp.StatusCode)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), resp.Body); err != nil {
		return "", fmt.Errorf("ダウンロード中の読み込みに失敗: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractEntry は zipPath 内の entryName（root直下の固定名）を destPath へ書き出す。
// destPath は呼び出し側が決めた固定パスで、zip 内のパスは使わない（zip-slip の懸念なし）。
func extractEntry(zipPath, entryName, destPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("zip を開けません: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == entryName {
			return writeZipEntry(f, destPath)
		}
	}
	return fmt.Errorf("zip 内に実行ファイル %q がありません", entryName)
}

func writeZipEntry(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("zip エントリを開けません: %w", err)
	}
	defer rc.Close()
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("展開先を作成できません: %w", err)
	}
	_, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("展開に失敗: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("展開先のクローズに失敗: %w", closeErr)
	}
	return nil
}

func logmsg(logf func(string), s string) {
	if logf != nil {
		logf(s)
	}
}
