package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/mods"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

var errModBusy = errors.New("ヘッドレスを停止し、進行中の起動・更新処理が完了してから操作してください")

func writeModErr(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "mod_storage_failed"
	switch {
	case errors.Is(err, errModBusy):
		status, code = http.StatusConflict, "mod_busy"
	case errors.Is(err, mods.ErrConflict):
		status, code = http.StatusConflict, "mod_conflict"
	case errors.Is(err, mods.ErrInvalid):
		status, code = http.StatusBadRequest, "mod_invalid"
	case errors.Is(err, mods.ErrNotFound):
		status, code = http.StatusNotFound, "mod_not_found"
	case errors.Is(err, mods.ErrDownload):
		status, code = http.StatusBadGateway, "mod_download_failed"
	}
	writeErr(w, status, code, err.Error())
}

func modBody(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return mods.ErrInvalid
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return mods.ErrInvalid
	}
	return nil
}

func (s *Server) modDir() string {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return filepath.Dir(platform.ExpandHome(s.cfg.HeadlessPathOrDefault(s.dataDir, platform.HeadlessBinaryName())))
}

func (s *Server) modRead() (mods.State, error) {
	store, err := mods.Open(s.modDir())
	if errors.Is(err, os.ErrNotExist) {
		return mods.State{Entries: []mods.Entry{}}, nil
	}
	if err != nil {
		return mods.State{}, err
	}
	defer store.Close()
	return store.Load()
}

func (s *Server) modBlocked() bool {
	return s.driver.Status().State != headless.StateStopped || s.restart.snapshot().inProgress || s.steamRunning()
}

func (s *Server) handleMods(w http.ResponseWriter, r *http.Request) {
	st, err := s.modRead()
	if err != nil {
		writeModErr(w, err)
		return
	}
	busy := !s.operationMu.TryLock()
	if !busy {
		s.operationMu.Unlock()
	}
	writeOK(w, map[string]any{"entries": st.Entries, "busy": busy, "canModify": !busy && !s.modBlocked()})
}

// mutateMods は受信から配置完了まで排他する。進行中の起動準備もMod変更を拒否する。
func (s *Server) mutateMods(w http.ResponseWriter, r *http.Request, change func(*mods.Store, *mods.State) error) {
	if !s.operationMu.TryLock() {
		writeModErr(w, errModBusy)
		return
	}
	defer s.operationMu.Unlock()
	if s.modBlocked() {
		writeModErr(w, errModBusy)
		return
	}
	store, err := mods.Open(s.modDir())
	if err != nil {
		writeModErr(w, err)
		return
	}
	defer store.Close()
	if err = store.Recover(); err != nil {
		writeModErr(w, err)
		return
	}
	old, err := store.Load()
	if err != nil {
		writeModErr(w, err)
		return
	}
	// 別々に読んでスライス・前版のポインタまで独立させる。
	next, err := store.Load()
	if err != nil {
		writeModErr(w, err)
		return
	}
	if err = change(store, &next); err == nil {
		err = store.Commit(old, next)
	}
	if err != nil {
		store.Cleanup()
		writeModErr(w, err)
		return
	}
	writeOK(w, next)
}

func newModID() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}

func replaceMod(st *mods.State, id string, version mods.Version) error {
	if id == "" || (id == "rml" && st.Find(id) == nil) {
		if id == "" {
			var err error
			id, err = newModID()
			if err != nil {
				return err
			}
		}
		st.Entries = append(st.Entries, mods.Entry{ID: id, Enabled: true, Current: version, UpdateSource: version.Source})
		return nil
	}
	e := st.Find(id)
	if e == nil {
		return mods.ErrNotFound
	}
	previous := e.Current
	e.Previous = &previous
	e.PreviousUpdateSource = e.UpdateSource
	e.Current = version
	// 手動差し替えでも登録済みのGitHub更新元を保持する。導入版だけ不明になる。
	if version.Source != nil {
		e.UpdateSource = version.Source
	}
	return nil
}

func (s *Server) handleModUpload(w http.ResponseWriter, r *http.Request) {
	s.mutateMods(w, r, func(store *mods.Store, st *mods.State) error {
		id, name := r.URL.Query().Get("id"), r.URL.Query().Get("name")
		if id == "rml" {
			return mods.ErrInvalid
		}
		if id != "" && st.Find(id) == nil {
			return mods.ErrNotFound
		}
		r.Body = http.MaxBytesReader(w, r.Body, mods.MaxDLL)
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return fmt.Errorf("%w: DLLを受信できません（上限32 MiB）", mods.ErrInvalid)
		}
		f, err := store.PutDLL(name, b)
		if err != nil {
			return err
		}
		return replaceMod(st, id, mods.Version{Files: []mods.File{f}, AddedAt: time.Now().UTC()})
	})
}

func (s *Server) handleModReleases(w http.ResponseWriter, r *http.Request) {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		var err error
		page, err = strconv.Atoi(p)
		if err != nil {
			writeModErr(w, mods.ErrInvalid)
			return
		}
	}
	includePrereleases := false
	if value := r.URL.Query().Get("includePrereleases"); value != "" {
		var err error
		includePrereleases, err = strconv.ParseBool(value)
		if err != nil {
			writeModErr(w, mods.ErrInvalid)
			return
		}
	}
	out, err := s.modGitHub.List(r.Context(), r.URL.Query().Get("repo"), page, includePrereleases)
	if err != nil {
		writeModErr(w, err)
		return
	}
	writeOK(w, out)
}

func (s *Server) handleModCheck(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := modBody(w, r, &body); err != nil {
		writeModErr(w, err)
		return
	}
	st, err := s.modRead()
	if err != nil {
		writeModErr(w, err)
		return
	}
	e := st.Find(body.ID)
	if e == nil && body.ID != "rml" {
		writeModErr(w, mods.ErrNotFound)
		return
	}
	repo, asset, tag := mods.OfficialRepo, "ResoniteModLoader.dll", ""
	if body.ID != "rml" {
		if e.UpdateSource == nil {
			writeModErr(w, mods.ErrInvalid)
			return
		}
		repo, asset = e.UpdateSource.Repo, e.UpdateSource.Asset
	}
	if e != nil && e.Current.Source != nil && strings.EqualFold(e.Current.Source.Repo, repo) && e.Current.Source.Asset == asset {
		tag = e.Current.Source.Tag
	}
	release, err := s.modGitHub.Release(r.Context(), repo, "", false)
	if err != nil {
		writeModErr(w, err)
		return
	}
	if _, err = release.Asset(asset); err != nil {
		asset = ""
	}
	if body.ID == "rml" {
		if _, err = release.Asset("0Harmony.dll"); err != nil {
			writeModErr(w, err)
			return
		}
	}
	writeOK(w, map[string]any{"repo": repo, "tag": release.Tag, "asset": asset, "updateAvailable": tag != "" && tag != release.Tag, "versionUnknown": e != nil && tag == "", "installed": e != nil})
}

func (s *Server) handleModGitHub(w http.ResponseWriter, r *http.Request) {
	s.mutateMods(w, r, func(store *mods.Store, st *mods.State) error {
		var body struct {
			ID                 string `json:"id"`
			Repo               string `json:"repo"`
			Tag                string `json:"tag"`
			Asset              string `json:"asset"`
			IncludePrereleases bool   `json:"includePrereleases"`
		}
		if err := modBody(w, r, &body); err != nil {
			return err
		}
		if body.ID != "" && body.ID != "rml" && st.Find(body.ID) == nil {
			return mods.ErrNotFound
		}
		if body.ID == "rml" {
			body.Repo = mods.OfficialRepo
			body.Asset = "ResoniteModLoader.dll"
		}
		repo, err := mods.Repository(body.Repo)
		if err != nil {
			return err
		}
		if body.Tag == "" {
			return mods.ErrInvalid
		}
		release, err := s.modGitHub.Release(r.Context(), repo, body.Tag, body.IncludePrereleases)
		if err != nil {
			return err
		}
		names := []string{body.Asset}
		if body.ID == "rml" {
			names = append(names, "0Harmony.dll")
		}
		version := mods.Version{AddedAt: time.Now().UTC(), Source: &mods.Source{Repo: repo, Tag: release.Tag, Asset: body.Asset}}
		for _, name := range names {
			b, err := s.modGitHub.Download(r.Context(), repo, release, name)
			if err != nil {
				return err
			}
			f, err := store.PutDLL(name, b)
			if err != nil {
				return err
			}
			version.Files = append(version.Files, f)
		}
		return replaceMod(st, body.ID, version)
	})
}

func (s *Server) handleModPatch(w http.ResponseWriter, r *http.Request) {
	s.mutateMods(w, r, func(store *mods.Store, st *mods.State) error {
		var body struct {
			Enabled            *bool  `json:"enabled"`
			Repo               string `json:"repo"`
			Tag                string `json:"tag"`
			Asset              string `json:"asset"`
			IncludePrereleases bool   `json:"includePrereleases"`
		}
		if err := modBody(w, r, &body); err != nil {
			return err
		}
		e := st.Find(r.PathValue("id"))
		if e == nil {
			return mods.ErrNotFound
		}
		if body.Enabled != nil {
			if body.Repo != "" || body.Tag != "" || body.Asset != "" || body.IncludePrereleases {
				return mods.ErrInvalid
			}
			e.Enabled = *body.Enabled
			return nil
		}
		if e.ID == "rml" || body.Tag == "" {
			return mods.ErrInvalid
		}
		repo, err := mods.Repository(body.Repo)
		if err != nil {
			return err
		}
		release, err := s.modGitHub.Release(r.Context(), repo, body.Tag, body.IncludePrereleases)
		if err != nil {
			return err
		}
		if _, err = release.Asset(body.Asset); err != nil {
			return err
		}
		e.UpdateSource = &mods.Source{Repo: repo, Asset: body.Asset}
		return nil
	})
}

func (s *Server) handleModRestore(w http.ResponseWriter, r *http.Request) {
	s.mutateMods(w, r, func(store *mods.Store, st *mods.State) error {
		e := st.Find(r.PathValue("id"))
		if e == nil {
			return mods.ErrNotFound
		}
		if e.Previous == nil {
			return mods.ErrInvalid
		}
		e.Current = *e.Previous
		e.Previous = nil
		e.UpdateSource = e.PreviousUpdateSource
		e.PreviousUpdateSource = nil
		return nil
	})
}

func (s *Server) handleModDelete(w http.ResponseWriter, r *http.Request) {
	s.mutateMods(w, r, func(store *mods.Store, st *mods.State) error {
		id := r.PathValue("id")
		if id == "rml" {
			return mods.ErrInvalid
		}
		for i, e := range st.Entries {
			if e.ID == id {
				st.Entries = append(st.Entries[:i], st.Entries[i+1:]...)
				return nil
			}
		}
		return mods.ErrNotFound
	})
}

func (s *Server) startHeadless(headlessPath, launchPath, name string) error {
	if !s.operationMu.TryLock() {
		return errModBusy
	}
	defer s.operationMu.Unlock()
	return s.startHeadlessLocked(headlessPath, launchPath, name)
}

// 全起動経路がここを通り、ジャーナル復旧とRML起動指定を共用する。
func (s *Server) startHeadlessLocked(headlessPath, launchPath, name string) error {
	if s.steamRunning() {
		return errModBusy
	}
	if s.driver.Status().State != headless.StateStopped {
		return headless.ErrAlreadyRunning
	}
	store, err := mods.Open(filepath.Dir(headlessPath))
	if err != nil {
		return err
	}
	defer store.Close()
	args, err := store.LaunchArgs()
	if err != nil {
		s.driver.PublishSys("Mod構成を確認できないため起動を中断しました: " + err.Error())
		return err
	}
	return s.driver.StartWithArgs(headlessPath, launchPath, name, args...)
}
