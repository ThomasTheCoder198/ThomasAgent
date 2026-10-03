package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thomasthecoder198/thomastheragx/contracts/internal/errorcodegen"
)

const (
	dirPerm  = 0o755
	filePerm = 0o644
)

func main() {
	inputPath := flag.String("in", "errors.yaml", "catalog file")
	goOut := flag.String("go", "", "Go output file")
	pyOut := flag.String("py", "", "Python output file")
	tsOut := flag.String("ts", "", "TypeScript output file")
	flag.Parse()

	if err := run(*inputPath, *goOut, *pyOut, *tsOut); err != nil {
		fmt.Fprintln(os.Stderr, "errorcodegen:", err)
		os.Exit(1)
	}
}

func run(inputPath, goOut, pyOut, tsOut string) error {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}
	catalog, err := errorcodegen.Parse(raw)
	if err != nil {
		return err
	}
	goSrc, err := errorcodegen.RenderGo(catalog)
	if err != nil {
		return err
	}
	outputs := map[string]string{goOut: goSrc, pyOut: errorcodegen.RenderPython(catalog), tsOut: errorcodegen.RenderTS(catalog)}
	for path, content := range outputs {
		if path == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
			return fmt.Errorf("mkdir for %s: %w", path, err)
		}
		if err := os.WriteFile(path, []byte(content), filePerm); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}
