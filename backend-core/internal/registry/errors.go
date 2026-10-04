package registry

import "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"

// fieldError delegates to the shared validation shape (Task 0) so the details format lives in one place.
func fieldError(field, code string) error { return errors.FieldError(field, code) }

func errProviderNotFound() error { return errors.New(errors.CodeRegistryProviderNotFound) }
func errModelNotFound() error    { return errors.New(errors.CodeRegistryModelNotFound) }
func errNameTaken() error        { return errors.New(errors.CodeRegistryNameTaken) }
func errModelInUse() error       { return errors.New(errors.CodeRegistryModelInUse) }
func errRoleNotAssigned(role Role) error {
	return errors.New(errors.CodeRegistryRoleNotAssigned, errors.WithDetails(map[string]any{"role": string(role)}))
}
func errCapabilityMismatch(role Role, needed [][]Capability) error {
	return errors.New(errors.CodeRegistryCapabilityMismatch, errors.WithDetails(map[string]any{"role": string(role), "requires": needed}))
}

func appError(err error) error {
	if err == nil {
		return nil
	}
	return errors.ToAppError(err)
}
