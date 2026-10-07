// Package png converts SVG bytes to PNG by invoking the resvg CLI as a subprocess.
package png

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/oxforge/diago/internal/font"
)

// ErrResvgNotFound is returned when the resvg binary cannot be located.
var ErrResvgNotFound = errors.New("resvg binary not found")

// notFoundError is an ErrResvgNotFound whose message says what to do next.
type notFoundError struct{ msg string }

func (e notFoundError) Error() string { return e.msg }
func (e notFoundError) Unwrap() error { return ErrResvgNotFound }

// Options controls PNG rendering parameters.
type Options struct {
	Width      int     // Target width in pixels (0 = use SVG intrinsic width).
	Height     int     // Target height in pixels (0 = use SVG intrinsic height).
	Scale      float64 // Scale factor (default 2.0). Ignored if Width or Height is set.
	Background string  // Background color (e.g. "#ffffff"). Empty = transparent/from SVG.
}

// findResvg locates the resvg binary. Checks DIAGO_RESVG_PATH first, then $PATH.
func findResvg() (string, error) {
	if p := os.Getenv("DIAGO_RESVG_PATH"); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", notFoundError{fmt.Sprintf("DIAGO_RESVG_PATH is %s, which does not exist: point it at the resvg binary, or unset it to look resvg up on PATH", p)}
		}
		return p, nil
	}
	p, err := exec.LookPath("resvg")
	if err != nil {
		return "", notFoundError{"PNG output needs resvg, and none is on PATH: install it (brew install resvg, cargo install resvg, or a binary from https://github.com/linebender/resvg/releases) or set DIAGO_RESVG_PATH to one; svg and text output need nothing"}
	}
	return p, nil
}

// validateOpts checks that rendering options are valid.
func validateOpts(opts Options) error {
	if opts.Width < 0 {
		return fmt.Errorf("png render: invalid width: %d (must be >= 0)", opts.Width)
	}
	if opts.Height < 0 {
		return fmt.Errorf("png render: invalid height: %d (must be >= 0)", opts.Height)
	}
	if opts.Scale < 0 {
		return fmt.Errorf("png render: invalid scale: %g (must be >= 0)", opts.Scale)
	}
	return nil
}

// writeFontFiles writes all embedded TTF fonts to a temporary directory
// and returns the directory path and a cleanup function.
func writeFontFiles() (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "diago-fonts-*")
	if err != nil {
		return "", nil, fmt.Errorf("png render: create font temp dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	for name, ttf := range font.EmbeddedTTFs() {
		path := filepath.Join(dir, name+".ttf")
		if err := os.WriteFile(path, ttf, 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("png render: write font %s: %w", name, err)
		}
	}
	return dir, cleanup, nil
}

// Render converts SVG bytes to PNG by invoking resvg.
func Render(ctx context.Context, svgBytes []byte, opts Options) ([]byte, error) {
	if err := validateOpts(opts); err != nil {
		return nil, err
	}

	bin, err := findResvg()
	if err != nil {
		return nil, err
	}

	fontDir, cleanup, err := writeFontFiles()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := buildArgs(opts, fontDir)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = bytes.NewReader(svgBytes)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("png render: %s: %w", stderr.String(), err)
	}

	return stdout.Bytes(), nil
}

// buildArgs constructs resvg CLI arguments from Options.
func buildArgs(opts Options, fontDir string) []string {
	args := []string{}

	if opts.Width > 0 {
		args = append(args, "--width", fmt.Sprintf("%d", opts.Width))
	}
	if opts.Height > 0 {
		args = append(args, "--height", fmt.Sprintf("%d", opts.Height))
	}
	if opts.Width == 0 && opts.Height == 0 {
		scale := opts.Scale
		if scale == 0 {
			scale = 2.0
		}
		args = append(args, "--zoom", fmt.Sprintf("%g", scale))
	}
	if opts.Background != "" {
		args = append(args, "--background", opts.Background)
	}

	// Load embedded fonts and skip system fonts for deterministic rendering.
	args = append(args, "--use-fonts-dir", fontDir, "--skip-system-fonts")

	// stdin input, stdout output
	args = append(args, "-", "-c")
	return args
}
