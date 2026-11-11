package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type searchingTTS struct {
	calls   int
	results map[string]string
}

func (s *searchingTTS) Synthesize(context.Context, string, TTSSynthesisOpts) ([]byte, string, error) {
	return []byte("x"), "mp3", nil
}

func (s *searchingTTS) SearchVoice(_ context.Context, name string) (string, string, error) {
	s.calls++
	return s.results[name], name, nil
}

func TestResolveTTSVoice(t *testing.T) {
	st := &searchingTTS{results: map[string]string{"特朗普": "id-trump"}}
	e := newTestEngine()
	e.SetTTSConfig(&TTSCfg{Enabled: true, Voice: "id-default", TTS: st,
		Presets: map[string]string{"丁真": "id-dingzhen", "AD学姐": "id-ad"}})

	cases := map[string]string{
		"丁真":                               "id-dingzhen",                      // preset
		"ad学姐":                             "id-ad",                            // preset, case-insensitive
		"0123456789abcdef0123456789abcdef": "0123456789abcdef0123456789abcdef", // raw voice ID
		"特朗普":                              "id-trump",                         // searched
		"没有这个人":                            "",                                 // no match -> default voice
		"":                                 "",
	}
	for name, want := range cases {
		if got := e.resolveTTSVoice(name); got != want {
			t.Errorf("resolveTTSVoice(%q) = %q, want %q", name, got, want)
		}
	}
	calls := st.calls
	e.resolveTTSVoice("特朗普")
	e.resolveTTSVoice("没有这个人")
	if st.calls != calls {
		t.Errorf("searches should be cached, calls went %d -> %d", calls, st.calls)
	}
}

func TestFishTTS_SearchVoice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model" || r.URL.Query().Get("title") != "特朗普" || r.URL.Query().Get("sort_by") != "task_count" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"items":[{"_id":"a","title":"Trump EN"},{"_id":"b","title":"特朗普中文"},{"_id":"c","title":"特朗普2"}]}`))
	}))
	defer srv.Close()
	id, title, err := NewFishTTS("k", srv.URL, "", nil).SearchVoice(context.Background(), "特朗普")
	if err != nil || id != "b" || title != "特朗普中文" {
		t.Fatalf("id=%q title=%q err=%v", id, title, err)
	}
}
