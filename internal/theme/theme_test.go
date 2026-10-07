package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultThemeComplete(t *testing.T) {
	th := DefaultTheme()

	assert.NotEmpty(t, th.Background, "Background should be set")

	// Node
	assert.NotEmpty(t, th.Node.Fill, "Node.Fill should be set")
	assert.NotEmpty(t, th.Node.Stroke, "Node.Stroke should be set")
	assert.Greater(t, th.Node.StrokeWidth, 0.0, "Node.StrokeWidth should be positive")
	assert.NotEmpty(t, th.Node.Font.Family, "Node.Font.Family should be set")
	assert.Greater(t, th.Node.Font.Size, 0.0, "Node.Font.Size should be positive")
	assert.NotEmpty(t, th.Node.Font.Color, "Node.Font.Color should be set")
	assert.Greater(t, th.Node.Padding.X, 0.0, "Node.Padding.X should be positive")
	assert.Greater(t, th.Node.Padding.Y, 0.0, "Node.Padding.Y should be positive")

	// Edge
	assert.NotEmpty(t, th.Edge.Stroke, "Edge.Stroke should be set")
	assert.Greater(t, th.Edge.StrokeWidth, 0.0, "Edge.StrokeWidth should be positive")
	assert.Greater(t, th.Edge.ArrowSize, 0.0, "Edge.ArrowSize should be positive")
	assert.NotEmpty(t, th.Edge.LabelFont.Family, "Edge.LabelFont.Family should be set")
	assert.Greater(t, th.Edge.LabelFont.Size, 0.0, "Edge.LabelFont.Size should be positive")
	assert.NotEmpty(t, th.Edge.LabelFont.Color, "Edge.LabelFont.Color should be set")

	// Group
	assert.NotEmpty(t, th.Group.Fill, "Group.Fill should be set")
	assert.NotEmpty(t, th.Group.Stroke, "Group.Stroke should be set")
	assert.Greater(t, th.Group.StrokeWidth, 0.0, "Group.StrokeWidth should be positive")
	assert.NotEmpty(t, th.Group.StrokeDash, "Group.StrokeDash should be set")
	assert.NotEmpty(t, th.Group.LabelFont.Family, "Group.LabelFont.Family should be set")
	assert.Greater(t, th.Group.Padding, 0.0, "Group.Padding should be positive")
}

func TestDefaultThemeHasActorStyle(t *testing.T) {
	th := DefaultTheme()

	assert.NotEmpty(t, th.Actor.Fill, "Actor.Fill should be set")
	assert.NotEmpty(t, th.Actor.Stroke, "Actor.Stroke should be set")
	assert.Greater(t, th.Actor.StrokeWidth, 0.0, "Actor.StrokeWidth should be positive")
	assert.NotEmpty(t, th.Actor.Font.Family, "Actor.Font.Family should be set")
	assert.Greater(t, th.Actor.Font.Size, 0.0, "Actor.Font.Size should be positive")
	assert.NotEmpty(t, th.Actor.Font.Color, "Actor.Font.Color should be set")
}

func TestDefaultThemeHasFragmentStyle(t *testing.T) {
	th := DefaultTheme()

	assert.NotEmpty(t, th.Fragment.Stroke, "Fragment.Stroke should be set")
	assert.Greater(t, th.Fragment.StrokeWidth, 0.0, "Fragment.StrokeWidth should be positive")
	assert.NotEmpty(t, th.Fragment.LabelFont.Family, "Fragment.LabelFont.Family should be set")
	assert.Greater(t, th.Fragment.LabelFont.Size, 0.0, "Fragment.LabelFont.Size should be positive")
}

func TestDefaultThemeHasActivationStyle(t *testing.T) {
	th := DefaultTheme()

	assert.NotEmpty(t, th.Activation.Fill, "Activation.Fill should be set")
	assert.NotEmpty(t, th.Activation.Stroke, "Activation.Stroke should be set")
	assert.Greater(t, th.Activation.Width, 0.0, "Activation.Width should be positive")
}

func TestDefaultThemeHasNameAndStyle(t *testing.T) {
	th := DefaultTheme()
	assert.Equal(t, "default", th.Name)
	assert.Equal(t, "clean", th.Style)
}

func TestTheme_DiffColors(t *testing.T) {
	th, err := Load("default")
	require.NoError(t, err)
	assert.Equal(t, "#2f9e44", th.Diff.Added)
	assert.Equal(t, "#e8590c", th.Diff.Changed)
	for _, name := range []string{"dark", "midnight", "sketch"} {
		th, err := Load(name)
		require.NoError(t, err, name)
		assert.NotEmpty(t, th.Diff.Added, name)
		assert.NotEmpty(t, th.Diff.Changed, name)
	}
}

func TestTheme_DiffFallbackToPalette(t *testing.T) {
	jt := jsonTheme{Colors: map[string]string{"green": "#00ff00", "orange": "#ff8800"}}
	th := toTheme(jt)
	assert.Equal(t, "#00ff00", th.Diff.Added)
	assert.Equal(t, "#ff8800", th.Diff.Changed)
	jt.Diff = &jsonDiffStyle{Added: "#111111", Changed: "#222222"}
	th = toTheme(jt)
	assert.Equal(t, "#111111", th.Diff.Added)
	assert.Equal(t, "#222222", th.Diff.Changed)
}

func TestDefaultThemeHasClassStyle(t *testing.T) {
	th := DefaultTheme()
	assert.Equal(t, "Inter", th.Class.MemberFont.Family)
	assert.Equal(t, 12.0, th.Class.MemberFont.Size)
	assert.Equal(t, th.Node.Font.Color, th.Class.MemberFont.Color)
	assert.Equal(t, 1.0, th.Class.SeparatorWidth)
}
