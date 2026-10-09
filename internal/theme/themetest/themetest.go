// Package themetest gives tests the embedded default theme.
package themetest

import (
	"fmt"

	"github.com/oxforge/diago/internal/theme"
)

// The default theme is embedded at compile time: a failure to load it means
// the binary itself is corrupt, so the package asserts it at init.
func init() {
	if _, err := theme.Load("default"); err != nil {
		panic(fmt.Sprintf("themetest: failed to load embedded default theme: %v", err))
	}
}

// Default returns the default light theme, a fresh copy on every call.
func Default() theme.Theme {
	th, _ := theme.Load("default") // checked at init
	return th
}
