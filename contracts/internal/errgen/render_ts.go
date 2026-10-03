package errgen

import (
	"fmt"
	"strings"
)

func RenderTS(cat Catalog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s\n\nexport const ErrorCode = {\n", generatedHeader)
	for _, e := range cat.Errors {
		fmt.Fprintf(&b, "  %s: %q,\n", e.Code, e.Code)
	}
	b.WriteString("} as const;\n\nexport type ErrorCode = (typeof ErrorCode)[keyof typeof ErrorCode];\n\n")
	b.WriteString("export const errorCatalog: Record<ErrorCode, { status: number; retryable: boolean; message: { vi: string; en: string } }> = {\n")
	for _, e := range cat.Errors {
		fmt.Fprintf(&b, "  %s: { status: %d, retryable: %t, message: { vi: %q, en: %q } },\n",
			e.Code, e.Status, e.Retryable, e.Message.VI, e.Message.EN)
	}
	b.WriteString("};\n")
	return b.String()
}
