// Package config loads and validates the print service configuration from
// environment variables. There are no config files by design: an unattended
// device is configured through its systemd unit or container environment.
//
// Outlet and device identity are deliberately not configurable; they are
// bound to the API key server-side, so a key can never impersonate another
// outlet's device.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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

	// APIKey authenticates this device against the POS API. It may be
	// zero-valued when KeyFile is set and holds the current key.
	APIKey APIKey

	// KeyFile is an optional file holding the current API key. When set,
	// a key rotated at runtime (pushed by the server) is persisted there,
	// so rotation never requires touching the environment again.
	KeyFile string

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

	rawKey := strings.TrimSpace(getenv("POS_API_KEY"))
	if rawKey != "" {
		key, err := ParseAPIKey(rawKey)
		if err != nil {
			errs = append(errs, fmt.Errorf("POS_API_KEY: %w", err))
		} else {
			cfg.APIKey = key
		}
	}
	cfg.KeyFile = strings.TrimSpace(getenv("POS_API_KEY_FILE"))
	if rawKey == "" && cfg.KeyFile == "" {
		errs = append(errs, fmt.Errorf("POS_API_KEY or POS_API_KEY_FILE is required (create a key in the POS admin UI)"))
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

// LoadKeyFile reads and parses an API key previously persisted by key
// rotation (see SaveKeyFile).
func LoadKeyFile(path string) (APIKey, error) {
	if path == "" {
		return APIKey{}, errors.New("no key file configured")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return APIKey{}, fmt.Errorf("read key file %s: %w", path, err)
	}
	key, err := ParseAPIKey(strings.TrimSpace(string(raw)))
	if err != nil {
		return APIKey{}, fmt.Errorf("key file %s: %w", path, err)
	}
	return key, nil
}

// SaveKeyFile atomically persists the key with 0600 permissions so a
// rotated key survives restarts. It requires a configured file path; a
// device that was started with the key in the environment only cannot
// persist rotations.
func SaveKeyFile(path string, key APIKey) error {
	if path == "" {
		return errors.New("cannot persist rotated key: no key file configured (set POS_API_KEY_FILE)")
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".print-service-key-*")
	if err != nil {
		return fmt.Errorf("create temp key file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after successful rename

	if _, err := tmp.WriteString(key.Bearer() + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write key file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod key file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close key file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace key file %s: %w", path, err)
	}
	return nil
}
