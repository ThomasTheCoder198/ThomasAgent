package httpx

const (
	HeaderCacheControl  = "Cache-Control"
	CacheControlNoStore = "no-store"

	// Set on proxied app responses: the app's Content-Type is served from core's origin, so it must not be sniffed.
	HeaderContentTypeOptions  = "X-Content-Type-Options"
	ContentTypeOptionsNoSniff = "nosniff"

	HeaderAuthorization = "Authorization"
	AuthSchemeBearer    = "Bearer"

	// Names the app proxy forwards to ragx (spec §14.2); one spelling for httpx and platform/ragx.
	HeaderContentType    = "Content-Type"
	MediaTypeJSON        = "application/json"
	HeaderAcceptLanguage = "Accept-Language"
	HeaderIdempotencyKey = "Idempotency-Key"
)
