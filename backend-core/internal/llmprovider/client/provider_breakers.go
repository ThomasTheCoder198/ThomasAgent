package client

import (
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
)

// providerBreaker gives each provider its own circuit, so one failing provider cannot block others.
type providerBreaker struct {
	breaker  *retry.Breaker
	active   int
	lastUsed uint64
}

func (c *HTTPModelCatalog) acquireBreaker(providerID uuid.UUID) (*providerBreaker, error) {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	if c.breakers == nil {
		c.breakers = make(map[uuid.UUID]*providerBreaker)
	}
	entry, exists := c.breakers[providerID]
	if !exists {
		if len(c.breakers) >= c.MaxBreakers && !c.discardIdleBreaker() {
			return nil, errors.ErrProviderUnavailable
		}
		entry = &providerBreaker{breaker: c.Breaker.ForProvider("provider-" + providerID.String())}
		c.breakers[providerID] = entry
	}
	c.breakerClock++
	entry.lastUsed = c.breakerClock
	entry.active++
	return entry, nil
}

func (c *HTTPModelCatalog) releaseBreaker(entry *providerBreaker) {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	entry.active--
}

func (c *HTTPModelCatalog) discardIdleBreaker() bool {
	var oldest *providerBreaker
	var oldestID uuid.UUID
	for id, entry := range c.breakers {
		// Resetting a failing or in-flight circuit would bypass provider protection.
		if entry.active == 0 && entry.breaker.CanDiscard() && (oldest == nil || entry.lastUsed < oldest.lastUsed) {
			oldest, oldestID = entry, id
		}
	}
	if oldest == nil {
		return false
	}
	delete(c.breakers, oldestID)
	return true
}
