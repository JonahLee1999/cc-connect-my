package core

import (
	"strings"
	"testing"
)

func newVoiceTestEngine() (*Engine, *stubPlatformEngine, *[]string) {
	e := newTestEngine()
	e.i18n = NewI18n(LangChinese)
	saved := &[]string{}
	e.SetTTSConfig(&TTSCfg{
		Enabled: true, Provider: "fish", Voice: "id-dingzhen", TTS: &FishTTS{},
		Presets: map[string]string{"丁真": "id-dingzhen", "温柔动听女声": "id-soft", "AD学姐": "id-ad"},
	})
	e.SetTTSVoiceSaveFunc(func(v string) error { *saved = append(*saved, v); return nil })
	return e, &stubPlatformEngine{n: "feishu"}, saved
}

func TestCmdTTSVoice_ListAndSwitch(t *testing.T) {
	e, p, saved := newVoiceTestEngine()
	msg := &Message{SessionKey: "feishu:c:u", UserID: "u", ReplyCtx: "ctx"}

	e.cmdTTS(p, msg, []string{"voice"})
	list := strings.Join(p.getSent(), "\n")
	if !strings.Contains(list, "当前音色：丁真") || !strings.Contains(list, "✅ 丁真") || !strings.Contains(list, "温柔动听女声") {
		t.Fatalf("unexpected list: %q", list)
	}

	e.cmdTTS(p, msg, []string{"voice", "ad学姐"}) // case-insensitive preset name
	if got := e.tts.GetVoice(); got != "id-ad" {
		t.Fatalf("voice=%q, want id-ad", got)
	}
	e.cmdTTS(p, msg, []string{"voice", "0123456789abcdef0123456789abcdef"}) // raw voice ID
	if got := e.tts.GetVoice(); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("raw id not applied: %q", got)
	}
	e.cmdTTS(p, msg, []string{"voice", "没有", "这个"})
	if got := e.tts.GetVoice(); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unknown name must not switch: %q", got)
	}
	if len(*saved) != 2 || (*saved)[0] != "id-ad" {
		t.Fatalf("saved=%v", *saved)
	}
	last := p.getSent()[len(p.getSent())-1]
	if !strings.Contains(last, "没有这个音色") {
		t.Fatalf("expected unknown-voice reply, got %q", last)
	}
}

func TestTTSVoiceSwitchIsPrivileged(t *testing.T) {
	if isPrivilegedCommandInvocation("tts", []string{"voice"}) {
		t.Error("listing voices should be open to everyone")
	}
	if !isPrivilegedCommandInvocation("tts", []string{"voice", "丁真"}) {
		t.Error("switching the voice should require admin")
	}
	if isPrivilegedCommandInvocation("tts", []string{"always"}) {
		t.Error("tts mode switching keeps its existing permission")
	}
}
