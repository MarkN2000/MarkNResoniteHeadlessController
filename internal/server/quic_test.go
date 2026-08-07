package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/config"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/headless"
	"github.com/MarkN2000/MarkNResoniteHeadlessController/internal/resonite"
)

func newQUICServer(t *testing.T, createHeadless bool) (*httptest.Server, string, string) {
	t.Helper()
	tmp := t.TempDir()
	installDir := filepath.Join(tmp, "resonite")
	headlessDir := filepath.Join(installDir, "Headless")
	if createHeadless {
		if err := os.MkdirAll(headlessDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	cfg := &config.Config{
		Version:           config.SchemaVersion,
		AdminPasswordHash: string(hash),
		SessionSecret:     "quic-test-secret",
		Steam:             &config.Steam{InstallDir: installDir},
	}
	cfgPath := filepath.Join(tmp, "mrhc.config.json")
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	srv := New(cfg, cfgPath, headless.NewDriver(nil), resonite.NewClient(), nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, testPassword, filepath.Join(headlessDir, headlessConfigFileName)
}

func TestQUICConfig_GetPutPreservesUnknownFields(t *testing.T) {
	ts, pw, configPath := newQUICServer(t, true)
	original := map[string]any{
		"loginCredential": "secret-user",
		"unknown":         map[string]any{"keep": true},
		"quicConfig":      map[string]any{"publicIP": "203.0.113.1", "future": "keep"},
	}
	b, _ := json.Marshal(original)
	if err := os.WriteFile(configPath, b, 0o600); err != nil {
		t.Fatal(err)
	}

	var got okEnv[map[string]any]
	if code := authGet(t, ts.URL+"/api/v1/quic-config", pw, &got); code != http.StatusOK {
		t.Fatalf("GET status=%d", code)
	}
	if got.Data["publicIP"] != "203.0.113.1" {
		t.Fatalf("GET publicIP=%q", got.Data["publicIP"])
	}
	if len(got.Data) != 1 {
		t.Fatalf("GET must not expose other Config.json fields: %#v", got.Data)
	}

	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", `{"publicIP":"2001:db8::1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	stored, _, err := readHeadlessConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored["loginCredential"] != "secret-user" {
		t.Fatalf("top-level secret lost: %#v", stored)
	}
	if stored["unknown"].(map[string]any)["keep"] != true {
		t.Fatalf("unknown top-level field lost: %#v", stored)
	}
	q := stored["quicConfig"].(map[string]any)
	if q["publicIP"] != "[2001:db8::1]" || q["future"] != "keep" {
		t.Fatalf("quicConfig was not merged: %#v", q)
	}

	var gotIPv6 okEnv[quicConfigResp]
	if code := authGet(t, ts.URL+"/api/v1/quic-config", pw, &gotIPv6); code != http.StatusOK {
		t.Fatalf("GET bracketed IPv6 status=%d", code)
	}
	if gotIPv6.Data.PublicIP != "2001:db8::1" {
		t.Fatalf("GET bracketed IPv6 publicIP=%q", gotIPv6.Data.PublicIP)
	}
}

func TestQUICConfig_ClearAndValidation(t *testing.T) {
	ts, pw, configPath := newQUICServer(t, true)
	if err := os.WriteFile(configPath, []byte(`{"comment":"keep","quicConfig":{"publicIP":"203.0.113.1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"not-an-ip", "[203.0.113.1]", "[2001:db8::1"} {
		resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", `{"publicIP":"`+value+`"}`)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid IP %q status=%d", value, resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", `{"publicIP":"[2001:db8::2]"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bracketed IPv6 status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	stored, _, err := readHeadlessConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored["quicConfig"].(map[string]any)["publicIP"]; got != "[2001:db8::2]" {
		t.Fatalf("bracketed IPv6 was not normalized: %q", got)
	}
	for _, body := range []string{`null`, `{}`, `{"publicIP":null}`} {
		resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("missing publicIP body=%s status=%d", body, resp.StatusCode)
		}
		resp.Body.Close()
	}

	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", `{"publicIP":""}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	stored, _, err = readHeadlessConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored["comment"] != "keep" {
		t.Fatalf("other field lost: %#v", stored)
	}
	if _, exists := stored["quicConfig"]; exists {
		t.Fatalf("empty quicConfig should be removed: %#v", stored)
	}

	if err := os.WriteFile(configPath, []byte(`{"quicConfig":{"publicIP":"203.0.113.1","future":"keep"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", `{"publicIP":""}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear with unknown field status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	stored, _, err = readHeadlessConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if q := stored["quicConfig"].(map[string]any); q["future"] != "keep" {
		t.Fatalf("unknown quicConfig field lost while clearing publicIP: %#v", q)
	}
}

func TestQUICConfig_MissingFileBlankDoesNotCreate(t *testing.T) {
	ts, pw, configPath := newQUICServer(t, true)

	var got okEnv[quicConfigResp]
	if code := authGet(t, ts.URL+"/api/v1/quic-config", pw, &got); code != http.StatusOK {
		t.Fatalf("GET missing Config.json status=%d", code)
	}
	if got.Data.PublicIP != "" {
		t.Fatalf("GET missing Config.json publicIP=%q", got.Data.PublicIP)
	}
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", `{"publicIP":""}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("blank PUT status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("blank PUT must not create Config.json: %v", err)
	}
}

func TestQUICConfig_InvalidExistingAndMissingInstall(t *testing.T) {
	ts, pw, configPath := newQUICServer(t, true)
	if err := os.WriteFile(configPath, []byte(`{"quicConfig":"invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", `{"publicIP":"203.0.113.1"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("invalid existing config status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	if err := os.WriteFile(configPath, []byte(`null`), 0o600); err != nil {
		t.Fatal(err)
	}
	resp = authReq(t, http.MethodPut, ts.URL+"/api/v1/quic-config", pw, "application/json", `{"publicIP":"203.0.113.1"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("null root config status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	tsMissing, pwMissing, _ := newQUICServer(t, false)
	resp = authReq(t, http.MethodPut, tsMissing.URL+"/api/v1/quic-config", pwMissing, "application/json", `{"publicIP":"203.0.113.1"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("missing Headless dir status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}
