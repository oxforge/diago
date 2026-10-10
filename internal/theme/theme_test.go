package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultThemeSetsWhatValidateDoesNot: Load validates a theme, so the
// default's colors, strokes and fonts are already checked; these are the
// fields validate leaves open.
func TestDefaultThemeSetsWhatValidateDoesNot(t *testing.T) {
	th := DefaultTheme()
	assert.Equal(t, "default", th.Name)
	assert.Greater(t, th.Node.Padding.X, 0.0, "Node.Padding.X should be positive")
	assert.Greater(t, th.Node.Padding.Y, 0.0, "Node.Padding.Y should be positive")
	assert.NotEmpty(t, th.Group.StrokeDash, "Group.StrokeDash should be set")
}

func TestTheme_DiffColors(t *testing.T) {
	for name, want := range map[string]DiffStyle{
		"default":  {Added: "#2f9e44", Changed: "#1971c2", Removed: "#e03131"},
		"dark":     {Added: "#51cf66", Changed: "#4dabf7", Removed: "#ff6b6b"},
		"midnight": {Added: "#51cf66", Changed: "#74c0fc", Removed: "#ff6b6b"},
		"sketch":   {Added: "#2b8a3e", Changed: "#1864ab", Removed: "#c92a2a"},
	} {
		th, err := Load(name)
		require.NoError(t, err, name)
		assert.Equal(t, want, th.Diff, name)
	}
}

func TestTheme_DiffFallbackToPalette(t *testing.T) {
	colors := map[string]string{"green": "#00ff00", "blue": "#0000ff", "red": "#ff0000", "orange": "#ff8800"}
	assert.Equal(t, DiffStyle{Added: "#00ff00", Changed: "#0000ff", Removed: "#ff0000"}, DiffStyle{}.orPalette(colors),
		"no diff block: green, blue and red")
	assert.Equal(t, DiffStyle{Added: "#00ff00", Changed: "#222222", Removed: "#ff0000"}, DiffStyle{Changed: "#222222"}.orPalette(colors),
		"a partial block falls back per key")
	full := DiffStyle{Added: "#111111", Changed: "#222222", Removed: "#333333"}
	assert.Equal(t, full, full.orPalette(colors))
}

func TestDefaultThemeHasClassStyle(t *testing.T) {
	th := DefaultTheme()
	assert.Equal(t, "Inter", th.Class.MemberFont.Family)
	assert.Equal(t, 12.0, th.Class.MemberFont.Size)
	assert.Equal(t, th.Node.Font.Color, th.Class.MemberFont.Color)
	assert.Equal(t, 1.0, th.Class.SeparatorWidth)
}

func TestTheme_Accent(t *testing.T) {
	th := Theme{Colors: map[string]string{"red": "#e03131"}}
	for _, tc := range []struct{ color, want string }{
		{"", ""},
		{"red", "#e03131"},
		{"#123456", "#123456"},
		{"teal", ""},
	} {
		assert.Equal(t, tc.want, th.Accent(tc.color), "%q", tc.color)
	}
}
