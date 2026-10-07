package theme

import (
	"testing"
)

func TestDeriveNodeColors_LightBackground(t *testing.T) {
	colors := DeriveColors("#e03131", "#ffffff", ElementNode)
	if colors.Fill == "" || colors.Fill[0] != '#' {
		t.Fatalf("expected hex fill, got %q", colors.Fill)
	}
	if colors.Stroke == "" {
		t.Fatal("expected non-empty stroke")
	}
	if colors.Text == "" {
		t.Fatal("expected non-empty text color")
	}
	ratio := contrastRatio(colors.Text, colors.Fill)
	if ratio < 4.5 {
		t.Errorf("text/fill contrast ratio %.2f < 4.5 (text=%s fill=%s)", ratio, colors.Text, colors.Fill)
	}
}

func TestDeriveNodeColors_DarkBackground(t *testing.T) {
	colors := DeriveColors("#e03131", "#121212", ElementNode)
	if colors.Fill == "" || colors.Stroke == "" || colors.Text == "" {
		t.Fatal("expected non-empty colors")
	}
	ratio := contrastRatio(colors.Text, colors.Fill)
	if ratio < 4.5 {
		t.Errorf("text/fill contrast ratio %.2f < 4.5 (text=%s fill=%s)", ratio, colors.Text, colors.Fill)
	}
}

func TestDeriveEdgeColors(t *testing.T) {
	colors := DeriveColors("#1971c2", "#ffffff", ElementEdge)
	if colors.Stroke == "" || colors.Text == "" {
		t.Fatal("expected non-empty colors")
	}
	ratio := contrastRatio(colors.Text, "#ffffff")
	if ratio < 4.5 {
		t.Errorf("edge text/bg contrast ratio %.2f < 4.5", ratio)
	}
}

func TestDeriveGroupColors(t *testing.T) {
	colors := DeriveColors("#e8590c", "#ffffff", ElementGroup)
	if colors.Fill == "" || colors.Stroke == "" || colors.Text == "" {
		t.Fatal("expected non-empty colors")
	}
	ratio := contrastRatio(colors.Text, colors.Fill)
	if ratio < 4.5 {
		t.Errorf("group text/fill contrast ratio %.2f < 4.5", ratio)
	}
}

func TestDeriveColors_AllNamedColors_AllThemeBackgrounds(t *testing.T) {
	accents := []string{"#e03131", "#2f9e44", "#1971c2", "#e8950a", "#e8590c", "#9c36b5", "#868e96"}
	backgrounds := []string{"#ffffff", "#121212", "#0f0f14", "#faf8f5"}
	kinds := []ElementKind{ElementNode, ElementEdge, ElementGroup}

	for _, accent := range accents {
		for _, bg := range backgrounds {
			for _, kind := range kinds {
				colors := DeriveColors(accent, bg, kind)
				if colors.Fill == "" && kind != ElementEdge {
					t.Errorf("empty fill for accent=%s bg=%s kind=%d", accent, bg, kind)
				}
				if colors.Stroke == "" {
					t.Errorf("empty stroke for accent=%s bg=%s kind=%d", accent, bg, kind)
				}
				if colors.Text == "" {
					t.Errorf("empty text for accent=%s bg=%s kind=%d", accent, bg, kind)
				}
			}
		}
	}
}
