//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/oxforge/diago/internal/schema"
)

func main() {
	dir := filepath.Join("..", "..", "schemas")
	os.MkdirAll(dir, 0o755)

	for _, dt := range []string{"flow", "sequence", "class"} {
		data, err := schema.GenerateJSONSchema(dt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s schema: %v\n", dt, err)
			os.Exit(1)
		}
		path := filepath.Join(dir, dt+".json")
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", path)
	}
}
