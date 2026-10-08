package service

// Audit vocabulary of this domain; the values are stored in audit_events and must not change.
const (
	auditActorOwner = "owner"

	auditTargetProvider       = "provider"
	auditActionProviderCreate = "provider.create"
	auditActionProviderUpdate = "provider.update"
	auditActionProviderDelete = "provider.delete"

	auditTargetModel       = "model"
	auditActionModelCreate = "model.create"
	auditActionModelDelete = "model.delete"
	auditActionModelSync   = "model.sync"

	auditTargetRole       = "model_role"
	auditActionRoleAssign = "model_role.assign"
)
