package errors

const (
	FieldCodeRequired = "REQUIRED"
	FieldCodeInvalid  = "INVALID"

	detailsKeyFields = "fields"
	fieldKeyName     = "field"
	fieldKeyCode     = "code"
)

// FieldError is the validation shape of spec §18.2. It names the field and a reason code but never the submitted
// value, so a secret typed into a form cannot leak back through error details.
func FieldError(field, code string) *AppError {
	return New(CodeValidationFailed, WithDetails(map[string]any{
		detailsKeyFields: []map[string]string{{fieldKeyName: field, fieldKeyCode: code}},
	}))
}
