package mods

import (
	"bytes"
	"context"
	"debug/pe"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 実行コードを含まない最小のPE/CLI。ゲーム由来のDLLをテストへ持ち込まない。
func testDLL(marker byte) []byte {
	b := make([]byte, 1024)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[60:], 128)
	copy(b[128:], "PE\x00\x00")
	var h bytes.Buffer
	_ = binary.Write(&h, binary.LittleEndian, pe.FileHeader{Machine: pe.IMAGE_FILE_MACHINE_I386, NumberOfSections: 1, SizeOfOptionalHeader: 224, Characteristics: pe.IMAGE_FILE_DLL})
	opt := pe.OptionalHeader32{Magic: 0x10b, NumberOfRvaAndSizes: 16}
	opt.DataDirectory[14] = pe.DataDirectory{VirtualAddress: 0x2000, Size: 72}
	_ = binary.Write(&h, binary.LittleEndian, opt)
	_ = binary.Write(&h, binary.LittleEndian, pe.SectionHeader32{Name: [8]byte{'.', 't', 'e', 'x', 't'}, VirtualAddress: 0x2000, VirtualSize: 512, SizeOfRawData: 512, PointerToRawData: 512})
	copy(b[132:], h.Bytes())
	binary.LittleEndian.PutUint32(b[512:], 72)
	binary.LittleEndian.PutUint32(b[520:], 0x2050)
	binary.LittleEndian.PutUint32(b[524:], 16)
	copy(b[592:], "BSJB")
	b[700] = marker
	return b
}

func mustStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}
func load(t *testing.T, s *Store) State {
	t.Helper()
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	return st
}
func put(t *testing.T, s *Store, name string, n byte) File {
	t.Helper()
	f, err := s.PutDLL(name, testDLL(n))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func commit(t *testing.T, s *Store, old, next State) {
	t.Helper()
	if err := s.Commit(old, next); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleAndRecovery(t *testing.T) {
	s, dir := mustStore(t)
	first := put(t, s, "Example.dll", 1)
	st := State{Entries: []Entry{{ID: strings.Repeat("a", 32), Enabled: true, Current: Version{Files: []File{first}}}}}
	commit(t, s, load(t, s), st)
	if b, err := os.ReadFile(filepath.Join(dir, "rml_mods", "Example.dll")); err != nil || !bytes.Equal(b, testDLL(1)) {
		t.Fatal("配置失敗", err)
	}
	old := load(t, s)
	next := load(t, s)
	next.Entries[0].Enabled = false
	commit(t, s, old, next)
	if _, err := os.Stat(filepath.Join(dir, "rml_mods", "Example.dll")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("無効DLLが読込先に残る")
	}
	old = load(t, s)
	next = load(t, s)
	next.Entries[0].Enabled = true
	commit(t, s, old, next)
	old = load(t, s)
	next = load(t, s)
	previous := next.Entries[0].Current
	next.Entries[0].Previous = &previous
	next.Entries[0].Current = Version{Files: []File{put(t, s, "Example.dll", 2)}}
	commit(t, s, old, next)
	if len(next.Entries[0].Previous.Files) != 1 {
		t.Fatal("前版なし")
	}
	old = load(t, s)
	next = load(t, s)
	next.Entries[0].Current = *next.Entries[0].Previous
	next.Entries[0].Previous = nil
	commit(t, s, old, next)
	if b, _ := os.ReadFile(filepath.Join(dir, "rml_mods", "Example.dll")); !bytes.Equal(b, testDLL(1)) {
		t.Fatal("復元失敗")
	}
	// DLL差し替え後・state保存前の強制終了を再現。
	old = load(t, s)
	next = load(t, s)
	next.Entries[0].Current = Version{Files: []File{put(t, s, "Example.dll", 3)}}
	if err := s.writeJSON(storeDir+"/journal.json", journal{old, next}); err != nil {
		t.Fatal(err)
	}
	if err := s.applyFiles(deployed(old), deployed(next)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LaunchArgs(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "rml_mods", "Example.dll")); !bytes.Equal(b, testDLL(1)) {
		t.Fatal("中断復旧失敗")
	}
	// 設定は削除対象に含めない。
	if err := s.write("rml_config/Example.json", []byte("config")); err != nil {
		t.Fatal(err)
	}
	commit(t, s, load(t, s), State{Entries: []Entry{}})
	if _, err := os.Stat(filepath.Join(dir, "rml_config", "Example.json")); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(filepath.Join(dir, storeDir, "blobs"))
	if len(files) != 0 {
		t.Fatal("削除したDLL原本が残る")
	}
}

func TestRMLPairAndGlobalSwitch(t *testing.T) {
	s, dir := mustStore(t)
	rml := put(t, s, "ResoniteModLoader.dll", 1)
	harmony := put(t, s, "0Harmony.dll", 2)
	st := State{Entries: []Entry{{ID: "rml", Enabled: true, Current: Version{Files: []File{rml, harmony}}}, {ID: strings.Repeat("b", 32), Enabled: true, Current: Version{Files: []File{put(t, s, "Mod.dll", 3)}}}}}
	commit(t, s, load(t, s), st)
	args, err := s.LaunchArgs()
	if err != nil || len(args) != 2 || args[0] != "-LoadAssembly" {
		t.Fatal(args, err)
	}
	old, next := load(t, s), load(t, s)
	next.Find("rml").Enabled = false
	opened, err := os.Open(filepath.Join(dir, "Libraries", "ResoniteModLoader.dll"))
	if err != nil {
		t.Fatal(err)
	}
	commit(t, s, old, next)
	_ = opened.Close()
	args, err = s.LaunchArgs()
	if err != nil || len(args) != 0 || !load(t, s).Entries[1].Enabled {
		t.Fatal("全体OFFで個別状態が変化", args, err)
	}
	old, next = load(t, s), load(t, s)
	e := next.Find("rml")
	v := e.Current
	e.Previous = &v
	e.Current = Version{Files: []File{put(t, s, "ResoniteModLoader.dll", 4), put(t, s, "0Harmony.dll", 5)}}
	commit(t, s, old, next)
	old, next = load(t, s), load(t, s)
	e = next.Find("rml")
	e.Current = *e.Previous
	e.Previous = nil
	commit(t, s, old, next)
	for p, hash := range deployed(st) {
		b, err := s.read(p)
		if err != nil || digest(b) != hash {
			t.Fatal("組で復元できない", p, err)
		}
	}
}

func TestUnmanagedAndDamagedFilesArePreserved(t *testing.T) {
	s, _ := mustStore(t)
	if err := s.write("rml_mods/Other.dll", []byte("manual")); err != nil {
		t.Fatal(err)
	}
	st := State{Entries: []Entry{{ID: strings.Repeat("c", 32), Enabled: true, Current: Version{Files: []File{put(t, s, "Other.dll", 1)}}}}}
	if err := s.Commit(load(t, s), st); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if b, _ := s.read("rml_mods/Other.dll"); string(b) != "manual" {
		t.Fatal("管理外ファイルを上書きした")
	}
	st.Entries[0].Current.Files[0].Name = "Managed.dll"
	commit(t, s, load(t, s), st)
	if err := s.write("rml_mods/Managed.dll", []byte("external edit")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LaunchArgs(); !errors.Is(err, ErrConflict) {
		t.Fatal("不整合で起動可能", err)
	}
	if err := s.Commit(load(t, s), State{}); !errors.Is(err, ErrConflict) {
		t.Fatal("変更済DLLを削除した", err)
	}
	if err := s.write(storeDir+"/state.json", []byte("broken")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("破損stateを空扱いした")
	}
}

func TestValidation(t *testing.T) {
	for _, name := range []string{"../a.dll", `C:\a.dll`, "a/b.dll", "a.dll:evil", "NUL.dll", "a.exe", ".dll", "a\n.dll"} {
		if ValidateName(name) == nil {
			t.Errorf("受理した: %q", name)
		}
	}
	for _, b := range [][]byte{nil, []byte("MZ"), make([]byte, MaxDLL+1)} {
		if ValidateDLL(b) == nil {
			t.Fatal("不正DLLを受理した")
		}
	}
	if err := ValidateDLL(testDLL(1)); err != nil {
		t.Fatal(err)
	}
	s, dir := mustStore(t)
	if f := put(t, s, "Upper.DLL", 1); f.Name != "Upper.dll" {
		t.Fatal("Linuxで読めない拡張子", f.Name)
	}
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(dir, "rml_mods")); err != nil {
		t.Skip("リンク作成権限なし")
	}
	st := State{Entries: []Entry{{ID: strings.Repeat("d", 32), Enabled: true, Current: Version{Files: []File{put(t, s, "Escape.dll", 1)}}}}}
	if err := s.Commit(load(t, s), st); err == nil {
		t.Fatal("リンクを受理した")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGitHubSelectionAndDownloadValidation(t *testing.T) {
	for _, input := range []string{"http://github.com/a/b", "https://github.com.evil/a/b", "https://github.com/a/b?x=1", "../b", "a/..", "https://user@github.com/a/b"} {
		if _, err := Repository(input); err == nil {
			t.Fatal(input)
		}
	}
	if r, err := Repository("https://github.com/owner/repo.git"); err != nil || r != "owner/repo" {
		t.Fatal(r, err)
	}
	dll := testDLL(8)
	release := Release{Tag: "v2", Assets: []Asset{{Name: "Mod.dll", Size: int64(len(dll)), URL: "https://github.com/owner/repo/releases/download/v2/Mod.dll"}, {Name: "Other.dll", Size: 20}}}
	var downloads int
	g := GitHub{Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var b []byte
		if r.URL.Host == "api.github.com" {
			b, _ = json.Marshal(release)
		} else {
			downloads++
			b = dll
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
	})}}
	r, err := g.Release(context.Background(), "owner/repo", "", false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := g.Download(context.Background(), "owner/repo", r, "Mod.dll")
	if err != nil || !bytes.Equal(b, dll) || downloads != 1 {
		t.Fatal(downloads, err)
	}
	r.Assets = append([]Asset(nil), r.Assets...)
	r.Assets[0].URL = "https://localhost/Mod.dll"
	if _, err = g.Download(context.Background(), "owner/repo", r, "Mod.dll"); err == nil || downloads != 1 {
		t.Fatal("任意URLを取得した")
	}
	r = release
	r.Assets = append([]Asset(nil), release.Assets...)
	r.Assets[0].Size++
	if _, err = g.Download(context.Background(), "owner/repo", r, "Mod.dll"); err == nil {
		t.Fatal("途中切断を受理した")
	}
}

func TestGitHubPrereleases(t *testing.T) {
	ctx := context.Background()
	stable := Release{Tag: "v1", Assets: []Asset{{Name: "Mod.dll"}, {Name: "readme.txt"}}}
	pre := Release{Tag: "v2-rc.1", Prerelease: true, Assets: stable.Assets}
	draft := Release{Tag: "v3", Draft: true, Assets: stable.Assets}
	latestCalls := 0
	onlyPrereleases := false
	g := GitHub{Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var data any
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			latestCalls++
			data = stable
		case "/repos/owner/repo/releases/tags/v2-rc.1":
			data = pre
		case "/repos/owner/repo/releases/tags/v3":
			data = draft
		default:
			data = []Release{pre, draft, stable, {}}
			if onlyPrereleases {
				data = []Release{pre, draft}
			}
		}
		b, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
	})}}
	for _, include := range []bool{false, true} {
		out, err := g.List(ctx, "owner/repo", 1, include)
		want, first := 1, "v1"
		if include {
			want, first = 2, "v2-rc.1"
		}
		if err != nil || len(out.Releases) != want || out.Releases[0].Tag != first || out.HasMore {
			t.Fatalf("include=%v: %+v, %v", include, out, err)
		}
		for _, r := range out.Releases {
			if len(r.Assets) != 1 || r.Assets[0].Name != "Mod.dll" {
				t.Fatalf("DLL以外が一覧に含まれる: %+v", r)
			}
		}
		if _, err := g.Release(ctx, "owner/repo", pre.Tag, include); (err == nil) != include {
			t.Fatalf("プレリリースの取得 include=%v: %v", include, err)
		}
		if _, err := g.Release(ctx, "owner/repo", draft.Tag, include); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ドラフトを受理した: %v", err)
		}
	}
	if latestCalls != 1 {
		t.Fatalf("プレリリースを含める一覧でlatestへ問い合わせた: %d", latestCalls)
	}
	onlyPrereleases = true
	for _, page := range []int{1, 2} {
		out, err := g.List(ctx, "owner/repo", page, true)
		if err != nil || len(out.Releases) != 1 || out.Releases[0].Tag != pre.Tag || latestCalls != 1 {
			t.Fatalf("正式版なし page=%d: %+v, %v", page, out, err)
		}
	}
}
