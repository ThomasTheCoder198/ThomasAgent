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
	auditActionModelUpdate = "model.update"
	auditActionModelSync   = "model.sync"

	auditTargetRole       = "model_role"
	auditActionRoleAssign = "model_role.assign"
	auditActionRoleClear  = "model_role.clear"

	auditActionOrgRoleAssign = "model_role.assign_org"
	auditActionOrgRoleClear  = "model_role.clear_org"
)
