package main

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRun_Version(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-version"}, {"version"}} {
		code, out, stderr := runCLI(t, "", args...)
		assert.Equal(t, 0, code, args)
		assert.Empty(t, stderr, args)
		assert.Regexp(t, `^diago \S+\n$`, out, args)
	}
	code, out, stderr := runCLI(t, "", "version", "extra")
	assert.Equal(t, 2, code)
	assert.Empty(t, out)
	assert.Contains(t, stderr, "usage: diago --version")
}

func TestVersionOf(t *testing.T) {
	for want, info := range map[string]*debug.BuildInfo{
		"v0.4.0": {Main: debug.Module{Version: "v0.4.0"}},
		"v0.0.0-20261008090856-c3bf15e9f56b+dirty": {Main: debug.Module{Version: "v0.0.0-20261008090856-c3bf15e9f56b+dirty"}},
		"(devel)": {Main: debug.Module{Version: ""}},
	} {
		assert.Equal(t, want, versionOf(info))
	}
}

// TestRun_HelpGoesToStdout: help asked for prints on stdout and exits 0, for
// every verb and every spelling; a bad flag still prints on stderr and
// exits 2.
func TestRun_HelpGoesToStdout(t *testing.T) {
	for _, verb := range []string{"render", "diff", "check", "import"} {
		for _, args := range [][]string{{verb, "-h"}, {verb, "--help"}, {"help", verb}} {
			code, out, stderr := runCLI(t, "", args...)
			assert.Equal(t, 0, code, args)
			assert.True(t, strings.HasPrefix(out, "usage: diago "+verb), "%v: %q", args, out)
			assert.Empty(t, stderr, args)
		}
	}
	_, out, _ := runCLI(t, "", "help", "render")
	assert.Contains(t, out, "  -format string", "a verb's help lists its flags")

	code, out, stderr := runCLI(t, "", "help", "bogus")
	assert.Equal(t, 2, code)
	assert.Empty(t, out)
	assert.True(t, strings.HasPrefix(stderr, "diago help: unknown command \"bogus\"\nusage: diago <verb>"), stderr)

	code, out, stderr = runCLI(t, "", "render", "-bogus")
	assert.Equal(t, 2, code)
	assert.Empty(t, out)
	assert.Contains(t, stderr, "flag provided but not defined: -bogus")
	assert.Contains(t, stderr, "usage: diago render")

	_, out, _ = runCLI(t, "", "help")
	assert.Contains(t, out, "diago help [verb]")
	assert.Contains(t, out, "diago --version")
}

// TestRun_EveryHelpSpellingExitsZero: -h, --h, -help and --help print a
// usage on stdout with exit code 0 wherever they stand: alone, after help
// and after any spelling of version, as they do after a verb.
func TestRun_EveryHelpSpellingExitsZero(t *testing.T) {
	for _, h := range []string{"-h", "--h", "-help", "--help"} {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{h}, "usage: diago <verb>"},
			{[]string{"help", h}, "usage: diago <verb>"},
			{[]string{"render", h}, "usage: diago render"},
			{[]string{"version", h}, "usage: diago --version"},
			{[]string{"--version", h}, "usage: diago --version"},
			{[]string{"-version", h}, "usage: diago --version"},
		} {
			code, out, stderr := runCLI(t, "", tc.args...)
			assert.Equal(t, 0, code, tc.args)
			assert.True(t, strings.HasPrefix(out, tc.want), "%v: %q", tc.args, out)
			assert.Empty(t, stderr, tc.args)
		}
	}
	for _, v := range []string{"--version", "-version"} {
		code, out, stderr := runCLI(t, "", "help", v)
		assert.Equal(t, 0, code, v)
		assert.True(t, strings.HasPrefix(out, "usage: diago --version"), "%s: %q", v, out)
		assert.Empty(t, stderr, v)
	}
}

func TestRun_HelpOnHelpAndVersion(t *testing.T) {
	code, out, _ := runCLI(t, "", "help", "help")
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(out, "usage: diago <verb>"), out)

	code, out, stderr := runCLI(t, "", "help", "version")
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(out, "usage: diago --version"), out)
	assert.Empty(t, stderr)

	code, out, stderr = runCLI(t, "", "help", "render", "extra")
	assert.Equal(t, 2, code)
	assert.Empty(t, out)
	assert.Contains(t, stderr, "usage: diago help [verb]")
}
