// extract.go はリリースアーカイブ（zip / tar.gz）から実行ファイル1個だけを取り出す処理と、
// 取り出したバイナリが本当に対象 OS/arch の実行ファイルかの検査を担う。
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"debug/elf"
	"debug/pe"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// maxBinarySize は取り出す実行ファイルのサイズ上限。アーカイブのヘッダ申告は信用せず
// 実コピー量で制限する（誤アセット・zip bomb 級の事故からディスクを守る保険）。
const maxBinarySize = 200 << 20 // 200 MiB

// extractBinary はアーカイブ arcPath から entryName（"mrhc-<os>-<arch>/mrhc(.exe)"）だけを
// destPath へ書き出す（mode 0755・書込後に fsync）。形式は assetFile の拡張子で選ぶ
// （arcPath は一時ファイル名で拡張子に意味がない）。アーカイブ内のパスはファイルパス構築に
// 使わないため zip-slip の懸念はない。symlink エントリ等の非通常ファイルは明示エラー。
func extractBinary(arcPath, assetFile, entryName, destPath string) error {
	if strings.HasSuffix(assetFile, ".zip") {
		return extractZipBinary(arcPath, entryName, destPath)
	}
	return extractTarGzBinary(arcPath, entryName, destPath)
}

func extractZipBinary(arcPath, entryName, destPath string) error {
	zr, err := zip.OpenReader(arcPath)
	if err != nil {
		return fmt.Errorf("zip を開けません: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if cleanEntryName(f.Name) != entryName {
			continue
		}
		// zip の symlink エントリは Open() がリンク先文字列を内容として返すため、
		// 検査しないと数十バイトのテキストを実行ファイルとして配置してしまう。
		if !f.Mode().IsRegular() {
			return fmt.Errorf("アーカイブ内の %q が通常ファイルではありません", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("zip エントリを開けません: %w", err)
		}
		defer rc.Close()
		return writeBinary(rc, destPath)
	}
	return fmt.Errorf("アーカイブ内に %q がありません", entryName)
}

func extractTarGzBinary(arcPath, entryName, destPath string) error {
	f, err := os.Open(arcPath)
	if err != nil {
		return fmt.Errorf("アーカイブを開けません: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip の読み取りに失敗: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("アーカイブ内に %q がありません", entryName)
		}
		if err != nil {
			return fmt.Errorf("tar の読み取りに失敗: %w", err)
		}
		if cleanEntryName(hdr.Name) != entryName {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("アーカイブ内の %q が通常ファイルではありません (type %c)", hdr.Name, hdr.Typeflag)
		}
		return writeBinary(tr, destPath)
	}
}

// cleanEntryName はエントリ名を比較用に正規化する（"./" 接頭辞や "\" 区切りのゆらぎを吸収）。
func cleanEntryName(name string) string {
	return path.Clean(strings.ReplaceAll(name, "\\", "/"))
}

// writeBinary は r の内容を destPath へ書き出す（0755・サイズ上限・fsync）。
// fsync は rename 直前の電源断で確定パスが空ファイル化するのを防ぐ。
func writeBinary(r io.Reader, destPath string) error {
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("展開先を作成できません: %w", err)
	}
	n, copyErr := io.Copy(out, io.LimitReader(r, maxBinarySize+1))
	if copyErr == nil && n > maxBinarySize {
		copyErr = fmt.Errorf("実行ファイルがサイズ上限（%d MiB）を超えています", maxBinarySize>>20)
	}
	var syncErr error
	if copyErr == nil {
		syncErr = out.Sync()
	}
	closeErr := out.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("展開に失敗: %w", copyErr)
	case syncErr != nil:
		return fmt.Errorf("展開先の同期に失敗: %w", syncErr)
	case closeErr != nil:
		return fmt.Errorf("展開先のクローズに失敗: %w", closeErr)
	}
	return nil
}

// peMachines / elfMachines は GOARCH と実行ファイルヘッダ上のマシン種別の対応。
var peMachines = map[string]uint16{
	"amd64": pe.IMAGE_FILE_MACHINE_AMD64,
	"arm64": pe.IMAGE_FILE_MACHINE_ARM64,
}

var elfMachines = map[string]elf.Machine{
	"amd64": elf.EM_X86_64,
	"arm64": elf.EM_AARCH64,
}

// verifyBinaryFormat は path が key（"<goos>/<goarch>"）に対応する実行ファイル形式かを検査する。
// リリースワークフローの事故（別 OS/arch の中身・バイナリ以外の混入）を入れ替え前に検出する保険。
func verifyBinaryFormat(filePath, key string) error {
	goos, goarch, _ := strings.Cut(key, "/")
	if goos == "windows" {
		f, err := pe.Open(filePath)
		if err != nil {
			return fmt.Errorf("Windows 実行ファイル（PE）として読めません: %w", err)
		}
		defer f.Close()
		if want := peMachines[goarch]; f.Machine != want {
			return fmt.Errorf("実行ファイルのアーキテクチャが一致しません（期待 %s・実際 PE machine 0x%X）", goarch, f.Machine)
		}
		return nil
	}
	f, err := elf.Open(filePath)
	if err != nil {
		return fmt.Errorf("Linux 実行ファイル（ELF）として読めません: %w", err)
	}
	defer f.Close()
	if want := elfMachines[goarch]; f.Machine != want {
		return fmt.Errorf("実行ファイルのアーキテクチャが一致しません（期待 %s・実際 ELF machine %v）", goarch, f.Machine)
	}
	return nil
}
