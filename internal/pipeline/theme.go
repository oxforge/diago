package pipeline

import (
	"encoding/json"

	"github.com/oxforge/diago/internal/schema"
	"github.com/oxforge/diago/internal/theme"
)

func resolveTheme(name string) (theme.Theme, error) {
	if name == "" {
		name = "default"
	}
	return theme.Load(name)
}

// specThemeName is the theme a render of spec uses: override (the -theme flag)
// when set, else the spec's top-level theme field, else "default". A spec
// that is not valid JSON names no theme; its parse reports the error.
func specThemeName(spec []byte, override string) string {
	if override != "" {
		return override
	}
	var probe struct {
		Theme string `json:"theme"`
	}
	if json.Unmarshal(spec, &probe) == nil && probe.Theme != "" {
		return probe.Theme
	}
	return "default"
}

// specTheme loads the theme a render of spec uses (specThemeName). A theme the
// spec names that does not load is a validation error on its field "theme";
// one override names stays the error theme.Load returns.
func specTheme(spec []byte, override string) (theme.Theme, error) {
	th, err := theme.Load(specThemeName(spec, override))
	if err != nil && override == "" {
		return theme.Theme{}, schema.ValidationErrors{{Field: "theme", Message: err.Error()}}
	}
	return th, err
}

// RenderTheme names the theme a render of spec in format uses, override
// (the -theme flag) when set, else the spec's theme field, else "default",
// checked to load as the render checks it, in every format: a spec's name
// that does not load is a validation error on "theme", an override's the
// error theme.Load returns. Text art takes no theme, so for it the name is
// "". ParsePrevious lays a previous spec out under it.
func RenderTheme(spec []byte, format, override string) (string, error) {
	if _, err := specTheme(spec, override); err != nil {
		return "", err
	}
	if isTextFormat(format) {
		return "", nil
	}
	return specThemeName(spec, override), nil
}
