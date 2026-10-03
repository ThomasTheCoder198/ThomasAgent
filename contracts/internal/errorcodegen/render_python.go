package errorcodegen

import (
	"fmt"
	"strings"
)

func RenderPython(catalog Catalog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", generatedHeader)
	b.WriteString("from dataclasses import dataclass\nfrom enum import StrEnum\nfrom typing import Final\n\n\nclass ErrorCode(StrEnum):\n")
	for _, definition := range catalog.Errors {
		fmt.Fprintf(&b, "    %s = %q\n", definition.Code, definition.Code)
	}
	b.WriteString("\n\n@dataclass(frozen=True)\nclass ErrorDefinition:\n    http_status: int\n    retryable: bool\n    message_vi: str\n    message_en: str\n\n\n")
	b.WriteString("ERROR_DEFINITIONS: Final[dict[ErrorCode, ErrorDefinition]] = {\n")
	for _, definition := range catalog.Errors {
		fmt.Fprintf(&b, "    ErrorCode.%s: ErrorDefinition(http_status=%d, retryable=%s, message_vi=%q, message_en=%q),\n",
			definition.Code, definition.Status, pyBool(definition.Retryable), definition.Message.VI, definition.Message.EN)
	}
	b.WriteString("}\n")
	return b.String()
}

func pyBool(v bool) string {
	if v {
		return "True"
	}
	return "False"
}
