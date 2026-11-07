package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_RequiredAppendPromptFileFailsClosed(t *testing.T) {
	for _, content := range []string{"missing", "", "  \n", "---\nname: empty-policy\ndescription: no instructions\n---\n"} {
		t.Run(content, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "SKILL.md")
			if content != "missing" {
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := New(map[string]any{"cmd": "go", "work_dir": dir, "append_system_prompt_file": "SKILL.md"})
			if err == nil || !strings.Contains(err.Error(), "append_system_prompt_file") {
				t.Fatalf("required policy must fail closed, got %v", err)
			}
		})
	}
}

func TestAppendPromptFile_ReloadsAndStaysPinnedAcrossWorkspaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("---\nname: task-policy\ndescription: policy\n---\n"+body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("Current approval policy v1")
	created, err := New(map[string]any{"cmd": "go", "work_dir": dir, "append_system_prompt": "Project identity", "append_system_prompt_file": "SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}
	a := created.(*Agent)
	_, got, err := a.sessionPrompts()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Project identity") || !strings.Contains(got, "Current approval policy v1") || strings.Contains(got, "description: policy") {
		t.Fatalf("wrong loaded instructions: %q", got)
	}
	// Binding a new workspace must not redirect the configured policy to a
	// same-named file supplied by that workspace.
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "SKILL.md"), []byte("Untrusted replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	a.SetWorkDir(other)
	write("Current approval policy v2")
	_, got, err = a.sessionPrompts()
	if err != nil || !strings.Contains(got, "Current approval policy v2") || strings.Contains(got, "Untrusted replacement") {
		t.Fatalf("policy was stale or redirected: %q %v", got, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// Reject the session before starting a CLI or modifying provider state.
	if session, err := a.StartSession(context.Background(), "existing-thread"); err == nil || session != nil || !strings.Contains(err.Error(), "append_system_prompt_file") {
		t.Fatalf("resumed without required policy: session=%v error=%v", session, err)
	}
}
