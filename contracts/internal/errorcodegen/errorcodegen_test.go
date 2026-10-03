package errorcodegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T) Catalog {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "errors.yaml"))
	require.NoError(t, err)
	catalog, err := Parse(raw)
	require.NoError(t, err)
	return catalog
}

// Golden files written on a Windows host may carry CRLF; renderers always emit LF.
func golden(t *testing.T, name, output string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(path, []byte(output), 0o644))
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

func TestRenderGo_MatchesGolden(t *testing.T) {
	out, err := RenderGo(loadFixture(t))
	require.NoError(t, err)
	require.Equal(t, golden(t, "golden.go.txt", out), out)
}

func TestRenderGo_DefinesNamedErrorsAndReadableDefinitions(t *testing.T) {
	out, err := RenderGo(loadFixture(t))
	require.NoError(t, err)
	require.Contains(t, out, "package errors")
	require.Contains(t, out, "ErrRateLimited")
	require.Contains(t, out, "NamedError = NamedError(CodeRateLimited)")
	require.Contains(t, out, "func LookupDefinition(code Code) ErrorDefinition")
	require.Contains(t, out, "HTTPStatus:")
	require.Contains(t, out, "MessageVI:")
	require.NotContains(t, out, "var catalog")
}

func TestRenderPython_MatchesGolden(t *testing.T) {
	output := RenderPython(loadFixture(t))
	require.Equal(t, golden(t, "golden.py.txt", output), output)
}

func TestRenderTS_MatchesGolden(t *testing.T) {
	output := RenderTS(loadFixture(t))
	require.Equal(t, golden(t, "golden.ts.txt", output), output)
}

func TestParse_RejectsInvalidCatalog(t *testing.T) {
	cases := map[string]string{
		"lowercase code":      "version: 1\nerrors:\n  - {code: bad_code, status: 400, retryable: false, message: {vi: a, en: b}}\n",
		"duplicate code":      "version: 1\nerrors:\n  - {code: A_B, status: 400, retryable: false, message: {vi: a, en: b}}\n  - {code: A_B, status: 400, retryable: false, message: {vi: a, en: b}}\n",
		"status out of range": "version: 1\nerrors:\n  - {code: A_B, status: 99, retryable: false, message: {vi: a, en: b}}\n",
		"missing message":     "version: 1\nerrors:\n  - {code: A_B, status: 400, retryable: false, message: {vi: a}}\n",
		"wrong version":       "version: 2\nerrors: []\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(src))
			require.Error(t, err)
		})
	}
}
