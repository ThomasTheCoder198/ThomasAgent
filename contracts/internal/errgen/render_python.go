package errgen

import (
	"fmt"
	"strings"
)

func RenderPython(cat Catalog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", generatedHeader)
	b.WriteString("from dataclasses import dataclass\nfrom enum import StrEnum\nfrom typing import Final\n\n\nclass ErrorCode(StrEnum):\n")
	for _, e := range cat.Errors {
		fmt.Fprintf(&b, "    %s = %q\n", e.Code, e.Code)
	}
	b.WriteString("\n\n@dataclass(frozen=True)\nclass ErrorSpec:\n    status: int\n    retryable: bool\n    message_vi: str\n    message_en: str\n\n\n")
	b.WriteString("CATALOG: Final[dict[ErrorCode, ErrorSpec]] = {\n")
	for _, e := range cat.Errors {
		fmt.Fprintf(&b, "    ErrorCode.%s: ErrorSpec(status=%d, retryable=%s, message_vi=%q, message_en=%q),\n",
			e.Code, e.Status, pyBool(e.Retryable), e.Message.VI, e.Message.EN)
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
