package config

import (
	_ "embed"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
)

type RetryConfig struct {
	MaxAttempts int           `env:"MAX_ATTEMPTS"`
	BaseDelay   time.Duration `env:"BASE_DELAY"`
	MaxDelay    time.Duration `env:"MAX_DELAY"`
}

type BreakerConfig struct {
	FailureThreshold uint32        `env:"FAILURE_THRESHOLD"`
	OpenTimeout      time.Duration `env:"OPEN_TIMEOUT"`
	HalfOpenMaxCalls uint32        `env:"HALF_OPEN_MAX_CALLS"`
}

type ConsumerConfig struct {
	MaxDeliveries     int           `env:"MAX_DELIVERIES"`
	VisibilityTimeout time.Duration `env:"VISIBILITY_TIMEOUT"`
	BlockTimeout      time.Duration `env:"BLOCK_TIMEOUT"`
	BatchSize         int64         `env:"BATCH_SIZE"`
}

type OutboxRelayConfig struct {
	BatchSize    int           `env:"BATCH_SIZE"`
	PollInterval time.Duration `env:"POLL_INTERVAL"`
}

type HTTPConfig struct {
	MaxBodyBytes            int64         `env:"MAX_BODY_BYTES"`
	RequestTimeout          time.Duration `env:"REQUEST_TIMEOUT"`
	EventStreamHeartbeat    time.Duration `env:"EVENT_STREAM_HEARTBEAT"`
	EventStreamWriteTimeout time.Duration `env:"EVENT_STREAM_WRITE_TIMEOUT"`
}

type MetricsConfig struct {
	Addr        string    `env:"ADDR"`
	HTTPBuckets []float64 `env:"HTTP_BUCKETS" envSeparator:","`
	LLMBuckets  []float64 `env:"LLM_BUCKETS" envSeparator:","`
}

type LLMLogConfig struct {
	Content         bool `env:"CONTENT"`
	ContentMaxBytes int  `env:"CONTENT_MAX_BYTES"`
}

func (c MetricsConfig) validate() []error {
	var errs []error
	if !ascendingPositive(c.HTTPBuckets) {
		errs = append(errs, errors.New("CORE_METRICS_HTTP_BUCKETS must be finite, positive and strictly ascending"))
	}
	if !ascendingPositive(c.LLMBuckets) {
		errs = append(errs, errors.New("CORE_METRICS_LLM_BUCKETS must be finite, positive and strictly ascending"))
	}
	return errs
}

func ascendingPositive(buckets []float64) bool {
	if len(buckets) == 0 {
		return false
	}
	for index, bucket := range buckets {
		if math.IsNaN(bucket) || math.IsInf(bucket, 0) || bucket <= 0 || (index > 0 && bucket <= buckets[index-1]) {
			return false
		}
	}
	return true
}

func (c HTTPConfig) validate() []error {
	var errs []error
	if c.MaxBodyBytes < 1 {
		errs = append(errs, errors.New("CORE_HTTP_MAX_BODY_BYTES must be >= 1"))
	}
	if c.RequestTimeout <= 0 {
		errs = append(errs, errors.New("CORE_HTTP_REQUEST_TIMEOUT must be > 0"))
	}
	if c.EventStreamHeartbeat <= 0 {
		errs = append(errs, errors.New("CORE_HTTP_EVENT_STREAM_HEARTBEAT must be > 0"))
	}
	if c.EventStreamWriteTimeout <= 0 {
		errs = append(errs, errors.New("CORE_HTTP_EVENT_STREAM_WRITE_TIMEOUT must be > 0"))
	}
	return errs
}

type AppConfig struct {
	SigningKey     string        `env:"SIGNING_KEY"`
	SigningKeyFile string        `env:"SIGNING_KEY_FILE"`
	SigningKeyID   string        `env:"SIGNING_KEY_ID"`
	TokenIssuer    string        `env:"TOKEN_ISSUER"`
	TokenAudience  string        `env:"TOKEN_AUDIENCE"`
	TokenTTL       time.Duration `env:"TOKEN_TTL"`
}

// MaxAppTokenTTL is the spec's upper bound (§3.1); ragx rejects longer tokens.
const (
	MaxAppTokenTTL = 60 * time.Second
	// MinAppTokenTTL is one second because exp is written in whole seconds.
	MinAppTokenTTL = time.Second
)

// The key itself is checked when serve or worker start, so migrate and outbox-relay run without it.
func (c AppConfig) validate() []error {
	var errs []error
	if c.SigningKey != "" && c.SigningKeyFile != "" {
		errs = append(errs, errors.New("set only one of CORE_APP_SIGNING_KEY and CORE_APP_SIGNING_KEY_FILE"))
	}
	if c.SigningKeyID == "" || c.TokenIssuer == "" || c.TokenAudience == "" {
		errs = append(errs, errors.New("CORE_APP_SIGNING_KEY_ID, CORE_APP_TOKEN_ISSUER and CORE_APP_TOKEN_AUDIENCE must be set"))
	}
	// The signer writes exp in whole seconds, so a fraction would be silently truncated.
	if c.TokenTTL < MinAppTokenTTL || c.TokenTTL > MaxAppTokenTTL || c.TokenTTL%time.Second != 0 {
		errs = append(errs, fmt.Errorf("CORE_APP_TOKEN_TTL must be a whole number of seconds, >= %ds and <= %ds",
			int(MinAppTokenTTL/time.Second), int(MaxAppTokenTTL/time.Second)))
	}
	return errs
}

// RagxConfig is how core reaches ragx. Typed calls are bounded by RequestTimeout; the app proxy's forward uses the
// HTTP route's deadline and caps the response body at MaxResponseBytes. IdleConnTimeout must stay below ragx's
// keep-alive, so core drops an idle connection before ragx closes it (ruling F22 reversed, R-EOF).
type RagxConfig struct {
	URL                   string        `env:"URL"`
	ConnectTimeout        time.Duration `env:"CONNECT_TIMEOUT"`
	RequestTimeout        time.Duration `env:"REQUEST_TIMEOUT"`
	ResponseHeaderTimeout time.Duration `env:"RESPONSE_HEADER_TIMEOUT"`
	IdleConnTimeout       time.Duration `env:"IDLE_CONN_TIMEOUT"`
	MaxErrorBodyBytes     int64         `env:"MAX_ERROR_BODY_BYTES"`
	MaxResponseBytes      int64         `env:"MAX_RESPONSE_BYTES"`
	ProvisionAttempts     int           `env:"PROVISION_ATTEMPTS"`
}

func (c RagxConfig) validate() []error {
	var errs []error
	parsed, err := url.Parse(c.URL)
	if err != nil || (parsed.Scheme != schemeHTTP && parsed.Scheme != schemeHTTPS) || parsed.Host == "" {
		errs = append(errs, errors.New("CORE_RAGX_URL must be an absolute http(s) URL"))
	}
	return append(errs, c.validateLimits()...)
}

func (c RagxConfig) validateLimits() []error {
	var errs []error
	if c.ConnectTimeout <= 0 || c.RequestTimeout <= 0 || c.ResponseHeaderTimeout <= 0 || c.IdleConnTimeout <= 0 {
		errs = append(errs, errors.New("CORE_RAGX_*_TIMEOUT values must be > 0"))
	}
	if c.MaxErrorBodyBytes < 1 || c.MaxResponseBytes < 1 || c.ProvisionAttempts < 1 {
		errs = append(errs, errors.New("CORE_RAGX_MAX_ERROR_BODY_BYTES, CORE_RAGX_MAX_RESPONSE_BYTES and CORE_RAGX_PROVISION_ATTEMPTS must be >= 1"))
	}
	return errs
}

// validateProvisionBudget keeps synchronous provisioning inside one HTTP request (spec §14.4): every attempt may
// take the full ragx timeout, and every retry may wait the full backoff.
func (c Config) validateProvisionBudget() []error {
	attempts := time.Duration(c.Ragx.ProvisionAttempts)
	if attempts < 1 {
		return nil // validateLimits reports it
	}
	budget := attempts*c.Ragx.RequestTimeout + (attempts-1)*c.Retry.MaxDelay
	if budget >= c.HTTP.RequestTimeout {
		return []error{errors.New("CORE_RAGX_PROVISION_ATTEMPTS × CORE_RAGX_REQUEST_TIMEOUT plus CORE_RETRY_MAX_DELAY backoff must stay below CORE_HTTP_REQUEST_TIMEOUT")}
	}
	return nil
}

type VaultConfig struct {
	MasterKey string `env:"MASTER_KEY,required,notEmpty"`
	KeyID     string `env:"KEY_ID"`
}

type AuthConfig struct {
	OwnerEmail            string             `env:"OWNER_EMAIL"`
	OwnerPassword         string             `env:"OWNER_PASSWORD"`
	SessionTTL            time.Duration      `env:"SESSION_TTL"`
	CookieSecure          bool               `env:"COOKIE_SECURE"`
	LoginMaxAttempts      int                `env:"LOGIN_MAX_ATTEMPTS"`
	LoginEmailMaxAttempts int                `env:"LOGIN_EMAIL_MAX_ATTEMPTS"`
	LoginIPMaxAttempts    int                `env:"LOGIN_IP_MAX_ATTEMPTS"`
	LoginWindow           time.Duration      `env:"LOGIN_WINDOW"`
	LoginLimiterKey       string             `env:"LOGIN_LIMITER_KEY,required,notEmpty"`
	MinPasswordLength     int                `env:"MIN_PASSWORD_LENGTH"`
	TrustedProxyHeader    string             `env:"TRUSTED_PROXY_HEADER"`
	TrustedProxyCIDRs     []string           `env:"TRUSTED_PROXY_CIDRS" envSeparator:","`
	SessionPurgeInterval  time.Duration      `env:"SESSION_PURGE_INTERVAL"`
	SessionTouchInterval  time.Duration      `env:"SESSION_TOUCH_INTERVAL"`
	PasswordHash          PasswordHashConfig `envPrefix:"PASSWORD_HASH_"`
}

type PasswordHashConfig struct {
	Iterations     uint32 `env:"ITERATIONS"`
	MemoryKiB      uint32 `env:"MEMORY_KIB"`
	MaxConcurrency int    `env:"MAX_CONCURRENCY"`
	Parallelism    uint8  `env:"PARALLELISM"`
	SaltLength     uint32 `env:"SALT_LENGTH"`
	KeyLength      uint32 `env:"KEY_LENGTH"`
}

func (c AuthConfig) validate() []error {
	errs := c.validateSessionAndLogin()
	errs = append(errs, c.PasswordHash.validate()...)
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			errs = append(errs, errors.New("CORE_AUTH_TRUSTED_PROXY_CIDRS must contain valid CIDRs"))
		}
	}
	return errs
}

func (c AuthConfig) validateSessionAndLogin() []error {
	var errs []error
	if c.SessionTouchInterval <= 0 {
		errs = append(errs, errors.New("CORE_AUTH_SESSION_TOUCH_INTERVAL must be > 0"))
	}
	if c.LoginEmailMaxAttempts < 1 {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_EMAIL_MAX_ATTEMPTS must be >= 1"))
	}
	if c.LoginIPMaxAttempts < 1 {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS must be >= 1"))
	}
	if c.SessionTTL <= 0 {
		errs = append(errs, errors.New("CORE_AUTH_SESSION_TTL must be > 0"))
	}
	if c.SessionPurgeInterval <= 0 {
		errs = append(errs, errors.New("CORE_AUTH_SESSION_PURGE_INTERVAL must be > 0"))
	}
	if c.MinPasswordLength < 1 {
		errs = append(errs, errors.New("CORE_AUTH_MIN_PASSWORD_LENGTH must be >= 1"))
	}
	if c.LoginWindow <= 0 {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_WINDOW must be > 0"))
	}
	if c.LoginMaxAttempts < 1 {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_MAX_ATTEMPTS must be >= 1"))
	}
	return errs
}

func (c PasswordHashConfig) validate() []error {
	var errs []error
	if c.Parallelism < 1 {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_PARALLELISM must be >= 1"))
	}
	if c.SaltLength < MinimumArgonSaltLength {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_SALT_LENGTH must be >= 8"))
	}
	if c.KeyLength < MinimumArgonKeyLength {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_KEY_LENGTH must be >= 16"))
	}
	if c.Iterations < MinimumArgonIterations {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_ITERATIONS must be >= 2"))
	}
	if c.MemoryKiB != ArgonMemoryKiB {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_MEMORY_KIB must equal 65536"))
	}
	if c.MaxConcurrency < 1 {
		errs = append(errs, errors.New("CORE_AUTH_PASSWORD_HASH_MAX_CONCURRENCY must be >= 1"))
	}
	return errs
}

type Config struct {
	ProviderMaxRemoteModels  int               `env:"PROVIDER_MAX_REMOTE_MODELS"`
	ProviderMaxBreakers      int               `env:"PROVIDER_MAX_BREAKERS"`
	ProviderHTTPTimeout      time.Duration     `env:"PROVIDER_HTTP_TIMEOUT"`
	ProviderBreaker          BreakerConfig     `envPrefix:"PROVIDER_BREAKER_"`
	ProviderMaxResponseBytes int64             `env:"PROVIDER_MAX_RESPONSE_BYTES"`
	ProviderPrivateAllowlist []string          `env:"PROVIDER_PRIVATE_ALLOWLIST" envSeparator:","`
	Auth                     AuthConfig        `envPrefix:"AUTH_"`
	Vault                    VaultConfig       `envPrefix:"VAULT_"`
	App                      AppConfig         `envPrefix:"APP_"`
	Ragx                     RagxConfig        `envPrefix:"RAGX_"`
	HTTP                     HTTPConfig        `envPrefix:"HTTP_"`
	Metrics                  MetricsConfig     `envPrefix:"METRICS_"`
	LLMLog                   LLMLogConfig      `envPrefix:"LLM_LOG_"`
	Environment              string            `env:"ENV"`
	ServiceName              string            `env:"SERVICE_NAME"`
	HTTPAddr                 string            `env:"HTTP_ADDR"`
	ShutdownTimeout          time.Duration     `env:"SHUTDOWN_TIMEOUT"`
	ReadHeaderTimeout        time.Duration     `env:"READ_HEADER_TIMEOUT"`
	DatabaseURL              string            `env:"DATABASE_URL,required,notEmpty"`
	RedisURL                 string            `env:"REDIS_URL,required,notEmpty"`
	LogsOTLPEndpoint         string            `env:"LOGS_OTLP_ENDPOINT"`
	LogLevel                 string            `env:"LOG_LEVEL"`
	Retry                    RetryConfig       `envPrefix:"RETRY_"`
	Stream                   ConsumerConfig    `envPrefix:"STREAM_"`
	OutboxRelay              OutboxRelayConfig `envPrefix:"RELAY_"`
}

const envPrefix = "CORE_"

const (
	metricsAddrEnv = envPrefix + "METRICS_ADDR"
	envFileEnv     = envPrefix + "ENV_FILE"
	defaultEnvFile = ".env"
)

//go:embed config.yaml
var defaultConfigYAML []byte

const DefaultProviderMaxResponseBytes int64 = 8388608

const (
	MinimumArgonIterations         uint32 = 2
	ArgonMemoryKiB                 uint32 = 65536
	DefaultArgonConcurrency               = 4
	MinimumSecretLength                   = 32
	DefaultLoginEmailMaxAttempts          = 20
	DefaultLoginIPMaxAttempts             = 100
	DefaultArgonParallelism        uint8  = 1
	DefaultArgonSaltLength         uint32 = 16
	DefaultArgonKeyLength          uint32 = 32
	MinimumArgonSaltLength         uint32 = 8
	MinimumArgonKeyLength          uint32 = 16
	DefaultSessionTouchInterval           = 5 * time.Minute
	DefaultProviderMaxRemoteModels        = 1000
	DefaultProviderMaxBreakers            = 256
)

func Load() (Config, error) {
	values, err := loadEnvironment()
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	err = env.ParseWithOptions(&cfg, env.Options{Prefix: envPrefix, Environment: values})
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadEnvironment keeps defaults, local dotenv overrides and process overrides separate from global process state.
func loadEnvironment() (map[string]string, error) {
	values := make(map[string]string)
	if err := yaml.Unmarshal(defaultConfigYAML, &values); err != nil {
		return nil, fmt.Errorf("load config.yaml: %w", err)
	}
	file := defaultEnvFile
	if configured, ok := os.LookupEnv(envFileEnv); ok {
		file = configured
	}
	if file != "" {
		fromFile, err := godotenv.Read(file)
		if err != nil && (!os.IsNotExist(err) || file != defaultEnvFile) {
			return nil, fmt.Errorf("load dotenv: %w", err)
		}
		for key, value := range fromFile {
			mergeOverride(values, key, value)
		}
	}
	for _, assignment := range os.Environ() {
		key, value, ok := strings.Cut(assignment, "=")
		if ok {
			mergeOverride(values, key, value)
		}
	}
	return values, nil
}

func mergeOverride(values map[string]string, key, value string) {
	// Empty retains a configured default, except the metrics listener supports an explicit off switch.
	if value == "" && key != metricsAddrEnv && values[key] != "" {
		return
	}
	values[key] = value
}

func (c Config) validate() error {
	var errs []error
	if _, err := logging.ParseLevel(c.LogLevel); err != nil {
		errs = append(errs, fmt.Errorf("CORE_LOG_LEVEL: %w", err))
	}
	if c.Retry.MaxDelay < c.Retry.BaseDelay {
		errs = append(errs, errors.New("CORE_RETRY_MAX_DELAY must be >= CORE_RETRY_BASE_DELAY"))
	}
	if c.Retry.MaxAttempts < 1 || c.Stream.MaxDeliveries < 1 {
		errs = append(errs, errors.New("CORE_RETRY_MAX_ATTEMPTS and CORE_STREAM_MAX_DELIVERIES must be >= 1"))
	}
	errs = append(errs, c.HTTP.validate()...)
	errs = append(errs, c.Metrics.validate()...)
	if c.LLMLog.ContentMaxBytes < 1 {
		errs = append(errs, errors.New("CORE_LLM_LOG_CONTENT_MAX_BYTES must be >= 1"))
	}
	errs = append(errs, c.Auth.validate()...)
	errs = append(errs, c.App.validate()...)
	errs = append(errs, c.Ragx.validate()...)
	errs = append(errs, c.validateProvisionBudget()...)
	errs = append(errs, c.validateLoginLimiterKey()...)
	errs = append(errs, c.validateProvider()...)
	errs = append(errs, c.validateLogsEndpoint()...)
	return errors.Join(errs...)
}

// validateLoginLimiterKey rejects HMAC keys short enough to brute-force the hashed limiter identities.
func (c Config) validateLoginLimiterKey() []error {
	var errs []error
	key := c.Auth.LoginLimiterKey
	if len(key) < MinimumSecretLength {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_LIMITER_KEY must be at least 32 characters"))
	}
	if !strings.EqualFold(c.Environment, "dev") && strings.HasPrefix(strings.ToLower(key), "change-me") {
		errs = append(errs, errors.New("CORE_AUTH_LOGIN_LIMITER_KEY must not use a change-me value outside dev"))
	}
	return errs
}

func (c Config) validateProvider() []error {
	var errs []error
	if c.ProviderMaxRemoteModels < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_MAX_REMOTE_MODELS must be >= 1"))
	}
	if c.ProviderMaxBreakers < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_MAX_BREAKERS must be >= 1"))
	}
	if c.ProviderHTTPTimeout <= 0 {
		errs = append(errs, errors.New("CORE_PROVIDER_HTTP_TIMEOUT must be > 0"))
	}
	if c.ProviderMaxResponseBytes < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_MAX_RESPONSE_BYTES must be >= 1"))
	}
	if c.ProviderBreaker.FailureThreshold < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_BREAKER_FAILURE_THRESHOLD must be >= 1"))
	}
	if c.ProviderBreaker.HalfOpenMaxCalls < 1 {
		errs = append(errs, errors.New("CORE_PROVIDER_BREAKER_HALF_OPEN_MAX_CALLS must be >= 1"))
	}
	if c.ProviderBreaker.OpenTimeout <= 0 {
		errs = append(errs, errors.New("CORE_PROVIDER_BREAKER_OPEN_TIMEOUT must be > 0"))
	}
	if err := outbound.ValidatePrivateAllowlist(c.ProviderPrivateAllowlist); err != nil {
		errs = append(errs, fmt.Errorf("CORE_PROVIDER_PRIVATE_ALLOWLIST: %w", err))
	}
	return errs
}

const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
	rootPath    = "/"
)

// validateLogsEndpoint wants the complete logs URL: a bare collector base URL would silently post to the wrong path.
func (c Config) validateLogsEndpoint() []error {
	if c.LogsOTLPEndpoint == "" {
		return nil
	}
	parsed, err := url.Parse(c.LogsOTLPEndpoint)
	if err != nil || (parsed.Scheme != schemeHTTP && parsed.Scheme != schemeHTTPS) || parsed.Host == "" ||
		parsed.Path == "" || parsed.Path == rootPath {
		return []error{errors.New("CORE_LOGS_OTLP_ENDPOINT must be a full http(s) URL including the logs path")}
	}
	return nil
}
