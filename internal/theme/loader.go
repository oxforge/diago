package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oxforge/diago/themes"
)

// Load loads a theme by name. It first looks in the embedded FS (themes/*.json),
// then falls back to DIAGO_THEME_DIR if set. A name is a plain file name,
// without a path, so a theme never comes from outside that directory.
func Load(name string) (Theme, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return Theme{}, fmt.Errorf("theme %q: a theme name is a plain file name, without a path", name)
	}
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
		if errors.Is(err, fs.ErrNotExist) {
			return Theme{}, fmt.Errorf("theme %q not found", name)
		}
		if err != nil {
			var pe *fs.PathError
			if errors.As(err, &pe) {
				err = pe.Err // the reason, without the theme directory's path
			}
			return Theme{}, fmt.Errorf("theme %q: read: %w", name, err)
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
