package dotnetruntime

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// tarEntry はテスト用 tar アーカイブの 1 エントリ。
type tarEntry struct {
	name     string
	typeflag byte
	mode     int64
	data     string
	linkname string
}

func buildTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: e.mode, Linkname: e.linkname}
		if e.typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.data))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// runtimeTarGz は最小構成（host + shared 版ディレクトリ + ライブラリ1個）のランタイム tar.gz。
func runtimeTarGz(t *testing.T, version string) []byte {
	return buildTarGz(t, []tarEntry{
		{name: "./", typeflag: tar.TypeDir, mode: 0o755},
		{name: "./dotnet", typeflag: tar.TypeReg, mode: 0o755, data: "#!host"},
		{name: "./shared/Microsoft.NETCore.App/" + version + "/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "./shared/Microsoft.NETCore.App/" + version + "/libcoreclr.so", typeflag: tar.TypeReg, mode: 0o644, data: "lib"},
	})
}

// fakeFeed は latest.version / アーカイブ / .sha512 を配信する偽フィード。
type fakeFeed struct {
	srv           *httptest.Server
	latestBody    string
	archiveName   string // 例 "dotnet-runtime-10.0.8-linux-x64.tar.gz"
	archive       []byte
	shaBody       string // 空なら archive から自動計算
	archiveHits   atomic.Int64
	archiveSlowMS int // >0 なら最初の数バイト送信後にこのミリ秒待つ（無進捗テスト用）
}

func newFakeFeed(t *testing.T, channel, latestBody, archiveName string, archive []byte) *fakeFeed {
	t.Helper()
	f := &fakeFeed{latestBody: latestBody, archiveName: archiveName, archive: archive}
	mux := http.NewServeMux()
	mux.HandleFunc("/dotnet/Runtime/"+channel+"/latest.version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, f.latestBody)
	})
	mux.HandleFunc("/dotnet/Runtime/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+f.archiveName):
			f.archiveHits.Add(1)
			if f.archiveSlowMS > 0 {
				w.Header().Set("Content-Length", fmt.Sprint(len(f.archive)))
				w.Write(f.archive[:4])
				w.(http.Flusher).Flush()
				time.Sleep(time.Duration(f.archiveSlowMS) * time.Millisecond)
				return
			}
			w.Write(f.archive)
		case strings.HasSuffix(r.URL.Path, "/"+f.archiveName+".sha512"):
			body := f.shaBody
			if body == "" {
				sum := sha512.Sum512(f.archive)
				body = hex.EncodeToString(sum[:]) + "  " + f.archiveName + "\n"
			}
			fmt.Fprint(w, body)
		default:
			http.NotFound(w, r)
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeFeed) acquirer() *Acquirer {
	return &Acquirer{FeedBase: f.srv.URL + "/dotnet", Client: f.srv.Client(), IdleTimeout: 5 * time.Second}
}

// setPlatform は platformKey を差し替える（テスト終了時に復元）。
func setPlatform(t *testing.T, key string) {
	t.Helper()
	orig := platformKey
	platformKey = key
	t.Cleanup(func() { platformKey = orig })
}

func collectEvents() (*[]Event, func(Event)) {
	var events []Event
	return &events, func(e Event) { events = append(events, e) }
}

func hasEvent(events []Event, kind, msgKey string) bool {
	for _, e := range events {
		if e.Kind == kind && (msgKey == "" || e.MsgKey == msgKey) {
			return true
		}
	}
	return false
}

func TestRidFor(t *testing.T) {
	cases := []struct {
		key, rid, ext string
		wantErr       bool
	}{
		{"windows/amd64", "win-x64", ".zip", false},
		{"linux/amd64", "linux-x64", ".tar.gz", false},
		{"linux/arm64", "linux-arm64", ".tar.gz", false},
		{"darwin/arm64", "", "", true},
	}
	for _, tc := range cases {
		rid, ext, err := ridFor(tc.key)
		if (err != nil) != tc.wantErr || rid != tc.rid || ext != tc.ext {
			t.Errorf("ridFor(%q) = %q, %q, %v", tc.key, rid, ext, err)
		}
	}
}

func TestParseLatestVersion(t *testing.T) {
	cases := []struct {
		name, body, want string
		wantErr          bool
	}{
		{"1行", "10.0.8", "10.0.8", false},
		{"1行+改行", "10.0.8\n", "10.0.8", false},
		{"2行（hash+版）", "abc123def\n10.0.8\n", "10.0.8", false},
		{"CRLF", "abc123def\r\n10.0.8\r\n", "10.0.8", false},
		{"空", "\n\n", "", true},
	}
	for _, tc := range cases {
		got, err := parseLatestVersion(tc.body)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: parseLatestVersion(%q) = %q, %v", tc.name, tc.body, got, err)
		}
	}
}

func TestEnsureLinuxTarGz(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "10.0", "10.0.8\n", "dotnet-runtime-10.0.8-linux-x64.tar.gz", runtimeTarGz(t, "10.0.8"))
	dir := t.TempDir()
	events, onEvent := collectEvents()

	version, err := feed.acquirer().Ensure(context.Background(), dir, "10.0", onEvent)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if version != "10.0.8" {
		t.Errorf("version = %q", version)
	}
	host := filepath.Join(dir, "dotnet-runtime", "dotnet")
	fi, err := os.Stat(host)
	if err != nil {
		t.Fatalf("host が無い: %v", err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("host に実行ビットが無い: %v", fi.Mode())
	}
	if _, err := os.Stat(filepath.Join(dir, "dotnet-runtime", "shared", "Microsoft.NETCore.App", "10.0.8", "libcoreclr.so")); err != nil {
		t.Errorf("shared が展開されていない: %v", err)
	}
	if !hasEvent(*events, "milestone", "") || !hasEvent(*events, "log", "dotnetInstall") || !hasEvent(*events, "log", "dotnetInstalled") {
		t.Errorf("イベント不足: %+v", *events)
	}
	if !hasEvent(*events, "progress", "") {
		t.Errorf("progress イベントが無い")
	}
	// 中間生成物が残っていない
	for _, leftover := range []string{".dotnet-runtime.new", ".dotnet-runtime.old"} {
		if _, err := os.Stat(filepath.Join(dir, leftover)); !os.IsNotExist(err) {
			t.Errorf("%s が残っている", leftover)
		}
	}
}

func TestEnsureWindowsZip(t *testing.T) {
	setPlatform(t, "windows/amd64")
	archive := buildZip(t, map[string]string{
		"dotnet.exe": "host",
		"shared/Microsoft.NETCore.App/10.0.8/coreclr.dll": "lib",
	})
	feed := newFakeFeed(t, "10.0", "10.0.8", "dotnet-runtime-10.0.8-win-x64.zip", archive)
	dir := t.TempDir()

	version, err := feed.acquirer().Ensure(context.Background(), dir, "10.0", nil)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if version != "10.0.8" {
		t.Errorf("version = %q", version)
	}
	for _, p := range []string{
		filepath.Join(dir, "dotnet-runtime", "dotnet.exe"),
		filepath.Join(dir, "dotnet-runtime", "shared", "Microsoft.NETCore.App", "10.0.8", "coreclr.dll"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s が無い: %v", p, err)
		}
	}
}

func TestEnsureIdempotentSkip(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "10.0", "10.0.8", "dotnet-runtime-10.0.8-linux-x64.tar.gz", runtimeTarGz(t, "10.0.8"))
	dir := t.TempDir()
	// 設置済み状態を作る
	if err := os.MkdirAll(filepath.Join(dir, "dotnet-runtime", "shared", "Microsoft.NETCore.App", "10.0.8"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dotnet-runtime", "dotnet"), []byte("host"), 0o755); err != nil {
		t.Fatal(err)
	}
	events, onEvent := collectEvents()

	version, err := feed.acquirer().Ensure(context.Background(), dir, "10.0", onEvent)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if version != "10.0.8" {
		t.Errorf("version = %q", version)
	}
	if feed.archiveHits.Load() != 0 {
		t.Errorf("設置済みなのにアーカイブを取得した（%d 回）", feed.archiveHits.Load())
	}
	if len(*events) != 0 {
		t.Errorf("設置済みスキップでイベントが出た: %+v", *events)
	}
}

func TestEnsurePrereleaseRefused(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "11.0", "11.0.0-preview.4.26230.115", "unused.tar.gz", nil)
	_, err := feed.acquirer().Ensure(context.Background(), t.TempDir(), "11.0", nil)
	if err == nil || !strings.Contains(err.Error(), "プレリリース") {
		t.Fatalf("プレリリース拒否になっていない: %v", err)
	}
	if feed.archiveHits.Load() != 0 {
		t.Errorf("プレリリースなのにアーカイブを取得した")
	}
}

func TestEnsureBadLatestVersion(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "10.0", "<html>error</html>", "unused.tar.gz", nil)
	if _, err := feed.acquirer().Ensure(context.Background(), t.TempDir(), "10.0", nil); err == nil {
		t.Fatal("不正な latest.version でエラーにならない")
	}
}

func TestEnsureSHAMismatch(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "10.0", "10.0.8", "dotnet-runtime-10.0.8-linux-x64.tar.gz", runtimeTarGz(t, "10.0.8"))
	feed.shaBody = strings.Repeat("ab", 64) + "  dotnet-runtime-10.0.8-linux-x64.tar.gz\n"
	dir := t.TempDir()

	_, err := feed.acquirer().Ensure(context.Background(), dir, "10.0", nil)
	if err == nil || !strings.Contains(err.Error(), "SHA-512") {
		t.Fatalf("SHA 不一致がエラーにならない: %v", err)
	}
	// 部分残しゼロ（確定ディレクトリ・stage・一時アーカイブ）
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("失敗後に残骸がある: %v", names)
	}
}

func TestEnsureBadChecksumFormat(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "10.0", "10.0.8", "dotnet-runtime-10.0.8-linux-x64.tar.gz", runtimeTarGz(t, "10.0.8"))
	feed.shaBody = "not-a-checksum\n"
	if _, err := feed.acquirer().Ensure(context.Background(), t.TempDir(), "10.0", nil); err == nil ||
		!strings.Contains(err.Error(), "チェックサム") {
		t.Fatalf("チェックサム形式不正がエラーにならない: %v", err)
	}
}

func TestEnsureReplacesExisting(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "10.0", "10.0.9", "dotnet-runtime-10.0.9-linux-x64.tar.gz", runtimeTarGz(t, "10.0.9"))
	dir := t.TempDir()
	// 旧ランタイム（別版＋ゴミファイル）を設置済みにする
	old := filepath.Join(dir, "dotnet-runtime")
	if err := os.MkdirAll(filepath.Join(old, "shared", "Microsoft.NETCore.App", "10.0.2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "stale-marker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := feed.acquirer().Ensure(context.Background(), dir, "10.0", nil); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(old, "stale-marker")); !os.IsNotExist(err) {
		t.Error("全置換されていない（旧ファイルが残存）")
	}
	if _, err := os.Stat(filepath.Join(old, "shared", "Microsoft.NETCore.App", "10.0.9")); err != nil {
		t.Errorf("新版が設置されていない: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".dotnet-runtime.old")); !os.IsNotExist(err) {
		t.Error(".old が残っている")
	}
}

func TestEnsureTarSlipRejected(t *testing.T) {
	setPlatform(t, "linux/amd64")
	evil := buildTarGz(t, []tarEntry{
		{name: "../evil", typeflag: tar.TypeReg, mode: 0o644, data: "x"},
	})
	feed := newFakeFeed(t, "10.0", "10.0.8", "dotnet-runtime-10.0.8-linux-x64.tar.gz", evil)
	dir := t.TempDir()
	if _, err := feed.acquirer().Ensure(context.Background(), dir, "10.0", nil); err == nil ||
		!strings.Contains(err.Error(), "パスが不正") {
		t.Fatalf("tar-slip が拒否されない: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil")); !os.IsNotExist(err) {
		t.Error("destDir の外に書かれた")
	}
}

func TestEnsureBadSymlinkRejected(t *testing.T) {
	setPlatform(t, "linux/amd64")
	evil := buildTarGz(t, []tarEntry{
		{name: "./link", typeflag: tar.TypeSymlink, mode: 0o777, linkname: "../../etc/passwd"},
	})
	feed := newFakeFeed(t, "10.0", "10.0.8", "dotnet-runtime-10.0.8-linux-x64.tar.gz", evil)
	if _, err := feed.acquirer().Ensure(context.Background(), t.TempDir(), "10.0", nil); err == nil ||
		!strings.Contains(err.Error(), "リンク先が不正") {
		t.Fatalf("不正 symlink が拒否されない: %v", err)
	}
}

func TestEnsureIdleTimeout(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "10.0", "10.0.8", "dotnet-runtime-10.0.8-linux-x64.tar.gz", runtimeTarGz(t, "10.0.8"))
	feed.archiveSlowMS = 1500
	a := feed.acquirer()
	a.IdleTimeout = 100 * time.Millisecond

	_, err := a.Ensure(context.Background(), t.TempDir(), "10.0", nil)
	if err == nil || !strings.Contains(err.Error(), "無進捗") {
		t.Fatalf("無進捗打ち切りになっていない: %v", err)
	}
}

func TestEnsureContextCancel(t *testing.T) {
	setPlatform(t, "linux/amd64")
	feed := newFakeFeed(t, "10.0", "10.0.8", "dotnet-runtime-10.0.8-linux-x64.tar.gz", runtimeTarGz(t, "10.0.8"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 即キャンセル
	if _, err := feed.acquirer().Ensure(ctx, t.TempDir(), "10.0", nil); err == nil {
		t.Fatal("キャンセル済み ctx でエラーにならない")
	}
}

func TestRecoverStaleSwap(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "dotnet-runtime")
	old := filepath.Join(dir, ".dotnet-runtime.old")
	if err := os.MkdirAll(filepath.Join(old, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".dotnet-runtime.new"), 0o755); err != nil {
		t.Fatal(err)
	}

	recoverStaleSwap(dir, final)

	if _, err := os.Stat(filepath.Join(final, "shared")); err != nil {
		t.Errorf(".old が final へ復元されていない: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".dotnet-runtime.new")); !os.IsNotExist(err) {
		t.Error(".new が掃除されていない")
	}
}
