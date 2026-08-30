package config

import (
	"strings"
	"testing"
	"time"
)

func getenvWith(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

func TestLoadValid(t *testing.T) {
	t.Parallel()
	cfg, err := Load(getenvWith(map[string]string{
		"POS_BASE_URL":           "https://api.kayord.com/",
		"POS_API_KEY":            "kpos_pk_8f3a91c2.c2VjcmV0c2VjcmV0c2Vjcg",
		"LOG_LEVEL":              "DEBUG",
		"PROBE_INTERVAL_SECONDS": "15",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BaseURL != "https://api.kayord.com" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", cfg.BaseURL)
	}
	if cfg.APIKey.KeyID != "pk_8f3a91c2" {
		t.Errorf("KeyID = %q", cfg.APIKey.KeyID)
	}
	if cfg.APIKey.Bearer() != "kpos_pk_8f3a91c2.c2VjcmV0c2VjcmV0c2Vjcg" {
		t.Errorf("Bearer() = %q", cfg.APIKey.Bearer())
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want lowercased", cfg.LogLevel)
	}
	if cfg.ProbeInterval != 15*time.Second {
		t.Errorf("ProbeInterval = %v, want 15s", cfg.ProbeInterval)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := Load(getenvWith(map[string]string{
		"POS_BASE_URL": "http://localhost:5117",
		"POS_API_KEY":  "kpos_pk_deadbeef.s3cr3t",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.ProbeInterval != DefaultProbeInterval {
		t.Errorf("ProbeInterval = %v, want %v", cfg.ProbeInterval, DefaultProbeInterval)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  map[string]string
		want string // substring of the error
	}{
		{
			name: "missing everything",
			env:  map[string]string{},
			want: "POS_BASE_URL",
		},
		{
			name: "bad url",
			env:  map[string]string{"POS_BASE_URL": "ftp://x", "POS_API_KEY": "kpos_pk_a.b"},
			want: "not a valid http(s) URL",
		},
		{
			name: "key without prefix",
			env:  map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEY": "nope.pk_a.b"},
			want: `must start with "kpos_"`,
		},
		{
			name: "missing key",
			env:  map[string]string{"POS_BASE_URL": "https://api"},
			want: "POS_API_KEY is required",
		},
		{
			name: "key without secret",
			env:  map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEY": "kpos_pk_a."},
			want: "non-empty key id and secret",
		},
		{
			name: "bad log level",
			env:  map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEY": "kpos_pk_a.b", "LOG_LEVEL": "loud"},
			want: "LOG_LEVEL",
		},
		{
			name: "bad probe interval",
			env:  map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEY": "kpos_pk_a.b", "PROBE_INTERVAL_SECONDS": "0"},
			want: "PROBE_INTERVAL_SECONDS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(getenvWith(tt.env))
			if err == nil {
				t.Fatalf("Load() succeeded, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestParseAPIKey(t *testing.T) {
	t.Parallel()
	key, err := ParseAPIKey("kpos_pk_8f3a91c2.abc")
	if err != nil {
		t.Fatalf("ParseAPIKey() error = %v", err)
	}
	if key.KeyID != "pk_8f3a91c2" || key.Secret != "abc" {
		t.Errorf("ParseAPIKey() = %+v", key)
	}
	if got := key.String(); strings.Contains(got, "abc") {
		// The secret must never survive into the log-safe form.
		t.Errorf("String() leaks the secret: %q", got)
	}
}

func TestAPIKeyStringMasksSecret(t *testing.T) {
	t.Parallel()
	key := APIKey{KeyID: "pk_x", Secret: "supersecretvalue"}
	if got, want := key.String(), "kpos_pk_x.****alue"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
