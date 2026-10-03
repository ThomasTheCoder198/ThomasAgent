package errorcodegen

import (
	"fmt"
	"strings"
)

func RenderTS(catalog Catalog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s\n\nexport const ErrorCode = {\n", generatedHeader)
	for _, definition := range catalog.Errors {
		fmt.Fprintf(&b, "  %s: %q,\n", definition.Code, definition.Code)
	}
	b.WriteString("} as const;\n\nexport type ErrorCode = (typeof ErrorCode)[keyof typeof ErrorCode];\n\n")
	b.WriteString("export const errorDefinitions: Record<ErrorCode, { httpStatus: number; retryable: boolean; message: { vi: string; en: string } }> = {\n")
	for _, definition := range catalog.Errors {
		fmt.Fprintf(&b, "  %s: { httpStatus: %d, retryable: %t, message: { vi: %q, en: %q } },\n",
			definition.Code, definition.Status, definition.Retryable, definition.Message.VI, definition.Message.EN)
	}
	b.WriteString("};\n")
	return b.String()
}
