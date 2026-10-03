package jobs

const (
	jobsTracerName    = "jobs"
	ensureGroupSpan   = "core.jobs.ensure_group"
	consumeSpan       = "core.jobs.consume"
	pollSpan          = "core.jobs.poll"
	handleSpan        = "core.jobs.handle"
	enqueueOutboxSpan = "core.jobs.enqueue"
	outboxRelaySpan   = "core.jobs.outbox_relay"
	publishOutboxSpan = "core.jobs.publish"
)
