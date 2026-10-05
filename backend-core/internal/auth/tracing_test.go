package auth

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The review explicitly requires named trace constants; this checks that source
// contract while observability_test.go verifies the emitted spans themselves.
func TestAuthTraceNames_AreDeclaredOnce(t *testing.T) {
	names := map[string]int{"core.auth": 0, "core.auth.session.purge": 0, "core.auth.limiter.check": 0, "core.auth.limiter.reset": 0, "core.auth.argon.create": 0, "core.auth.argon.compare": 0}
	for _, directory := range []string{".", filepath.Join("..", "..", "cmd", "core")} {
		files, err := filepath.Glob(filepath.Join(directory, "*.go"))
		require.NoError(t, err)
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			require.NoError(t, err)
			constants := declaredLiterals(file)
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				require.NoError(t, err)
				if _, tracked := names[value]; tracked {
					names[value]++
					require.True(t, constants[literal], "%s: %s must be declared as a named constant", path, value)
				}
				return true
			})
		}
	}
	for name, count := range names {
		require.Equal(t, 1, count, "%s must have one declaration", name)
	}
}

func declaredLiterals(file *ast.File) map[*ast.BasicLit]bool {
	constants := map[*ast.BasicLit]bool{}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		ast.Inspect(group, func(node ast.Node) bool {
			if literal, ok := node.(*ast.BasicLit); ok {
				constants[literal] = true
			}
			return true
		})
	}
	return constants
}
