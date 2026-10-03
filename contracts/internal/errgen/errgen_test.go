package errgen

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
	cat, err := Parse(raw)
	require.NoError(t, err)
	return cat
}

// Golden files written on a Windows host may carry CRLF; renderers always emit LF.
func golden(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

func TestRenderGoMatchesGolden(t *testing.T) {
	out, err := RenderGo(loadFixture(t))
	require.NoError(t, err)
	require.Equal(t, golden(t, "golden.go.txt"), out)
}

func TestRenderGoDefinesNamedErrorsAndReadableDefinitions(t *testing.T) {
	out, err := RenderGo(loadFixture(t))
	require.NoError(t, err)
	require.Contains(t, out, "package errors")
	require.Contains(t, out, "ErrRateLimited")
	require.Contains(t, out, "Sentinel = Sentinel(CodeRateLimited)")
	require.Contains(t, out, "func LookupDefinition(code Code) ErrorDefinition")
	require.Contains(t, out, "HTTPStatus:")
	require.Contains(t, out, "MessageVI:")
	require.NotContains(t, out, "var catalog")
}

func TestRenderGoCompatibilityMatchesGolden(t *testing.T) {
	out, err := RenderGoCompatibility(loadFixture(t))
	require.NoError(t, err)
	require.Equal(t, golden(t, "golden.compat.go.txt"), out)
	require.Contains(t, out, "type Code = catalogerrors.Code")
	require.NotContains(t, out, "Slow down")
}

func TestRenderPythonMatchesGolden(t *testing.T) {
	require.Equal(t, golden(t, "golden.py.txt"), RenderPython(loadFixture(t)))
}

func TestRenderTSMatchesGolden(t *testing.T) {
	require.Equal(t, golden(t, "golden.ts.txt"), RenderTS(loadFixture(t)))
}

func TestParseRejectsInvalidCatalog(t *testing.T) {
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
