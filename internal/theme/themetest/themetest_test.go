package themetest

import "testing"

func TestDefaultIsTheDefaultTheme(t *testing.T) {
	if th := Default(); th.Name != "default" {
		t.Errorf("Default().Name = %q, want default", th.Name)
	}
}
