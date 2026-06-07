// helpers_test.go はテスト用の素材（偽リリースサーバー・最小 PE/ELF・アーカイブ生成）。
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// setPlatform はテスト中だけ platformKey を差し替える。
func setPlatform(t *testing.T, key string) {
	t.Helper()
	orig := platformKey
	platformKey = key
	t.Cleanup(func() { platformKey = orig })
}

// newTestUpdater は偽サーバー向けの Updater を返す。
func newTestUpdater(baseURL, version, exePath string) *Updater {
	return &Updater{
		BaseURL: baseURL,
		Version: version,
		ExePath: exePath,
		CheckClient: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		DLClient: &http.Client{},
	}
}

// serveRelease は GitHub Releases を模した httptest サーバーを立てる。
//   - /releases/latest → tag が空なら 404、それ以外は /releases/tag/<tag> へ 302
//   - /releases/download/<tag>/<file> → files の内容（SHA256SUMS は自動生成。
//     files に "SHA256SUMS" キーがあればそれで上書き＝改竄テスト用）
func serveRelease(t *testing.T, tag string, files map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if tag == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Location", "/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/releases/download/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/releases/download/"+tag+"/") {
			http.NotFound(w, r)
			return
		}
		name := path.Base(r.URL.Path)
		if b, ok := files[name]; ok {
			_, _ = w.Write(b)
			return
		}
		if name == "SHA256SUMS" {
			var sb strings.Builder
			for f, b := range files {
				sum := sha256.Sum256(b)
				fmt.Fprintf(&sb, "%s  %s\n", hex.EncodeToString(sum[:]), f)
			}
			_, _ = w.Write([]byte(sb.String()))
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fakeELF は debug/elf が解釈できる最小の ELF64 ヘッダ＋filler を返す。
func fakeELF(machine elf.Machine, filler string) []byte {
	b := make([]byte, 64)
	copy(b, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1}) // magic, ELFCLASS64, LE, version
	binary.LittleEndian.PutUint16(b[16:], 2)      // e_type = ET_EXEC
	binary.LittleEndian.PutUint16(b[18:], uint16(machine))
	binary.LittleEndian.PutUint32(b[20:], 1)  // e_version
	binary.LittleEndian.PutUint16(b[52:], 64) // e_ehsize
	return append(b, filler...)
}

// fakePE は debug/pe が解釈できる最小の PE ヘッダ＋filler を返す。
func fakePE(machine uint16, filler string) []byte {
	b := make([]byte, 0x40+4+20) // DOSヘッダ + "PE\0\0" + COFF FileHeader
	b[0], b[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(b[0x3c:], 0x40) // e_lfanew
	copy(b[0x40:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[0x44:], machine)
	return append(b, filler...)
}

type tarEntry struct {
	name string
	body []byte
	typ  byte   // 0 は TypeReg
	link string // TypeSymlink 用
}

func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.name, Mode: 0o755, Typeflag: typ, Linkname: e.link}
		if typ == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write(e.body); err != nil {
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

type zipEntry struct {
	name    string
	body    []byte
	symlink bool
}

func makeZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.symlink {
			hdr.SetMode(fs.ModeSymlink | 0o777)
		} else {
			hdr.SetMode(0o755)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeExe は path に内容 body の偽実行ファイルを置く。
func writeExe(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
}

// readFile は path の内容を返す（存在しなければ test 失敗）。
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// mustNotExist は path が存在しないことを検証する。
func mustNotExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("%s が残っています（err=%v）", filepath.Base(p), err)
	}
}
