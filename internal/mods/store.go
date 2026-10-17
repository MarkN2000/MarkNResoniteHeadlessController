// Package mods はRMLとMod DLLの保存・配置を扱う。呼び出し側が起動・更新との排他を持つ。
package mods

import (
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

const MaxDLL = 32 << 20
const storeDir = ".mrhc-mods"
const OfficialRepo = "resonite-modding-group/ResoniteModLoader"

var (
	ErrInvalid    = errors.New("入力が不正です")
	ErrConflict   = errors.New("Modファイルの状態が競合しています")
	ErrNotFound   = errors.New("対象のModがありません")
	hashPattern   = regexp.MustCompile(`^[a-f0-9]{64}$`)
	idPattern     = regexp.MustCompile(`^[a-f0-9]{32}$`)
	devicePattern = regexp.MustCompile(`^(COM|LPT)[0-9¹²³]$`)
)

type Source struct {
	Repo  string `json:"repo"`
	Tag   string `json:"tag,omitempty"`
	Asset string `json:"asset"`
}

type File struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
}

type Version struct {
	Files   []File    `json:"files"`
	AddedAt time.Time `json:"addedAt"`
	Source  *Source   `json:"source,omitempty"`
}

type Entry struct {
	ID                   string   `json:"id"`
	Enabled              bool     `json:"enabled"`
	Current              Version  `json:"current"`
	Previous             *Version `json:"previous,omitempty"`
	UpdateSource         *Source  `json:"updateSource,omitempty"`
	PreviousUpdateSource *Source  `json:"previousUpdateSource,omitempty"`
}

type State struct {
	Entries []Entry `json:"entries"`
}

func (s *State) Find(id string) *Entry {
	for i := range s.Entries {
		if s.Entries[i].ID == id {
			return &s.Entries[i]
		}
	}
	return nil
}

func ValidateName(name string) error {
	if name == "" || len(name) > 180 || strings.TrimSpace(name) != name || strings.ContainsAny(name, `/\:<>"|?*`) || !strings.HasSuffix(strings.ToLower(name), ".dll") {
		return fmt.Errorf("%w: DLLファイル名を指定してください", ErrInvalid)
	}
	for _, c := range name {
		if c < 32 || c == 127 {
			return ErrInvalid
		}
	}
	stem := strings.ToUpper(strings.Split(name, ".")[0])
	if stem == "" || strings.TrimSpace(stem) != stem || stem == "CON" || stem == "CONIN$" || stem == "CONOUT$" || stem == "PRN" || stem == "AUX" || stem == "NUL" || devicePattern.MatchString(stem) {
		return ErrInvalid
	}
	return nil
}

// ValidateDLL はPEのCLIヘッダーを調べる。アセンブリをロード・実行しない。
func ValidateDLL(data []byte) error {
	if len(data) == 0 || len(data) > MaxDLL {
		return fmt.Errorf("%w: DLLは32 MiBまでです", ErrInvalid)
	}
	f, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: DLL形式ではありません", ErrInvalid)
	}
	defer f.Close()
	var cli pe.DataDirectory
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		cli = h.DataDirectory[14]
	case *pe.OptionalHeader64:
		cli = h.DataDirectory[14]
	default:
		return ErrInvalid
	}
	if f.Characteristics&pe.IMAGE_FILE_DLL == 0 || cli.VirtualAddress == 0 || cli.Size < 72 {
		return fmt.Errorf("%w: .NET DLLではありません", ErrInvalid)
	}
	// RVAをファイル内の範囲へ解決し、CLIヘッダーと.NETメタデータの署名を確認する。
	readRVA := func(rva, size uint32) []byte {
		for _, s := range f.Sections {
			if uint64(rva) >= uint64(s.VirtualAddress) && uint64(rva)+uint64(size) <= uint64(s.VirtualAddress)+uint64(s.Size) {
				start := uint64(s.Offset) + uint64(rva-s.VirtualAddress)
				if start+uint64(size) <= uint64(len(data)) {
					return data[start : start+uint64(size)]
				}
			}
		}
		return nil
	}
	if header := readRVA(cli.VirtualAddress, 72); len(header) == 72 && binary.LittleEndian.Uint32(header) >= 72 {
		rva, size := binary.LittleEndian.Uint32(header[8:]), binary.LittleEndian.Uint32(header[12:])
		if metadata := readRVA(rva, size); len(metadata) >= 16 && string(metadata[:4]) == "BSJB" {
			return nil
		}
	}
	return fmt.Errorf("%w: CLIヘッダーが不正です", ErrInvalid)
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type Store struct{ root *os.Root }

func Open(headlessDir string) (*Store, error) {
	r, err := os.OpenRoot(headlessDir)
	if err != nil {
		return nil, err
	}
	return &Store{root: r}, nil
}
func (s *Store) Close() error { return s.root.Close() }

// checkPath はルート内でもリンクを保存先として使わない。os.Rootがルート外への脱出も防ぐ。
func (s *Store) checkPath(name string) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		p := strings.Join(parts[:i+1], "/")
		info, err := s.root.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) {
			return fmt.Errorf("%w: %s", ErrConflict, p)
		}
	}
	return nil
}

func (s *Store) read(name string) ([]byte, error) {
	if err := s.checkPath(name); err != nil {
		return nil, err
	}
	f, err := s.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxDLL+1))
	if len(b) > MaxDLL {
		return nil, ErrInvalid
	}
	return b, err
}

func (s *Store) write(name string, data []byte) error {
	if err := s.checkPath(name); err != nil {
		return err
	}
	if err := s.checkPath(storeDir + "/tmp"); err != nil {
		return err
	}
	if err := s.root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	if err := s.root.MkdirAll(storeDir, 0700); err != nil {
		return err
	}
	// 呼び出し側の排他により一時ファイルは常に1つ。
	f, err := s.root.OpenFile(storeDir+"/tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = s.root.Rename(storeDir+"/tmp", name)
	}
	if err != nil {
		_ = s.root.Remove(storeDir + "/tmp")
	}
	return err
}

func (s *Store) writeJSON(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return s.write(name, b)
}

func (s *Store) Load() (State, error) {
	st := State{Entries: []Entry{}}
	b, err := s.read(storeDir + "/state.json")
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err = json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	if st.Entries == nil {
		st.Entries = []Entry{}
	}
	return st, validateState(st)
}

func validateState(st State) error {
	ids, names := map[string]bool{}, map[string]bool{}
	for _, e := range st.Entries {
		if (e.ID != "rml" && !idPattern.MatchString(e.ID)) || ids[e.ID] {
			return ErrInvalid
		}
		ids[e.ID] = true
		versions := []Version{e.Current}
		if e.Previous != nil {
			versions = append(versions, *e.Previous)
		}
		for _, v := range versions {
			if (e.ID == "rml" && (len(v.Files) != 2 || v.Files[0].Name != "ResoniteModLoader.dll" || v.Files[1].Name != "0Harmony.dll")) || (e.ID != "rml" && len(v.Files) != 1) {
				return ErrInvalid
			}
			for _, f := range v.Files {
				if ValidateName(f.Name) != nil || !hashPattern.MatchString(f.Hash) {
					return ErrInvalid
				}
			}
		}
		if e.ID != "rml" {
			n := strings.ToLower(e.Current.Files[0].Name)
			if names[n] {
				return fmt.Errorf("%w: 同名のModがあります", ErrConflict)
			}
			names[n] = true
		}
	}
	return nil
}

func deployed(st State) map[string]string {
	m := map[string]string{}
	for _, e := range st.Entries {
		if e.ID == "rml" {
			m["Libraries/ResoniteModLoader.dll"] = e.Current.Files[0].Hash
			m["rml_libs/0Harmony.dll"] = e.Current.Files[1].Hash
		} else if e.Enabled {
			m["rml_mods/"+e.Current.Files[0].Name] = e.Current.Files[0].Hash
		}
	}
	return m
}

func (s *Store) PutDLL(name string, b []byte) (File, error) {
	if err := ValidateName(name); err != nil {
		return File{}, err
	}
	if err := ValidateDLL(b); err != nil {
		return File{}, err
	}
	// RMLの*.dll列挙はLinuxで大文字の拡張子を拾わないため、配置名だけ正規化する。
	f := File{Name: name[:len(name)-4] + ".dll", Hash: digest(b)}
	name = storeDir + "/blobs/" + f.Hash + ".dll"
	if existing, err := s.read(name); err == nil {
		if digest(existing) != f.Hash {
			return File{}, ErrConflict
		}
		return f, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return File{}, err
	}
	return f, s.write(name, b)
}

func (s *Store) verifyBlobs(st State) error {
	for _, e := range st.Entries {
		vs := []Version{e.Current}
		if e.Previous != nil {
			vs = append(vs, *e.Previous)
		}
		for _, v := range vs {
			for _, f := range v.Files {
				b, err := s.read(storeDir + "/blobs/" + f.Hash + ".dll")
				if err != nil {
					return err
				}
				if digest(b) != f.Hash {
					return fmt.Errorf("%w: 保存DLLのハッシュ不一致", ErrConflict)
				}
			}
		}
	}
	return nil
}

// inspect は通常適用時には旧構成との一致、中断復旧時には旧/新いずれかとの一致を要求する。
func (s *Store) inspect(old, next map[string]string, recovery bool) error {
	paths := map[string]bool{}
	for p := range old {
		paths[p] = true
	}
	for p := range next {
		paths[p] = true
	}
	for p := range paths {
		if err := s.checkPath(p); err != nil {
			return err
		}
		// Linuxでも大文字小文字だけ異なるDLLの二重ロードを防ぐ。
		d, err := s.root.Open(path.Dir(p))
		if err == nil {
			names, readErr := d.Readdirnames(-1)
			_ = d.Close()
			if readErr != nil {
				return readErr
			}
			for _, n := range names {
				if strings.EqualFold(n, path.Base(p)) && n != path.Base(p) {
					return fmt.Errorf("%w: %s", ErrConflict, p)
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		b, err := s.read(p)
		if errors.Is(err, os.ErrNotExist) {
			if recovery || old[p] == "" {
				continue
			}
			return fmt.Errorf("%w: %s がありません", ErrConflict, p)
		}
		if err != nil {
			return err
		}
		h := digest(b)
		if h == old[p] || (recovery && h == next[p]) {
			continue
		}
		return fmt.Errorf("%w: %s は管理外または変更されています", ErrConflict, p)
	}
	return nil
}

func (s *Store) applyFiles(from, to map[string]string) error {
	for p, hash := range to {
		// ON/OFFや別Modの追加で、変化のないDLLを再配置しない。
		// アップロード元として開かれた既存DLLの置換はWindowsでは失敗する。
		if current, err := s.read(p); err == nil && digest(current) == hash {
			continue
		}
		b, err := s.read(storeDir + "/blobs/" + hash + ".dll")
		if err != nil {
			return err
		}
		if err = s.write(p, b); err != nil {
			return err
		}
	}
	for p := range from {
		if to[p] == "" {
			if err := s.root.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

type journal struct {
	Old  State `json:"old"`
	Next State `json:"next"`
}

func (s *Store) Recover() error {
	b, err := s.read(storeDir + "/journal.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var j journal
	if err = json.Unmarshal(b, &j); err != nil {
		return err
	}
	if err = validateState(j.Old); err != nil {
		return err
	}
	if err = validateState(j.Next); err != nil {
		return err
	}
	if err = s.verifyBlobs(j.Old); err != nil {
		return err
	}
	old, next := deployed(j.Old), deployed(j.Next)
	if err = s.inspect(old, next, true); err != nil {
		return err
	}
	if err = s.applyFiles(next, old); err != nil {
		return err
	}
	if err = s.writeJSON(storeDir+"/state.json", j.Old); err != nil {
		return err
	}
	return s.root.Remove(storeDir + "/journal.json")
}

func (s *Store) Commit(old, next State) error {
	if err := validateState(next); err != nil {
		return err
	}
	if err := s.verifyBlobs(next); err != nil {
		return err
	}
	before, after := deployed(old), deployed(next)
	if err := s.inspect(before, after, false); err != nil {
		return err
	}
	if err := s.writeJSON(storeDir+"/journal.json", journal{old, next}); err != nil {
		return err
	}
	err := s.applyFiles(before, after)
	if err == nil {
		err = s.writeJSON(storeDir+"/state.json", next)
	}
	if err == nil {
		err = s.root.Remove(storeDir + "/journal.json")
	}
	if err != nil {
		return errors.Join(err, s.Recover())
	}
	// コミット後の掃除失敗は適用失敗ではない。次の操作で再試行する。
	s.Cleanup()
	return nil
}

func (s *Store) Cleanup() {
	if _, err := s.read(storeDir + "/journal.json"); !errors.Is(err, os.ErrNotExist) {
		return
	}
	st, err := s.Load()
	if err != nil {
		return
	}
	if err = s.Collect(st); err != nil {
		log.Printf("[mods] 未参照DLLの削除に失敗: %v", err)
	}
}

// Collect は現在版と1世代前以外のDLLを除去する。
func (s *Store) Collect(st State) error {
	keep := map[string]bool{}
	for _, e := range st.Entries {
		vs := []Version{e.Current}
		if e.Previous != nil {
			vs = append(vs, *e.Previous)
		}
		for _, v := range vs {
			for _, f := range v.Files {
				keep[f.Hash+".dll"] = true
			}
		}
	}
	if err := s.checkPath(storeDir + "/blobs"); err != nil {
		return err
	}
	d, err := s.root.Open(storeDir + "/blobs")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	names, err := d.Readdirnames(-1)
	_ = d.Close()
	if err != nil {
		return err
	}
	for _, n := range names {
		if !keep[n] && strings.HasSuffix(n, ".dll") && hashPattern.MatchString(strings.TrimSuffix(n, ".dll")) {
			if err = s.root.Remove(storeDir + "/blobs/" + n); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) LaunchArgs() ([]string, error) {
	if err := s.Recover(); err != nil {
		return nil, err
	}
	st, err := s.Load()
	if err != nil {
		return nil, err
	}
	if err = s.inspect(deployed(st), deployed(st), false); err != nil {
		return nil, err
	}
	if e := st.Find("rml"); e != nil && e.Enabled {
		return []string{"-LoadAssembly", "Libraries/ResoniteModLoader.dll"}, nil
	}
	return nil, nil
}
