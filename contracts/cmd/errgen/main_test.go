package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunWritesGoPythonAndTypeScriptOutputs(t *testing.T) {
	directory := t.TempDir()
	canonical := filepath.Join(directory, "errors", "errors.go")
	python := filepath.Join(directory, "python", "codes.py")
	typescript := filepath.Join(directory, "typescript", "codes.ts")
	require.NoError(t, run("../../internal/errgen/testdata/errors.yaml", canonical, python, typescript))
	for path, expected := range map[string]string{
		canonical: "package errors",
		python:    "INTERNAL_ERROR", typescript: "INTERNAL_ERROR",
	} {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Contains(t, string(content), expected)
	}
}

func TestRunRejectsInvalidCatalogWithoutWritingOutputs(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "invalid.yaml")
	output := filepath.Join(directory, "errors.go")
	require.NoError(t, os.WriteFile(source, []byte("version: 2\nerrors: []\n"), filePerm))
	require.Error(t, run(source, output, "", ""))
	require.NoFileExists(t, output)
}
