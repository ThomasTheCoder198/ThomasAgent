package auth

const (
	TracerName       = "core.auth"
	SessionPurgeSpan = "core.auth.session.purge"
	limiterCheckSpan = "core.auth.limiter.check"
	limiterResetSpan = "core.auth.limiter.reset"
	argonCreateSpan  = "core.auth.argon.create"
	argonCompareSpan = "core.auth.argon.compare"
)
