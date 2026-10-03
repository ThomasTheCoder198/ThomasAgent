package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thomasthecoder198/thomastheragx/contracts/internal/errgen"
)

const (
	dirPerm  = 0o755
	filePerm = 0o644
)

func main() {
	in := flag.String("in", "errors.yaml", "catalog file")
	goOut := flag.String("go", "", "Go output file")
	goCompatibilityOut := flag.String("go-compat", "", "Go compatibility aliases output file")
	pyOut := flag.String("py", "", "Python output file")
	tsOut := flag.String("ts", "", "TypeScript output file")
	flag.Parse()

	if err := run(*in, *goOut, *goCompatibilityOut, *pyOut, *tsOut); err != nil {
		fmt.Fprintln(os.Stderr, "errgen:", err)
		os.Exit(1)
	}
}

func run(in, goOut, goCompatibilityOut, pyOut, tsOut string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("read %s: %w", in, err)
	}
	cat, err := errgen.Parse(raw)
	if err != nil {
		return err
	}
	goSrc, err := errgen.RenderGo(cat)
	if err != nil {
		return err
	}
	compatibilitySrc, err := errgen.RenderGoCompatibility(cat)
	if err != nil {
		return err
	}
	outputs := map[string]string{goOut: goSrc, goCompatibilityOut: compatibilitySrc, pyOut: errgen.RenderPython(cat), tsOut: errgen.RenderTS(cat)}
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
