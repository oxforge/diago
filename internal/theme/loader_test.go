package theme

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/themes"
)

func TestLoadDefaultTheme(t *testing.T) {
	th, err := Load("default")
	require.NoError(t, err)
	assert.Equal(t, "default", th.Name)
	assert.Equal(t, "clean", th.Style)
	assert.Equal(t, "#ffffff", th.Background)
	assert.Equal(t, "#f8f9fa", th.Node.Fill)
	assert.Equal(t, "#343a40", th.Node.Stroke)
	assert.Equal(t, 1.5, th.Node.StrokeWidth)
	assert.Equal(t, 6.0, th.Node.CornerRadius)
	assert.Equal(t, "Inter", th.Node.Font.Family)
	assert.Equal(t, 14.0, th.Node.Font.Size)
	assert.Equal(t, 500, th.Node.Font.Weight)
	assert.Equal(t, "#212529", th.Node.Font.Color)
	assert.Equal(t, 20.0, th.Node.Padding.X)
	assert.Equal(t, 12.0, th.Node.Padding.Y)
}

func TestLoadUnknownThemeError(t *testing.T) {
	_, err := Load("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestValidateRejectsEmptyBackground(t *testing.T) {
	th := DefaultTheme()
	th.Background = ""
	err := validate(th)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "background")
}

func TestValidateRejectsInvalidColor(t *testing.T) {
	th := DefaultTheme()
	th.Background = "notacolor"
	err := validate(th)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notacolor")
}

func TestValidateRejectsInvalidDiffRemoved(t *testing.T) {
	th := DefaultTheme()
	th.Diff.Removed = "notacolor"
	err := validate(th)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "diff.removed")
}

func TestValidateRejectsNegativeStrokeWidth(t *testing.T) {
	th := DefaultTheme()
	th.Node.StrokeWidth = -1
	err := validate(th)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stroke_width")
}

func TestValidateRejectsUnknownStyle(t *testing.T) {
	th := DefaultTheme()
	th.Style = "fancy"
	err := validate(th)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fancy")
}

func TestValidateRejectsUnknownFontFamily(t *testing.T) {
	th := DefaultTheme()
	th.Node.Font.Family = "Comic Sans"
	err := validate(th)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Comic Sans")
}

func TestLoadFromCustomDir(t *testing.T) {
	dir := t.TempDir()

	customJSON := `{
		"name": "custom",
		"style": "clean",
		"background": "#aabbcc",
		"node": {
			"fill": "#aabbcc",
			"stroke": "#112233",
			"stroke_width": 1.0,
			"corner_radius": 4,
			"font": {"family": "Inter", "size": 12, "weight": 400, "color": "#112233"},
			"padding": {"x": 10, "y": 8}
		},
		"edge": {
			"stroke": "#112233",
			"stroke_width": 1.0,
			"arrow_size": 6,
			"label_font": {"family": "Inter", "size": 11, "weight": 400, "color": "#112233"}
		},
		"group": {
			"fill": "#aabbcc",
			"stroke": "#112233",
			"stroke_width": 1.0,
			"stroke_dash": "4,2",
			"label_font": {"family": "Inter", "size": 11, "weight": 600, "color": "#112233"},
			"padding": 16
		},
		"actor": {
			"fill": "#aabbcc",
			"stroke": "#112233",
			"stroke_width": 1.0,
			"font": {"family": "Inter", "size": 13, "weight": 600, "color": "#112233"}
		},
		"fragment": {
			"fill": "#aabbcc",
			"stroke": "#112233",
			"stroke_width": 1.0,
			"stroke_dash": "4,2",
			"label_font": {"family": "Inter", "size": 10, "weight": 700, "color": "#112233"}
		},
		"activation": {
			"fill": "#aabbcc",
			"stroke": "#112233",
			"stroke_width": 1.0,
			"width": 8
		},
		"class": {
			"member_font": {"family": "Inter", "size": 12, "weight": 400, "color": "#112233"},
			"separator_width": 1.0
		},
		"colors": {
			"red": "#e03131",
			"green": "#2f9e44",
			"blue": "#1971c2",
			"yellow": "#e8950a",
			"orange": "#e8590c",
			"purple": "#9c36b5",
			"gray": "#868e96"
		}
	}`

	err := os.WriteFile(filepath.Join(dir, "custom.json"), []byte(customJSON), 0644)
	require.NoError(t, err)

	t.Setenv("DIAGO_THEME_DIR", dir)

	th, err := Load("custom")
	require.NoError(t, err)
	assert.Equal(t, "custom", th.Name)
	assert.Equal(t, "#aabbcc", th.Background)
}

func TestAllEmbeddedThemesValid(t *testing.T) {
	names := []string{"dark", "default", "midnight", "sketch"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			th, err := Load(name)
			require.NoError(t, err)
			err = validate(th)
			assert.NoError(t, err)
		})
	}
}

// TestEmbeddedThemes_EveryKeyIsAField: every key of every built-in theme
// file decodes into a Theme field, so a key without its JSON tag fails here
// instead of loading as a zero value.
func TestEmbeddedThemes_EveryKeyIsAField(t *testing.T) {
	files, err := fs.Glob(themes.FS, "*.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		data, err := themes.FS.ReadFile(f)
		require.NoError(t, err)
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		var th Theme
		assert.NoError(t, dec.Decode(&th), f)
	}
}

func TestValidateRejectsMissingClassSection(t *testing.T) {
	th := DefaultTheme()
	th.Class.MemberFont.Family = ""
	err := validate(th)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "class.member_font.family")
	th = DefaultTheme()
	th.Class.SeparatorWidth = 0
	err = validate(th)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "class.separator_width")
}

func TestValidate_PaddingBounds(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(th *Theme)
		wantErr string // "" accepts
	}{
		{"node padding x negative", func(th *Theme) { th.Node.Padding.X = -1 }, "node.padding.x"},
		{"node padding y negative", func(th *Theme) { th.Node.Padding.Y = -0.5 }, "node.padding.y"},
		{"node padding zero", func(th *Theme) { th.Node.Padding = Padding{} }, ""},
		{"group padding 11", func(th *Theme) { th.Group.Padding = 11 }, "group.padding"},
		{"group padding 12", func(th *Theme) { th.Group.Padding = 12 }, ""},
		{"group padding 24", func(th *Theme) { th.Group.Padding = 24 }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := DefaultTheme()
			tt.edit(&th)
			err := validate(th)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// A theme name never reaches outside DIAGO_THEME_DIR, and the error names
// no directory.
func TestLoad_ANameIsAPlainFileName(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "themes")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "outside"), 0o755))
	def, err := themes.FS.ReadFile("default.json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "outside", "evil.json"), def, 0o644))
	t.Setenv("DIAGO_THEME_DIR", dir)
	for _, name := range []string{"../outside/evil", "a/b", "/abs", `a\b`, "..", "."} {
		_, err := Load(name)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "plain file name", name)
		assert.NotContains(t, err.Error(), root, name)
	}
}

// A theme found in neither the built-ins nor DIAGO_THEME_DIR is named, its
// directory not.
func TestLoad_AMissingThemeNamesNoDirectory(t *testing.T) {
	t.Setenv("DIAGO_THEME_DIR", t.TempDir())
	_, err := Load("nope")
	assert.EqualError(t, err, `theme "nope" not found`)
}
