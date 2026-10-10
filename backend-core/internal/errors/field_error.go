package errors

const (
	FieldCodeRequired = "REQUIRED"
	FieldCodeInvalid  = "INVALID"
	FieldCodeTaken    = "TAKEN"

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

// FieldConflict reports a unique value that already exists, in the same details shape as FieldError.
func FieldConflict(field string) *AppError {
	return New(CodeConflict, WithDetails(map[string]any{
		detailsKeyFields: []map[string]string{{fieldKeyName: field, fieldKeyCode: FieldCodeTaken}},
	}))
}
