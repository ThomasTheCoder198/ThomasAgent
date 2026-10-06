package requestid

import (
	"regexp"

	"github.com/google/uuid"
)

// Values mirror contracts/request-id.json; TestContract_MatchesSharedDefinition fails when they drift.
const (
	Header        = "X-Request-Id"
	Pattern       = `^(req|job)_[A-Za-z0-9-]{8,64}$`
	RequestPrefix = "req_"
	JobPrefix     = "job_"
)

var validID = regexp.MustCompile(Pattern)

// IsValid accepts both prefixes so a background job keeps its id when it calls another service.
func IsValid(id string) bool { return validID.MatchString(id) }

func NewRequest() string { return RequestPrefix + uuid.NewString() }

func NewJob() string { return JobPrefix + uuid.NewString() }
