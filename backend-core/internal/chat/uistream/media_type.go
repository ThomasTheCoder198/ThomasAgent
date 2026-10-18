package uistream

import (
	"path/filepath"
	"strings"
)

// mediaTypes covers the ragx ingest formats; it is a fixed table so tests never depend on the host's MIME registry.
var mediaTypes = map[string]string{
	".pdf":  "application/pdf",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".csv":  "text/csv",
	".md":   "text/markdown",
	".txt":  "text/plain",
	".html": "text/html",
	".htm":  "text/html",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
}

const defaultMediaType = "application/octet-stream"

func mediaTypeOf(fileName string) string {
	if mediaType, ok := mediaTypes[strings.ToLower(filepath.Ext(fileName))]; ok {
		return mediaType
	}
	return defaultMediaType
}
