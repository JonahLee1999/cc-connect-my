package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFishTTS_Synthesize(t *testing.T) {
	var got map[string]any
	var auth, model, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth, model = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("model")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3fake-mp3"))
	}))
	defer srv.Close()

	tts := NewFishTTS("sk-test", srv.URL, "", nil)
	audio, format, err := tts.Synthesize(context.Background(), "你好", TTSSynthesisOpts{Voice: "voice-123", Speed: 1.2})
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "ID3fake-mp3" || format != "mp3" {
		t.Fatalf("audio=%q format=%q", audio, format)
	}
	if path != "/v1/tts" || auth != "Bearer sk-test" || model != "s1" {
		t.Fatalf("path=%q auth=%q model=%q", path, auth, model)
	}
	if got["text"] != "你好" || got["reference_id"] != "voice-123" || got["format"] != "mp3" {
		t.Fatalf("unexpected body: %v", got)
	}
	if p, _ := got["prosody"].(map[string]any); p == nil || p["speed"] != 1.2 {
		t.Fatalf("speed not sent: %v", got["prosody"])
	}
}

func TestFishTTS_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"message":"insufficient balance"}`))
	}))
	defer srv.Close()
	_, _, err := NewFishTTS("sk-test", srv.URL, "", nil).Synthesize(context.Background(), "hi", TTSSynthesisOpts{})
	if err == nil || !strings.Contains(err.Error(), "402") {
		t.Fatalf("expected a 402 error, got %v", err)
	}
}
