// extract.go はランタイムアーカイブの展開（tar.gz / zip）と、展開結果を
// <installDir>/dotnet-runtime へ入れ替えるスワップを担う。
package dotnetruntime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// extractArchive はアーカイブ arcPath を destDir へ全展開する（ext で形式を選ぶ）。
// tar はエントリの mode を保持する（dotnet host の実行ビットが要るため）。
func extractArchive(arcPath, ext, destDir string) error {
	if ext == ".zip" {
		return extractZipTree(arcPath, destDir)
	}
	return extractTarGz(arcPath, destDir)
}

// extractTarGz は tar.gz を destDir へ展開する。許可する型は Dir/Reg/Symlink のみ。
// エントリ名・リンク先は safeRelPath で検証し、destDir の外へ出るもの（tar-slip）は拒否する。
func extractTarGz(arcPath, destDir string) error {
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
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar の読み取りに失敗: %w", err)
		}
		rel, err := safeRelPath(hdr.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue // "./" 等のルートエントリ
		}
		target := filepath.Join(destDir, rel)
		switch hdr.Typeflag {
		case tar.TypeDir:
			// |0o700: 展開中の書き込みを保証（アーカイブの mode が読み取り専用でも継続できる）
			if err := os.MkdirAll(target, hdr.FileInfo().Mode().Perm()|0o700); err != nil {
				return fmt.Errorf("ディレクトリ作成に失敗: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("ディレクトリ作成に失敗: %w", err)
			}
			if err := writeFileFrom(tr, target, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			link := strings.ReplaceAll(hdr.Linkname, "\\", "/")
			joined := path.Join(path.Dir(filepath.ToSlash(rel)), link)
			if path.IsAbs(link) || !filepath.IsLocal(filepath.FromSlash(joined)) {
				return fmt.Errorf("アーカイブ内のリンク先が不正です: %q -> %q", hdr.Name, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("ディレクトリ作成に失敗: %w", err)
			}
			if err := os.Symlink(filepath.FromSlash(link), target); err != nil {
				return fmt.Errorf("シンボリックリンクの作成に失敗: %w", err)
			}
		case tar.TypeXGlobalHeader:
			// pax global header（メタデータのみ）は実体を持たない
		default:
			return fmt.Errorf("アーカイブ内に未対応のエントリ型があります: %q (type %c)", hdr.Name, hdr.Typeflag)
		}
	}
}

// extractZipTree は zip を destDir へ全展開する（Windows 用。実行ビットは不要）。
func extractZipTree(arcPath, destDir string) error {
	zr, err := zip.OpenReader(arcPath)
	if err != nil {
		return fmt.Errorf("zip を開けません: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		rel, err := safeRelPath(f.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(destDir, rel)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("ディレクトリ作成に失敗: %w", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("ディレクトリ作成に失敗: %w", err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("zip エントリを開けません: %w", err)
		}
		writeErr := writeFileFrom(rc, target, f.Mode())
		rc.Close()
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// safeRelPath はアーカイブ内エントリ名を destDir 配下の相対パスへ正規化する。
// 空（ルート "./" 等）は ("" , nil)。絶対パス・".." 脱出（tar-slip / zip-slip）はエラー。
func safeRelPath(name string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if clean == "." {
		return "", nil
	}
	if path.IsAbs(clean) || !filepath.IsLocal(filepath.FromSlash(clean)) {
		return "", fmt.Errorf("アーカイブ内のパスが不正です: %q", name)
	}
	return filepath.FromSlash(clean), nil
}

// writeFileFrom は r の内容を target へ書き出す（mode はアーカイブの値を保持）。
func writeFileFrom(r io.Reader, target string, mode fs.FileMode) error {
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("展開先を作成できません: %w", err)
	}
	_, copyErr := io.Copy(out, r)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("展開に失敗: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("展開先のクローズに失敗: %w", closeErr)
	}
	return nil
}

// swapRuntimeDir は展開済み stageDir を finalDir へ入れ替える。
// Windows は非空ディレクトリへの rename ができないため、既存→.old / stage→final / .old 削除の
// 2段 rename で行う（全置換。旧 minor の併存や他 arch の残骸を残さない）。
func swapRuntimeDir(installDir, stageDir, finalDir string) error {
	oldDir := filepath.Join(installDir, ".dotnet-runtime.old")
	if err := os.RemoveAll(oldDir); err != nil {
		return fmt.Errorf("stale な退避先の掃除に失敗: %w", err)
	}
	moved := false
	if _, err := os.Stat(finalDir); err == nil {
		if err := os.Rename(finalDir, oldDir); err != nil {
			return fmt.Errorf("既存ランタイムの退避に失敗: %w", err)
		}
		moved = true
	}
	if err := os.Rename(stageDir, finalDir); err != nil {
		if moved {
			_ = os.Rename(oldDir, finalDir) // 旧状態の復元を試みる（best-effort）
		}
		return fmt.Errorf("ランタイムの配置に失敗: %w", err)
	}
	_ = os.RemoveAll(oldDir) // 旧版の削除失敗は致命ではない（次回 Ensure 冒頭で再掃除）
	return nil
}

// recoverStaleSwap は過去の中断で残った中間状態を片づける（Ensure 冒頭で呼ぶ）。
//   - final が無く .old だけ残る（2段 rename の間で中断）→ .old を final へ復元
//   - .new は常に作り直すため無条件で削除・final が在るときの .old も削除
func recoverStaleSwap(installDir, finalDir string) {
	oldDir := filepath.Join(installDir, ".dotnet-runtime.old")
	if _, err := os.Stat(finalDir); os.IsNotExist(err) {
		if _, err := os.Stat(oldDir); err == nil {
			_ = os.Rename(oldDir, finalDir)
		}
	} else {
		_ = os.RemoveAll(oldDir)
	}
	_ = os.RemoveAll(filepath.Join(installDir, ".dotnet-runtime.new"))
}
