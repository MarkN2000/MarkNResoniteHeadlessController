package server

// ログ閲覧 API（GET /logs・GET /logs/{name}）のテスト。
// logsDir を temp の {install}/Headless/Logs に向け、一覧の並び/フィルタ・本文取得・
// 末尾切り詰め・不正名/未存在の拒否を検証する。

import (
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

// newLogsServer は logsDir を temp に向けた停止中サーバーを返す（makeLogs=false なら Logs 未作成）。
func newLogsServer(t *testing.T, makeLogs bool) (ts *httptest.Server, pw, logsDir string) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	install := filepath.Join(tmp, "resonite")
	logsDir = filepath.Join(install, "Headless", "Logs")
	if makeLogs {
		if err := os.MkdirAll(logsDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "logs-test-secret",
		Steam:             &config.Steam{InstallDir: install}, // logsDir = install/Headless/Logs
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	srv := New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, testPassword, logsDir
}

func writeLog(t *testing.T, dir, name, content string, mod time.Time) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestLogList_SortedNewestFirstAndFiltered(t *testing.T) {
	ts, pw, dir := newLogsServer(t, true)
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	writeLog(t, dir, "old.log", "old", base)
	writeLog(t, dir, "new.log", "new", base.Add(48*time.Hour))
	writeLog(t, dir, "mid.log", "mid", base.Add(24*time.Hour))
	writeLog(t, dir, "notes.txt", "ignore me", base.Add(72*time.Hour)) // .log 以外は除外

	var got okEnv[[]logFileInfo]
	if code := authGet(t, ts.URL+"/api/v1/logs", pw, &got); code != http.StatusOK {
		t.Fatalf("code=%d", code)
	}
	names := make([]string, len(got.Data))
	for i, f := range got.Data {
		names[i] = f.Name
	}
	want := []string{"new.log", "mid.log", "old.log"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v (and .txt excluded)", names, want)
	}
}

func TestLogList_MissingDirEmpty(t *testing.T) {
	ts, pw, _ := newLogsServer(t, false) // Logs フォルダ未作成
	var got okEnv[[]logFileInfo]
	if code := authGet(t, ts.URL+"/api/v1/logs", pw, &got); code != http.StatusOK {
		t.Fatalf("code=%d", code)
	}
	if !got.OK || len(got.Data) != 0 {
		t.Fatalf("expected empty list, got %+v", got)
	}
}

func TestLogGet_Content(t *testing.T) {
	ts, pw, dir := newLogsServer(t, true)
	writeLog(t, dir, "session.log", "hello\n日本語\n", time.Now())
	var got okEnv[struct {
		Name      string `json:"name"`
		Size      int64  `json:"size"`
		Truncated bool   `json:"truncated"`
		Content   string `json:"content"`
	}]
	if code := authGet(t, ts.URL+"/api/v1/logs/session.log", pw, &got); code != http.StatusOK {
		t.Fatalf("code=%d", code)
	}
	if got.Data.Truncated || got.Data.Content != "hello\n日本語\n" {
		t.Fatalf("content mismatch: %+v", got.Data)
	}
}

func TestLogGet_TailTruncation(t *testing.T) {
	ts, pw, dir := newLogsServer(t, true)
	// maxLogTailBytes を超える内容を行単位で作る。末尾は切り出され、先頭は最初の改行まで捨てられる。
	var b strings.Builder
	line := "0123456789ABCDEF0123456789ABCDEF\n" // 33 bytes
	for b.Len() <= maxLogTailBytes+5000 {
		b.WriteString(line)
	}
	full := b.String()
	writeLog(t, dir, "big.log", full, time.Now())

	var got okEnv[struct {
		Size      int64  `json:"size"`
		Truncated bool   `json:"truncated"`
		Content   string `json:"content"`
	}]
	if code := authGet(t, ts.URL+"/api/v1/logs/big.log", pw, &got); code != http.StatusOK {
		t.Fatalf("code=%d", code)
	}
	if !got.Data.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if got.Data.Size != int64(len(full)) {
		t.Fatalf("size should be the full file size: got=%d want=%d", got.Data.Size, len(full))
	}
	if int64(len(got.Data.Content)) >= got.Data.Size {
		t.Fatalf("content should be smaller than full file")
	}
	// 行頭から始まる（途中行で切れていない）= 期待する行で開始。
	if !strings.HasPrefix(got.Data.Content, line) {
		t.Fatalf("content should start at a line boundary, got prefix %q", got.Data.Content[:min(40, len(got.Data.Content))])
	}
}

func TestLogDownload_FullContent(t *testing.T) {
	ts, pw, dir := newLogsServer(t, true)
	name := "日本語 session.log"
	full := "BEGIN\n" + strings.Repeat("x", maxLogTailBytes) + "\nEND\n"
	writeLog(t, dir, name, full, time.Now())

	resp := authReq(t, http.MethodGet, ts.URL+"/api/v1/logs/"+url.PathEscape(name)+"/download", pw, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("code=%d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != full {
		t.Fatalf("download should return full content: got=%d bytes want=%d", len(body), len(full))
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err != nil {
		t.Fatalf("Content-Disposition: %v", err)
	}
	if mediaType != "attachment" || params["filename"] != name {
		t.Fatalf("Content-Disposition=%q params=%v", mediaType, params)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type=%q", got)
	}
}

func TestLogDownload_RequiresAuth(t *testing.T) {
	ts, _, dir := newLogsServer(t, true)
	writeLog(t, dir, "session.log", "secret", time.Now())

	resp, err := http.Get(ts.URL + "/api/v1/logs/session.log/download")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("code=%d, want=%d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestLogGet_RejectsBadNames(t *testing.T) {
	ts, pw, dir := newLogsServer(t, true)
	writeLog(t, dir, "ok.log", "x", time.Now())

	// .log 以外は 400
	if code := authGet(t, ts.URL+"/api/v1/logs/secret.txt", pw, nil); code != http.StatusBadRequest {
		t.Fatalf("non-.log should be 400, got %d", code)
	}
	// 存在しない .log は 404
	if code := authGet(t, ts.URL+"/api/v1/logs/missing.log", pw, nil); code != http.StatusNotFound {
		t.Fatalf("missing .log should be 404, got %d", code)
	}
	// ダウンロードも同じファイル名制約を使う。
	resp := authReq(t, http.MethodGet, ts.URL+"/api/v1/logs/secret.txt/download", pw, "", "")
	if code := decodeResp(t, resp, nil); code != http.StatusBadRequest {
		t.Fatalf("download of non-.log should be 400, got %d", code)
	}
	resp = authReq(t, http.MethodGet, ts.URL+"/api/v1/logs/missing.log/download", pw, "", "")
	if code := decodeResp(t, resp, nil); code != http.StatusNotFound {
		t.Fatalf("download of missing .log should be 404, got %d", code)
	}
}
