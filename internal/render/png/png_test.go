package png

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResvgNotFound(t *testing.T) {
	tests := []struct {
		name     string
		envPath  string // DIAGO_RESVG_PATH; empty looks resvg up on PATH
		envDir   bool   // DIAGO_RESVG_PATH is a directory
		pathDir  bool   // PATH is an empty directory
		mentions []string
	}{
		{
			name:     "DIAGO_RESVG_PATH names a missing file",
			envPath:  "/nonexistent/resvg-does-not-exist",
			mentions: []string{"DIAGO_RESVG_PATH is /nonexistent/resvg-does-not-exist", "unset it"},
		},
		{
			name:     "DIAGO_RESVG_PATH names a directory",
			envDir:   true,
			mentions: []string{", a directory: point it at the resvg binary", "unset it"},
		},
		{
			name:     "no resvg on PATH",
			pathDir:  true,
			mentions: []string{"PNG output needs resvg", "brew install resvg", "cargo install resvg", "svg and text output need nothing"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envPath := tt.envPath
			if tt.envDir {
				envPath = t.TempDir()
			}
			t.Setenv("DIAGO_RESVG_PATH", envPath)
			if tt.pathDir {
				t.Setenv("PATH", t.TempDir())
			}
			_, err := Render(context.Background(), []byte("<svg></svg>"), Options{})
			require.Error(t, err)
			assert.True(t, errors.Is(err, errResvgNotFound), "expected errResvgNotFound, got: %v", err)
			for _, m := range tt.mentions {
				assert.Contains(t, err.Error(), m)
			}
		})
	}
}

func TestRenderValidSVG(t *testing.T) {
	if _, err := exec.LookPath("resvg"); err != nil {
		t.Skip("resvg not installed, skipping")
	}

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">
		<rect width="100" height="100" fill="red"/>
	</svg>`)

	data, err := Render(context.Background(), svg, Options{Scale: 1})
	require.NoError(t, err)
	require.True(t, len(data) > 8, "PNG output should not be empty")

	// Check PNG magic bytes: 0x89 P N G \r \n 0x1a \n
	assert.Equal(t, byte(0x89), data[0])
	assert.Equal(t, byte('P'), data[1])
	assert.Equal(t, byte('N'), data[2])
	assert.Equal(t, byte('G'), data[3])
}

func pngDimensions(data []byte) (width, height uint32) {
	// IHDR chunk starts at byte 16, width at 16, height at 20 (big-endian uint32)
	if len(data) < 24 {
		return 0, 0
	}
	width = binary.BigEndian.Uint32(data[16:20])
	height = binary.BigEndian.Uint32(data[20:24])
	return
}

func TestRenderWithScale(t *testing.T) {
	if _, err := exec.LookPath("resvg"); err != nil {
		t.Skip("resvg not installed, skipping")
	}

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50">
		<rect width="100" height="50" fill="blue"/>
	</svg>`)

	data, err := Render(context.Background(), svg, Options{Scale: 2})
	require.NoError(t, err)

	w, h := pngDimensions(data)
	assert.Equal(t, uint32(200), w, "width should be 2x")
	assert.Equal(t, uint32(100), h, "height should be 2x")
}

func TestRenderWithWidth(t *testing.T) {
	if _, err := exec.LookPath("resvg"); err != nil {
		t.Skip("resvg not installed, skipping")
	}

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50">
		<rect width="100" height="50" fill="green"/>
	</svg>`)

	data, err := Render(context.Background(), svg, Options{Width: 300})
	require.NoError(t, err)

	w, h := pngDimensions(data)
	assert.Equal(t, uint32(300), w, "width should match requested")
	assert.Equal(t, uint32(150), h, "height should scale proportionally")
}

func TestRenderInvalidSVG(t *testing.T) {
	if _, err := exec.LookPath("resvg"); err != nil {
		t.Skip("resvg not installed, skipping")
	}

	_, err := Render(context.Background(), []byte("not svg at all"), Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "png render")
	assert.NotContains(t, err.Error(), "Warning", "resvg's warnings are not its error")
}

func TestRenderInvalidOptions(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>`)

	_, err := Render(context.Background(), svg, Options{Width: -1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid width")

	_, err = Render(context.Background(), svg, Options{Height: -5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid height")

	_, err = Render(context.Background(), svg, Options{Scale: -1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid scale")
}

// TestRender_AFailureWithoutAMessageNamesItsExit: a resvg that fails
// with nothing on stderr reports how it ended, with no empty message
// before it.
func TestRender_AFailureWithoutAMessageNamesItsExit(t *testing.T) {
	fakeResvg(t, 2, "exit 3")
	_, err := Render(context.Background(), []byte("<svg></svg>"), Options{})
	require.Error(t, err)
	assert.Equal(t, "png render: exit status 3", err.Error())
}

// TestRender_ARunPastTheDeadlineReportsTheDeadline: a resvg run the
// context's deadline kills reports the deadline, not the kill's signal,
// so the CLI tells the user the render ran out of time. A stand-in resvg
// that outlives the deadline plays the slow run.
func TestRender_ARunPastTheDeadlineReportsTheDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in resvg is a shell script")
	}
	slow := filepath.Join(t.TempDir(), "resvg")
	require.NoError(t, os.WriteFile(slow, []byte("#!/bin/sh\nexec sleep 10\n"), 0o755))
	t.Setenv("DIAGO_RESVG_PATH", slow)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := Render(ctx, []byte("<svg></svg>"), Options{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, "png render: context deadline exceeded", err.Error())
}

// fakeResvg points DIAGO_RESVG_PATH at a stand-in resvg that warns as
// resvg does: about a missing --resources-dir when it reads stdin, on file
// descriptor warnFD (stdout before resvg 0.42, stderr since), and about the
// SVG on stderr unless --quiet is set. It then runs tail.
func fakeResvg(t *testing.T, warnFD int, tail string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in resvg is a shell script")
	}
	script := fmt.Sprintf(`#!/bin/sh
dir= quiet=
for a; do
  case $a in --resources-dir) dir=1 ;; --quiet) quiet=1 ;; esac
done
[ -n "$dir" ] || echo 'Warning: Make sure to set --resources-dir when reading SVG from stdin.' >&%d
[ -n "$quiet" ] || echo 'Warning (in simplecss:298): The @font-face rule is not supported. Skipped.' >&2
cat >/dev/null
%s
`, warnFD, tail)
	p := filepath.Join(t.TempDir(), "resvg")
	require.NoError(t, os.WriteFile(p, []byte(script), 0o755))
	t.Setenv("DIAGO_RESVG_PATH", p)
}

// TestRender_ResvgWarningsStayOutOfThePNG: a resvg that prints its stdin
// warning on stdout (0.26 to 0.41) does not put it ahead of the PNG.
func TestRender_ResvgWarningsStayOutOfThePNG(t *testing.T) {
	fakeResvg(t, 1, `printf '\211PNG'`)
	data, err := Render(context.Background(), []byte("<svg></svg>"), Options{})
	require.NoError(t, err)
	assert.Equal(t, "\x89PNG", string(data))
}

// TestRender_AFailureCarriesResvgsErrorAlone: the error of a failed run is
// resvg's error message, without its warnings or a trailing newline.
func TestRender_AFailureCarriesResvgsErrorAlone(t *testing.T) {
	fakeResvg(t, 2, "echo 'Error: unknown token at 1:1.' >&2\nexit 1")
	_, err := Render(context.Background(), []byte("<svg></svg>"), Options{})
	require.Error(t, err)
	assert.Equal(t, "png render: Error: unknown token at 1:1.: exit status 1", err.Error())
}
