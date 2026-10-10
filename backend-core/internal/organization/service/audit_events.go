package service

// Audit vocabulary of this domain; the values are stored in audit_events and must not change.
const (
	auditActorOwner         = "owner"
	auditTargetOrganization = "organization"
	auditActionCreate       = "organization.create"
	auditActionUpdate       = "organization.update"
	auditActionProvision    = "organization.provision"
)
