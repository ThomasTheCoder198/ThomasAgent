package errors

import stderrors "errors"

// These causes are internal control and storage failures. Public boundaries normalize them to catalog errors.
var (
	ErrVaultInvalidMasterKey = stderrors.New("vault: master key must be base64 of exactly 32 bytes")
	ErrVaultDecrypt          = stderrors.New("vault: cannot decrypt secret")
	ErrVaultSecretNotFound   = stderrors.New("vault: secret not found")
	ErrMissingTenant         = stderrors.New("trusted tenant context is missing")
	ErrEventStreamClosed     = stderrors.New("event stream closed")
	ErrEventPayloadMultiLine = stderrors.New("event payload must be a single line")
	ErrEventHeartbeatInvalid = stderrors.New("event stream heartbeat must be positive")
	ErrEventWriteTimeout     = stderrors.New("event stream write timeout must be positive")
	ErrTrailingJSON          = stderrors.New("unexpected data after JSON value")
)
