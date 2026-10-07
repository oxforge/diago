package hooks_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	terminal  = "This session runs in a terminal"
	graphical = "This session may run in a graphical host"
	missing   = "The hook did not find the diago CLI on its PATH"
)

// runHook runs session-start with nothing in its environment but a PATH
// (holding a stub diago when withDiago is set) and env, checks the JSON it
// prints and returns its additionalContext.
func runHook(t *testing.T, withDiago bool, env ...string) string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	bin := t.TempDir()
	if withDiago {
		require.NoError(t, os.WriteFile(filepath.Join(bin, "diago"), []byte("#!/bin/sh\n"), 0o755))
	}
	cmd := exec.Command(bash, "session-start")
	cmd.Env = append([]string{"PATH=" + bin}, env...)
	out, err := cmd.Output()
	require.NoError(t, err)
	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal(out, &got), "output: %s", out)
	require.Equal(t, "SessionStart", got.HookSpecificOutput.HookEventName)
	return got.HookSpecificOutput.AdditionalContext
}

func TestSessionStart_WordsTheRuleForTheHost(t *testing.T) {
	tests := []struct {
		name       string
		entrypoint string // "" leaves CLAUDE_CODE_ENTRYPOINT unset
		want, not  string
	}{
		{"terminal", "cli", terminal, graphical},
		{"print mode", "sdk-cli", terminal, graphical},
		{"desktop", "claude-desktop", graphical, terminal},
		{"unset", "", graphical, terminal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var env []string
			if tt.entrypoint != "" {
				env = append(env, "CLAUDE_CODE_ENTRYPOINT="+tt.entrypoint)
			}
			got := runHook(t, true, env...)
			assert.Contains(t, got, "use the diago:illustrating skill")
			assert.Contains(t, got, "a change to code, architecture or business logic (one you made or propose, or a commit, branch or diff you summarize) is shown as a diff diagram")
			assert.Contains(t, got, tt.want)
			assert.NotContains(t, got, tt.not)
			assert.NotContains(t, got, missing)
		})
	}
}

func TestSessionStart_NamesTheScript(t *testing.T) {
	got := runHook(t, true, "CLAUDE_PLUGIN_ROOT=/opt/plugins/diago")
	assert.Contains(t, got, "Its diago-render script is at /opt/plugins/diago/scripts/diago-render.")
	assert.NotContains(t, runHook(t, true), "diago-render script")
}

func TestSessionStart_EscapesThePluginRoot(t *testing.T) {
	root := `/odd "dir" \ name`
	got := runHook(t, true, "CLAUDE_PLUGIN_ROOT="+root)
	assert.Contains(t, got, root+"/scripts/diago-render")
}

func TestSessionStart_WithoutDiagoTellsTheAgentToCheckItsShell(t *testing.T) {
	got := runHook(t, false, "CLAUDE_CODE_ENTRYPOINT=cli")
	assert.Contains(t, got, "use the diago:illustrating skill")
	assert.Contains(t, got, terminal)
	assert.Contains(t, got, missing)
	assert.Contains(t, got, "if `command -v diago` fails in your shell too")
	assert.Contains(t, got, "go install github.com/oxforge/diago/cmd/diago@latest")
}

func TestHooksJSON_RunsSessionStartOnEveryFreshContext(t *testing.T) {
	raw, err := os.ReadFile("hooks.json")
	require.NoError(t, err)
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(raw, &cfg))
	require.Len(t, cfg.Hooks, 1)
	ss := cfg.Hooks["SessionStart"]
	require.Len(t, ss, 1)
	assert.Equal(t, "startup|clear|compact", ss[0].Matcher)
	require.Len(t, ss[0].Hooks, 1)
	assert.Equal(t, "command", ss[0].Hooks[0].Type)
	assert.Equal(t, `bash "${CLAUDE_PLUGIN_ROOT}/hooks/session-start"`, ss[0].Hooks[0].Command)
}
