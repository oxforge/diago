package png

import (
	"context"
	"encoding/binary"
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResvgNotFound(t *testing.T) {
	t.Setenv("DIAGO_RESVG_PATH", "/nonexistent/resvg-does-not-exist")
	_, err := Render(context.Background(), []byte("<svg></svg>"), Options{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrResvgNotFound), "expected ErrResvgNotFound, got: %v", err)
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
