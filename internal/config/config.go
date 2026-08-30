// Package config loads and validates the print service configuration from
// environment variables. There are no config files by design: an unattended
// device is configured through its systemd unit or container environment.
//
// Outlet and device identity are deliberately not configurable; they are
// bound to the API key server-side, so a key can never impersonate another
// outlet's device.
package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// apiKeyPrefix is the fixed prefix of every print service key.
const apiKeyPrefix = "kpos_"

// APIKey is a print service API key in the wire format
// "kpos_{keyId}.{secret}", created by an outlet manager in the POS admin UI.
type APIKey struct {
	// KeyID is the public part of the key (e.g. "pk_8f3a91c2").
	// It is safe to log and is what operators see in the admin UI.
	KeyID string
	// Secret is the private part of the key. It must never be logged.
	Secret string
}

// ParseAPIKey parses "kpos_{keyId}.{secret}" into its public and private
// parts. The raw key is only ever shown once by the admin UI, so a malformed
// key is almost always a copy/paste mistake; the error should say so.
func ParseAPIKey(raw string) (APIKey, error) {
	if !strings.HasPrefix(raw, apiKeyPrefix) {
		return APIKey{}, fmt.Errorf("API key must start with %q", apiKeyPrefix)
	}
	keyID, secret, ok := strings.Cut(strings.TrimPrefix(raw, apiKeyPrefix), ".")
	if !ok || keyID == "" || secret == "" {
		return APIKey{}, fmt.Errorf("API key must have the form %q with a non-empty key id and secret", apiKeyPrefix+"{keyId}.{secret}")
	}
	return APIKey{KeyID: keyID, Secret: secret}, nil
}

// Bearer returns the full key in its wire format, ready to be sent as
// "Authorization: Bearer <key>". Callers must never log the result.
func (k APIKey) Bearer() string {
	return apiKeyPrefix + k.KeyID + "." + k.Secret
}

// String returns a masked representation that is safe for logs.
func (k APIKey) String() string {
	return apiKeyPrefix + k.KeyID + "." + mask(k.Secret)
}

func mask(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}

// Config is the validated service configuration.
type Config struct {
	// BaseURL is the POS API base URL without a trailing slash,
	// e.g. "https://api.kayord.com".
	BaseURL string

	// APIKeys authenticates the app instances against the POS API, one per
	// outlet the process serves. Each key binds an instance to its outlet
	// server-side.
	APIKeys []APIKey

	// LogLevel is one of "debug", "info", "warn" or "error".
	LogLevel string

	// ProbeInterval is how often printer reachability probes run.
	ProbeInterval time.Duration
}

// DefaultProbeInterval is used when PROBE_INTERVAL_SECONDS is unset.
const DefaultProbeInterval = 30 * time.Second

// Load reads the configuration from getenv (os.Getenv in production).
// It collects every problem it finds so the operator can fix them in one
// restart instead of playing whack-a-mole.
func Load(getenv func(string) string) (Config, error) {
	var errs []error

	cfg := Config{ProbeInterval: DefaultProbeInterval}

	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(getenv("POS_BASE_URL")), "/")
	if cfg.BaseURL == "" {
		errs = append(errs, fmt.Errorf("POS_BASE_URL is required (e.g. https://api.kayord.com)"))
	} else if u, err := url.Parse(cfg.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("POS_BASE_URL %q is not a valid http(s) URL", cfg.BaseURL))
	}

	keys, err := parseAPIKeys(strings.TrimSpace(getenv("POS_API_KEY")), strings.TrimSpace(getenv("POS_API_KEYS")))
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.APIKeys = keys
	}

	cfg.LogLevel = strings.ToLower(strings.TrimSpace(getenv("LOG_LEVEL")))
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL %q is invalid: want debug, info, warn or error", cfg.LogLevel))
	}

	if raw := strings.TrimSpace(getenv("PROBE_INTERVAL_SECONDS")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			errs = append(errs, fmt.Errorf("PROBE_INTERVAL_SECONDS %q is invalid: want a positive integer", raw))
		} else {
			cfg.ProbeInterval = time.Duration(seconds) * time.Second
		}
	}

	if err := joinErrs(errs); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// parseAPIKeys resolves the configured keys. POS_API_KEYS holds a
// comma-separated list of keys (one per outlet served by this process);
// POS_API_KEY is the single-key shorthand. Setting both is ambiguous and
// rejected, as is a malformed key anywhere in the list.
func parseAPIKeys(single, list string) ([]APIKey, error) {
	switch {
	case single != "" && list != "":
		return nil, fmt.Errorf("POS_API_KEY and POS_API_KEYS are mutually exclusive: set only one")
	case single != "":
		key, err := ParseAPIKey(single)
		if err != nil {
			return nil, fmt.Errorf("POS_API_KEY: %w", err)
		}
		return []APIKey{key}, nil
	case list != "":
		items := strings.Split(list, ",")
		keys := make([]APIKey, 0, len(items))
		for i, item := range items {
			key, err := ParseAPIKey(strings.TrimSpace(item))
			if err != nil {
				return nil, fmt.Errorf("POS_API_KEYS[%d]: %w", i, err)
			}
			keys = append(keys, key)
		}
		return keys, nil
	default:
		return nil, fmt.Errorf("POS_API_KEY is required (or POS_API_KEYS for multiple outlets; create keys in the POS admin UI)")
	}
}

func joinErrs(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	msgs := make([]string, len(errs))
	for i, err := range errs {
		msgs[i] = "- " + err.Error()
	}
	return fmt.Errorf("invalid configuration:\n%s", strings.Join(msgs, "\n"))
}
