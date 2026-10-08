package llm

// Repository holds the SQL for providers, models and role assignments. It has no state:
// every method takes the pool or the caller's transaction, and scopes by the tenant in ctx.
type Repository struct{}

func NewRepository() Repository { return Repository{} }
