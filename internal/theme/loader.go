package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oxforge/diago/themes"
)

// ThemeSummary holds the name and description of a theme.
type ThemeSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// List returns summaries of all available embedded themes.
func List() ([]ThemeSummary, error) {
	entries, err := themes.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("list themes: %w", err)
	}
	var summaries []ThemeSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := themes.FS.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("list themes: read %s: %w", e.Name(), err)
		}
		var jt jsonTheme
		if err := json.Unmarshal(data, &jt); err != nil {
			return nil, fmt.Errorf("list themes: parse %s: %w", e.Name(), err)
		}
		summaries = append(summaries, ThemeSummary{
			Name:        jt.Name,
			Description: jt.Description,
		})
	}
	return summaries, nil
}

// Load loads a theme by name. It first looks in the embedded FS (themes/*.json),
// then falls back to DIAGO_THEME_DIR if set.
func Load(name string) (Theme, error) {
	filename := name + ".json"

	// Try embedded FS first.
	data, err := themes.FS.ReadFile(filename)
	if err != nil {
		// Try DIAGO_THEME_DIR if set.
		themeDir := os.Getenv("DIAGO_THEME_DIR")
		if themeDir == "" {
			return Theme{}, fmt.Errorf("theme %q not found", name)
		}
		data, err = os.ReadFile(filepath.Join(themeDir, filename))
		if err != nil {
			return Theme{}, fmt.Errorf("theme %q not found: %w", name, err)
		}
	}

	var jt jsonTheme
	if err := json.Unmarshal(data, &jt); err != nil {
		return Theme{}, fmt.Errorf("theme %q: invalid JSON: %w", name, err)
	}

	th := toTheme(jt)

	if err := Validate(th); err != nil {
		return Theme{}, fmt.Errorf("theme %q: %w", name, err)
	}

	return th, nil
}
