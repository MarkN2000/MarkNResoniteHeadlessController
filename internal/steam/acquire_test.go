package steam

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeZip は entry 1件だけを含む zip のバイト列を返す（DD アセットを模した検証用）。
func makeZip(t *testing.T, entry, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(entry)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestAssetForPlatform(t *testing.T) {
	if _, err := assetForPlatform("linux/arm64"); err != nil {
		t.Fatalf("linux/arm64 は対応しているべき: %v", err)
	}
	_, err := assetForPlatform("plan9/mips")
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("未対応プラットフォームは ErrUnsupportedPlatform を返すべき: %v", err)
	}
}

// TestDDAssetsPinned は固定対象（3プラットフォーム）が SHA-256 64桁・file/exe 非空で揃っていることを検証する。
func TestDDAssetsPinned(t *testing.T) {
	for _, key := range []string{"windows/amd64", "linux/amd64", "linux/arm64"} {
		a, ok := ddAssets[key]
		if !ok {
			t.Errorf("固定対象が欠けている: %s", key)
			continue
		}
		if len(a.sha256) != 64 {
			t.Errorf("%s: sha256 桁数=%d（64であるべき）", key, len(a.sha256))
		}
		if a.file == "" || a.exe == "" {
			t.Errorf("%s: file/exe が空", key)
		}
	}
}

func TestDownloadVerifyExtract(t *testing.T) {
	content := "fake depotdownloader binary"
	zipBytes := makeZip(t, "DepotDownloader", content)
	wantSHA := sha256hex(zipBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destExe := filepath.Join(dir, "depotdownloader", ddVersion, "DepotDownloader")
	a := &Acquirer{BaseURL: srv.URL, Client: srv.Client()}
	if err := a.downloadVerifyExtract(context.Background(), srv.URL+"/dd.zip", wantSHA, "DepotDownloader", destExe, nil); err != nil {
		t.Fatalf("downloadVerifyExtract: %v", err)
	}
	got, err := os.ReadFile(destExe)
	if err != nil {
		t.Fatalf("read destExe: %v", err)
	}
	if string(got) != content {
		t.Errorf("内容不一致: %q", got)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(destExe)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&0o100 == 0 {
			t.Error("実行ビットが立っていない")
		}
	}
	// 一時ファイル（zip / .tmp）が残っていないこと
	entries, _ := os.ReadDir(filepath.Dir(destExe))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".dd-") || strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("一時ファイルが残存: %s", e.Name())
		}
	}
}

func TestDownloadVerifyExtract_SHAMismatch(t *testing.T) {
	zipBytes := makeZip(t, "DepotDownloader", "x")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destExe := filepath.Join(dir, "DepotDownloader")
	a := &Acquirer{BaseURL: srv.URL, Client: srv.Client()}
	err := a.downloadVerifyExtract(context.Background(), srv.URL+"/dd.zip", strings.Repeat("0", 64), "DepotDownloader", destExe, nil)
	if err == nil {
		t.Fatal("SHA 不一致でエラーになるべき")
	}
	if _, statErr := os.Stat(destExe); statErr == nil {
		t.Error("不一致時は確定ファイルを残さないべき")
	}
}

func TestDownloadVerifyExtract_MissingEntry(t *testing.T) {
	zipBytes := makeZip(t, "SomethingElse", "x")
	wantSHA := sha256hex(zipBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destExe := filepath.Join(dir, "DepotDownloader")
	a := &Acquirer{BaseURL: srv.URL, Client: srv.Client()}
	err := a.downloadVerifyExtract(context.Background(), srv.URL+"/dd.zip", wantSHA, "DepotDownloader", destExe, nil)
	if err == nil {
		t.Fatal("目的エントリ不在でエラーになるべき")
	}
}

// TestEnsure_Idempotent は確定パスに既に在れば DL せずスキップすることを検証する
// （到達不能な BaseURL を渡し、ネットワークに行かないことを担保）。
func TestEnsure_Idempotent(t *testing.T) {
	old := platformKey
	platformKey = "linux/amd64"
	defer func() { platformKey = old }()

	dir := t.TempDir()
	destDir := filepath.Join(dir, "depotdownloader", ddVersion)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destExe := filepath.Join(destDir, "DepotDownloader")
	if err := os.WriteFile(destExe, []byte("preexisting"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := &Acquirer{BaseURL: "http://invalid.invalid", Client: &http.Client{}}
	got, err := a.Ensure(context.Background(), dir, nil)
	if err != nil {
		t.Fatalf("既存時の Ensure はエラーにならないべき: %v", err)
	}
	if got != destExe {
		t.Errorf("got %q want %q", got, destExe)
	}
}
