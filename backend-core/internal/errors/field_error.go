package errors

const (
	FieldCodeRequired = "REQUIRED"
	FieldCodeInvalid  = "INVALID"

	detailsKeyFields = "fields"
	fieldKeyName     = "field"
	fieldKeyCode     = "code"
)

// FieldError describes a field without echoing its submitted value, which may be a secret.
func FieldError(field, code string) *AppError {
	return New(CodeValidationFailed, WithDetails(map[string]any{
		detailsKeyFields: []map[string]string{{fieldKeyName: field, fieldKeyCode: code}},
	}))
}
