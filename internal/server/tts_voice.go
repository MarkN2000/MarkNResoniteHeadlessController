package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	ttsSpeakersURL = "https://tts.markn2000.com/api/v1/speakers"
	ttsAPIURL      = "https://tts.markn2000.com/api/v1/tts"
)

// impulseValueForTemplate はテンプレート種別に応じた dynamicimpulsestring の値を作る。
// ttsVoice だけがテキストと話者 style ID を TTS API URL へ変換し、それ以外は従来どおり本文を返す。
// セッションのスポーン＆パルスとスケジュール告知で共用する。
func impulseValueForTemplate(tpl *itemTemplate, message string, speakerID int64) (string, error) {
	if tpl == nil || tpl.Input == nil {
		if speakerID != 0 {
			return "", fmt.Errorf("speakerId は ttsVoice テンプレートでのみ指定できます")
		}
		return message, nil
	}
	if tpl.Input.Kind != templateInputTTSVoice {
		return "", fmt.Errorf("未対応のテンプレート入力種別です")
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("ttsVoice テンプレートでは message を入力してください")
	}
	if speakerID <= 0 {
		return "", fmt.Errorf("ttsVoice テンプレートでは speakerId を指定してください")
	}
	return ttsAPIURL + "?text=" + url.QueryEscape(message) + "&speaker=" + fmt.Sprint(speakerID), nil
}

type ttsUpstreamSpeaker struct {
	Name   string `json:"name"`
	Styles []struct {
		Name string `json:"name"`
		ID   int64  `json:"id"`
	} `json:"styles"`
}

type ttsVoice struct {
	ID          int64  `json:"id"`
	SpeakerName string `json:"speakerName"`
	StyleName   string `json:"styleName"`
}

// handleTTSSpeakers は固定 TTS サービスの話者一覧を UI 用のフラットな voices へ変換する。
func (s *Server) handleTTSSpeakers(w http.ResponseWriter, r *http.Request) {
	client := s.ttsHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.ttsSpeakersURL, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "話者一覧を取得できません")
		return
	}
	res, err := client.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "話者一覧を取得できません")
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		writeErr(w, http.StatusBadGateway, "upstream_error", "話者一覧を取得できません")
		return
	}
	var speakers []ttsUpstreamSpeaker
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&speakers); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "話者一覧を取得できません")
		return
	}
	voices := make([]ttsVoice, 0)
	for _, speaker := range speakers {
		for _, style := range speaker.Styles {
			if style.ID <= 0 || strings.TrimSpace(speaker.Name) == "" || strings.TrimSpace(style.Name) == "" {
				continue
			}
			voices = append(voices, ttsVoice{ID: style.ID, SpeakerName: speaker.Name, StyleName: style.Name})
		}
	}
	writeOK(w, map[string]any{"voices": voices})
}
