package jobs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpanNames_UseCoreServicePrefix(t *testing.T) {
	require.Equal(t, "jobs", jobsTracerName)
	require.Equal(t, []string{
		"core.jobs.ensure_group", "core.jobs.consume", "core.jobs.poll", "core.jobs.handle",
		"core.jobs.enqueue", "core.jobs.outbox_relay", "core.jobs.publish",
	}, []string{ensureGroupSpan, consumeSpan, pollSpan, handleSpan, enqueueOutboxSpan, outboxRelaySpan, publishOutboxSpan})
}
