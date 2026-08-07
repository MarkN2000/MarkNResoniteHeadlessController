package server

// QUIC グローバル設定の HTTP 層。
// Resonite の Headless/Config.json 全文は認証情報等を含み得るため公開せず、
// quicConfig.publicIP だけを読み書きする。未知項目は raw map のまま温存する。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/platform"
)

const headlessConfigFileName = "Config.json"

var errQUICConfigInvalid = errors.New("Headless/Config.json の形式が不正です")

type quicConfigResp struct {
	PublicIP string `json:"publicIP"`
}

func (s *Server) headlessConfigPath() (dir, path string) {
	s.cfgMu.RLock()
	installDir := s.cfg.InstallDirOrDefault(s.dataDir)
	s.cfgMu.RUnlock()
	dir = filepath.Join(platform.ExpandHome(installDir), "Headless")
	return dir, filepath.Join(dir, headlessConfigFileName)
}

func ensureExistingDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s はディレクトリではありません", dir)
	}
	return nil
}

func readHeadlessConfig(path string) (map[string]any, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, false, nil
		}
		return nil, false, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, true, fmt.Errorf("%w: %v", errQUICConfigInvalid, err)
	}
	if m == nil {
		return nil, true, fmt.Errorf("%w: ルートはオブジェクトである必要があります", errQUICConfigInvalid)
	}
	return m, true, nil
}

func quicObject(m map[string]any) (map[string]any, bool, error) {
	v, exists := m["quicConfig"]
	if !exists || v == nil {
		return nil, exists, nil
	}
	q, ok := v.(map[string]any)
	if !ok {
		return nil, true, fmt.Errorf("%w: quicConfig はオブジェクトである必要があります", errQUICConfigInvalid)
	}
	return q, true, nil
}

func publicIPFrom(m map[string]any) (string, error) {
	q, _, err := quicObject(m)
	if err != nil || q == nil {
		return "", err
	}
	v, exists := q["publicIP"]
	if !exists || v == nil {
		return "", nil
	}
	ip, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: quicConfig.publicIP は文字列である必要があります", errQUICConfigInvalid)
	}
	// Resonite が QUIC の公開 URI を組み立てる際、IPv6 は Config.json 上でも
	// [IPv6] の形でないとポートとの区切りを解釈できない。UI/API は通常の
	// IPリテラルを扱うため、保存済みの角括弧は読み取り時だけ外して返す。
	if len(ip) >= 2 && ip[0] == '[' && ip[len(ip)-1] == ']' {
		inner := ip[1 : len(ip)-1]
		if strings.Contains(inner, ":") && net.ParseIP(inner) != nil {
			return inner, nil
		}
	}
	return ip, nil
}

// normalizePublicIP は UI/API 用のIPリテラルと Config.json に保存する値を返す。
// IPv6だけはResoniteの公開URI生成に必要な角括弧を付ける。既に角括弧付きの
// IPv6がAPIへ渡された場合も一重に正規化する。
func normalizePublicIP(value string) (display, stored string, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", true
	}
	candidate := value
	if len(value) >= 2 && value[0] == '[' && value[len(value)-1] == ']' {
		candidate = value[1 : len(value)-1]
		if !strings.Contains(candidate, ":") {
			return "", "", false
		}
	}
	if net.ParseIP(candidate) == nil {
		return "", "", false
	}
	if strings.Contains(candidate, ":") {
		return candidate, "[" + candidate + "]", true
	}
	return candidate, candidate, true
}

// atomicWriteJSON は同一ディレクトリの一時ファイルを完全に書いてから置き換える。
// 既存ファイルがあればパーミッションを維持し、新規は秘密を含み得るため 0600 とする。
func atomicWriteJSON(path string, m map[string]any) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return statErr
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".quic-config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	keep = true
	return nil
}

func writeQUICConfigError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		writeErr(w, http.StatusConflict, "headless_not_installed", "Resoniteヘッドレスがインストールされていません")
	case errors.Is(err, errQUICConfigInvalid):
		writeErr(w, http.StatusConflict, "quic_config_invalid", err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "quic_config_failed", err.Error())
	}
}

func (s *Server) handleQUICConfigGet(w http.ResponseWriter, r *http.Request) {
	dir, path := s.headlessConfigPath()
	s.quicConfigMu.Lock()
	defer s.quicConfigMu.Unlock()
	if err := ensureExistingDir(dir); err != nil {
		writeQUICConfigError(w, err)
		return
	}
	m, _, err := readHeadlessConfig(path)
	if err != nil {
		writeQUICConfigError(w, err)
		return
	}
	publicIP, err := publicIPFrom(m)
	if err != nil {
		writeQUICConfigError(w, err)
		return
	}
	writeOK(w, quicConfigResp{PublicIP: publicIP})
}

func (s *Server) handleQUICConfigPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PublicIP *string `json:"publicIP"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PublicIP == nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "不正なリクエスト")
		return
	}
	publicIP, storedPublicIP, valid := normalizePublicIP(*body.PublicIP)
	if !valid {
		writeErr(w, http.StatusBadRequest, "invalid_public_ip", "IPアドレスの形式が不正です")
		return
	}

	dir, path := s.headlessConfigPath()
	s.quicConfigMu.Lock()
	defer s.quicConfigMu.Unlock()
	if err := ensureExistingDir(dir); err != nil {
		writeQUICConfigError(w, err)
		return
	}
	m, existed, err := readHeadlessConfig(path)
	if err != nil {
		writeQUICConfigError(w, err)
		return
	}
	q, qExists, err := quicObject(m)
	if err != nil {
		writeQUICConfigError(w, err)
		return
	}
	if q != nil {
		if current, exists := q["publicIP"]; exists && current != nil {
			if _, ok := current.(string); !ok {
				writeQUICConfigError(w, fmt.Errorf("%w: quicConfig.publicIP は文字列である必要があります", errQUICConfigInvalid))
				return
			}
		}
	}

	if publicIP == "" {
		if q != nil {
			delete(q, "publicIP")
			if len(q) == 0 {
				delete(m, "quicConfig")
			}
		} else if qExists {
			delete(m, "quicConfig")
		}
		if !existed {
			writeOK(w, quicConfigResp{})
			return
		}
	} else {
		if q == nil {
			q = map[string]any{}
			m["quicConfig"] = q
		}
		q["publicIP"] = storedPublicIP
	}

	if err := atomicWriteJSON(path, m); err != nil {
		writeQUICConfigError(w, err)
		return
	}
	writeOK(w, quicConfigResp{PublicIP: publicIP})
}
