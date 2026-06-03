package server

// favorites.go はワールドお気に入りの永続化（favorites.json）と HTTP CRUD。
// runtime-state と同方式（別ファイル・favMu 直列化・read-modify-write・0600・jsonstate.go 共有）。
// 同一判定キーは recordId（R-GUID は一意・パス安全）。フロント WorldResult と同一フィールド。

import (
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxFavorites       = 100 // 1ユーザーのお気に入り上限（ファイル肥大の安全網）
	maxFavoriteNameLen = 256 // 名前の長さ上限（ユーザー生成・念のためのガード）
)

// errFavoritesFull は上限到達時に addFavorite が返す（HTTP 層で 400 にマップ）。
var errFavoritesFull = errors.New("お気に入りは最大 100 件です")

// favoriteResoniteURLRe は保存を許す resrec URL の形（worlds.go の charset と一致）。
// startWorldURL にそのまま渡すため、ここで形を縛って不正値の保存を防ぐ。
var favoriteResoniteURLRe = regexp.MustCompile(`^resrec:///(?:U|G)-[A-Za-z0-9_-]+/R-[A-Za-z0-9_-]+$`)

// favoriteWorld は1件のお気に入り（フロント WorldResult と同一フィールド）。
type favoriteWorld struct {
	Name         string `json:"name"`
	OwnerID      string `json:"ownerId"`
	RecordID     string `json:"recordId"`
	ResoniteURL  string `json:"resoniteUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
}

type favoritesState struct {
	Worlds []favoriteWorld `json:"worlds"`
}

func (s *Server) favoritesPath() string {
	return filepath.Join(s.dataDir, "favorites.json")
}

// loadFavoritesLocked / saveFavoritesLocked は favMu 保持前提の素の read/write。
func (s *Server) loadFavoritesLocked() favoritesState {
	if s.dataDir == "" {
		return favoritesState{Worlds: []favoriteWorld{}}
	}
	st := readJSONFile[favoritesState](s.favoritesPath())
	if st.Worlds == nil {
		st.Worlds = []favoriteWorld{}
	}
	return st
}

func (s *Server) saveFavoritesLocked(st favoritesState) {
	if s.dataDir == "" {
		return
	}
	writeJSONFile(s.favoritesPath(), st)
}

// --- 状態変更（HTTP から独立・単体テスト可能。検証は HTTP 層が担う） ---

func (s *Server) listFavorites() []favoriteWorld {
	s.favMu.Lock()
	defer s.favMu.Unlock()
	return s.loadFavoritesLocked().Worlds
}

// addFavorite は f を追加し更新後一覧を返す。recordId 既存なら冪等（無変更）。上限超過は errFavoritesFull。
func (s *Server) addFavorite(f favoriteWorld) ([]favoriteWorld, error) {
	s.favMu.Lock()
	defer s.favMu.Unlock()
	st := s.loadFavoritesLocked()
	for _, e := range st.Worlds {
		if e.RecordID == f.RecordID {
			return st.Worlds, nil // 既存＝冪等
		}
	}
	if len(st.Worlds) >= maxFavorites {
		return st.Worlds, errFavoritesFull
	}
	st.Worlds = append(st.Worlds, f)
	s.saveFavoritesLocked(st)
	return st.Worlds, nil
}

// removeFavorite は recordId 一致を取り除き更新後一覧を返す（無ければ無変更）。
func (s *Server) removeFavorite(recordID string) []favoriteWorld {
	s.favMu.Lock()
	defer s.favMu.Unlock()
	st := s.loadFavoritesLocked()
	out := make([]favoriteWorld, 0, len(st.Worlds))
	for _, e := range st.Worlds {
		if e.RecordID != recordID {
			out = append(out, e)
		}
	}
	if len(out) != len(st.Worlds) {
		st.Worlds = out
		s.saveFavoritesLocked(st)
	}
	return out
}

// --- HTTP CRUD（いずれも更新後一覧を data で返す） ---

// handleFavoritesList: GET /api/v1/favorites → []favoriteWorld（追加順）
func (s *Server) handleFavoritesList(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, s.listFavorites())
}

// handleFavoriteAdd: POST /api/v1/favorites（body=World）→ 追加（冪等）→ 更新後一覧
func (s *Server) handleFavoriteAdd(w http.ResponseWriter, r *http.Request) {
	var body favoriteWorld
	if !decodeBody(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.ResoniteURL = strings.TrimSpace(body.ResoniteURL)
	body.ThumbnailURL = strings.TrimSpace(body.ThumbnailURL)
	if !favoriteResoniteURLRe.MatchString(body.ResoniteURL) {
		writeErr(w, http.StatusBadRequest, "invalid_value", "resoniteUrl の形式が不正です")
		return
	}
	if body.OwnerID == "" || body.RecordID == "" {
		writeErr(w, http.StatusBadRequest, "invalid_value", "ownerId / recordId は必須です")
		return
	}
	if len([]rune(body.Name)) > maxFavoriteNameLen {
		writeErr(w, http.StatusBadRequest, "invalid_value", "name が長すぎます")
		return
	}
	if body.ThumbnailURL != "" && !strings.HasPrefix(body.ThumbnailURL, "https://") {
		writeErr(w, http.StatusBadRequest, "invalid_value", "thumbnailUrl は https のみ許可されます")
		return
	}
	worlds, err := s.addFavorite(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "limit_exceeded", err.Error())
		return
	}
	writeOK(w, worlds)
}

// handleFavoriteRemove: DELETE /api/v1/favorites/{recordId} → 削除 → 更新後一覧
func (s *Server) handleFavoriteRemove(w http.ResponseWriter, r *http.Request) {
	writeOK(w, s.removeFavorite(r.PathValue("recordId")))
}
