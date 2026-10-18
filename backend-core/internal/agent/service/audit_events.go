package service

const (
	auditActorOwner   = "owner"
	auditTargetAgent  = "agent"
	auditActionCreate = "agent.create"
	auditActionUpdate = "agent.update"
	auditActionDelete = "agent.delete"

	logKBLookupFailed = "agent kb lookup failed"
	fieldKBID         = "kb_id"

	// overrideFieldSeparator joins modelOverrides and the role in a field error ("modelOverrides.chat.default").
	overrideFieldSeparator = "."
)
