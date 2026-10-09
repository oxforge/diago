package theme

// DefaultTheme loads the embedded default theme for this package's tests.
func DefaultTheme() Theme {
	th, err := Load("default")
	if err != nil {
		panic(err)
	}
	return th
}
