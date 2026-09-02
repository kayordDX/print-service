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
		"POS_BASE_URL":           "https://pos-api.kayord.com",
		"POS_API_KEY":            "kpos_pk_8f3a91c2.c2VjcmV0c2VjcmV0c2Vjcg",
		"LOG_LEVEL":              "DEBUG",
		"PROBE_INTERVAL_SECONDS": "15",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BaseURL != "https://pos-api.kayord.com" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", cfg.BaseURL)
	}
	if cfg.APIKeys[0].KeyID != "pk_8f3a91c2" {
		t.Errorf("KeyID = %q", cfg.APIKeys[0].KeyID)
	}
	if cfg.APIKeys[0].Bearer() != "kpos_pk_8f3a91c2.c2VjcmV0c2VjcmV0c2Vjcg" {
		t.Errorf("Bearer() = %q", cfg.APIKeys[0].Bearer())
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want lowercased", cfg.LogLevel)
	}
	if cfg.ProbeInterval != 15*time.Second {
		t.Errorf("ProbeInterval = %v, want 15s", cfg.ProbeInterval)
	}
}

func TestLoadAPIKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		env      map[string]string
		wantIDs  []string
		wantErrs []string // substrings; every one must appear in the error
	}{
		{
			name:    "single shorthand",
			env:     map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEY": "kpos_pk_a.b"},
			wantIDs: []string{"pk_a"},
		},
		{
			name:    "single key in list",
			env:     map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEYS": "kpos_pk_a.b"},
			wantIDs: []string{"pk_a"},
		},
		{
			name:    "multiple keys",
			env:     map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEYS": "kpos_pk_a.b,kpos_pk_c.d"},
			wantIDs: []string{"pk_a", "pk_c"},
		},
		{
			name:    "stray whitespace is trimmed",
			env:     map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEYS": "  kpos_pk_a.b ,  kpos_pk_c.d  "},
			wantIDs: []string{"pk_a", "pk_c"},
		},
		{
			name:     "both vars set",
			env:      map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEY": "kpos_pk_a.b", "POS_API_KEYS": "kpos_pk_c.d"},
			wantErrs: []string{"mutually exclusive"},
		},
		{
			name:     "malformed item",
			env:      map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEYS": "kpos_pk_a.b,nonsense"},
			wantErrs: []string{"POS_API_KEYS[1]", `must start with "kpos_"`},
		},
		{
			name:     "empty item",
			env:      map[string]string{"POS_BASE_URL": "https://api", "POS_API_KEYS": "kpos_pk_a.b,"},
			wantErrs: []string{"POS_API_KEYS[1]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := Load(getenvWith(tt.env))
			if len(tt.wantErrs) > 0 {
				if err == nil {
					t.Fatalf("Load() succeeded, want error containing %q", tt.wantErrs)
				}
				for _, want := range tt.wantErrs {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Load() error = %v, want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(cfg.APIKeys) != len(tt.wantIDs) {
				t.Fatalf("len(APIKeys) = %d, want %d", len(cfg.APIKeys), len(tt.wantIDs))
			}
			for i, want := range tt.wantIDs {
				if cfg.APIKeys[i].KeyID != want {
					t.Errorf("APIKeys[%d].KeyID = %q, want %q", i, cfg.APIKeys[i].KeyID, want)
				}
			}
		})
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := Load(getenvWith(map[string]string{
		"POS_API_KEY": "kpos_pk_deadbeef.s3cr3t",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, DefaultBaseURL)
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
			want: "POS_API_KEY",
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
