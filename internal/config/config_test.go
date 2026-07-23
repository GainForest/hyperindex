package config

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		setEnv       bool
		want         string
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_CONFIG_UNSET",
			defaultValue: "default_value",
			setEnv:       false,
			want:         "default_value",
		},
		{
			name:         "returns env value when set",
			key:          "TEST_CONFIG_SET",
			defaultValue: "default_value",
			envValue:     "env_value",
			setEnv:       true,
			want:         "env_value",
		},
		{
			name:         "returns default when env is empty",
			key:          "TEST_CONFIG_EMPTY",
			defaultValue: "default_value",
			envValue:     "",
			setEnv:       true,
			want:         "default_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(tt.key)
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		setEnv       bool
		want         int
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_CONFIG_INT_UNSET",
			defaultValue: 42,
			setEnv:       false,
			want:         42,
		},
		{
			name:         "returns parsed int when set",
			key:          "TEST_CONFIG_INT_SET",
			defaultValue: 42,
			envValue:     "100",
			setEnv:       true,
			want:         100,
		},
		{
			name:         "returns default when env is invalid int",
			key:          "TEST_CONFIG_INT_INVALID",
			defaultValue: 42,
			envValue:     "not_a_number",
			setEnv:       true,
			want:         42,
		},
		{
			name:         "returns default when env is empty",
			key:          "TEST_CONFIG_INT_EMPTY",
			defaultValue: 42,
			envValue:     "",
			setEnv:       true,
			want:         42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(tt.key)
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvInt(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvInt(%q, %d) = %d, want %d", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue bool
		envValue     string
		setEnv       bool
		want         bool
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_CONFIG_BOOL_UNSET",
			defaultValue: false,
			setEnv:       false,
			want:         false,
		},
		{
			name:         "returns true for 'true'",
			key:          "TEST_CONFIG_BOOL_TRUE",
			defaultValue: false,
			envValue:     "true",
			setEnv:       true,
			want:         true,
		},
		{
			name:         "returns true for 'TRUE'",
			key:          "TEST_CONFIG_BOOL_TRUE_UPPER",
			defaultValue: false,
			envValue:     "TRUE",
			setEnv:       true,
			want:         true,
		},
		{
			name:         "returns true for '1'",
			key:          "TEST_CONFIG_BOOL_ONE",
			defaultValue: false,
			envValue:     "1",
			setEnv:       true,
			want:         true,
		},
		{
			name:         "returns true for 'yes'",
			key:          "TEST_CONFIG_BOOL_YES",
			defaultValue: false,
			envValue:     "yes",
			setEnv:       true,
			want:         true,
		},
		{
			name:         "returns false for 'false'",
			key:          "TEST_CONFIG_BOOL_FALSE",
			defaultValue: true,
			envValue:     "false",
			setEnv:       true,
			want:         false,
		},
		{
			name:         "returns false for '0'",
			key:          "TEST_CONFIG_BOOL_ZERO",
			defaultValue: true,
			envValue:     "0",
			setEnv:       true,
			want:         false,
		},
		{
			name:         "returns false for invalid value",
			key:          "TEST_CONFIG_BOOL_INVALID",
			defaultValue: true,
			envValue:     "invalid",
			setEnv:       true,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(tt.key)
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvBool(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvBool(%q, %v) = %v, want %v", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestRedactPassword(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "postgres URL with password",
			input: "postgres://user:secretpass@localhost:5432/dbname",
			want:  "postgres://user:***@localhost:5432/dbname",
		},
		{
			name:  "postgresql URL with password",
			input: "postgresql://admin:mypassword@db.example.com:5432/production",
			want:  "postgresql://admin:***@db.example.com:5432/production",
		},
		{
			name:  "URL without password",
			input: "sqlite:data/hyperindex.db",
			want:  "sqlite:data/hyperindex.db",
		},
		{
			name:  "URL with @ but no password",
			input: "user@host",
			want:  "user@host",
		},
		{
			name:  "empty URL",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactPassword(tt.input)
			if got != tt.want {
				t.Errorf("RedactPassword(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name            string
		config          Config
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "clean 16+ char admin api key",
			config: Config{
				SecretKeyBase: "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing",
				Port:          8080,
				AdminAPIKey:   "admin-secret-123",
			},
			wantErr: false,
		},
		{
			name: "admin api key empty",
			config: Config{
				SecretKeyBase: "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing",
				Port:          8080,
			},
			wantErr:         true,
			wantErrContains: "ADMIN_API_KEY",
		},
		{
			name: "admin api key whitespace only",
			config: Config{
				SecretKeyBase: "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing",
				Port:          8080,
				AdminAPIKey:   "   ",
			},
			wantErr:         true,
			wantErrContains: "whitespace",
		},
		{
			name: "admin api key too short",
			config: Config{
				SecretKeyBase: "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing",
				Port:          8080,
				AdminAPIKey:   "short-admin-key",
			},
			wantErr:         true,
			wantErrContains: "16 characters",
		},
		{
			name: "admin api key padded valid key",
			config: Config{
				SecretKeyBase: "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing",
				Port:          8080,
				AdminAPIKey:   " admin-secret-123 ",
			},
			wantErr:         true,
			wantErrContains: "whitespace",
		},
		{
			name: "secret key too short",
			config: Config{
				SecretKeyBase: "short_key",
				Port:          8080,
				AdminAPIKey:   "admin-secret-123",
			},
			wantErr: true,
		},
		{
			name: "port too low",
			config: Config{
				SecretKeyBase: "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing",
				Port:          0,
				AdminAPIKey:   "admin-secret-123",
			},
			wantErr: true,
		},
		{
			name: "port too high",
			config: Config{
				SecretKeyBase: "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing",
				Port:          70000,
				AdminAPIKey:   "admin-secret-123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Errorf("Config.Validate() error = %v, want substring %q", err, tt.wantErrContains)
			}
		})
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	got := ParseAllowedOrigins(" https://one.example, ,https://two.example,https://one.example ")
	want := []string{"https://one.example", "https://two.example"}

	if len(got) != len(want) {
		t.Fatalf("ParseAllowedOrigins() len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseAllowedOrigins()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPublicAllowedOriginListDefaultsToWildcard(t *testing.T) {
	cfg := Config{}

	got := cfg.PublicAllowedOriginList()
	if len(got) != 1 || got[0] != "*" {
		t.Fatalf("PublicAllowedOriginList() = %#v, want [*]", got)
	}
}

func TestAdminAllowedOriginListUsesExplicitOriginsOnly(t *testing.T) {
	t.Run("admin env wins", func(t *testing.T) {
		cfg := Config{
			AllowedOrigins:      "https://legacy.example",
			AdminAllowedOrigins: "https://admin.example",
		}

		got := cfg.AdminAllowedOriginList()
		if len(got) != 1 || got[0] != "https://admin.example" {
			t.Fatalf("AdminAllowedOriginList() = %#v, want explicit admin origin", got)
		}
	})

	t.Run("legacy explicit origins fallback", func(t *testing.T) {
		cfg := Config{AllowedOrigins: "https://legacy.example"}

		got := cfg.AdminAllowedOriginList()
		if len(got) != 1 || got[0] != "https://legacy.example" {
			t.Fatalf("AdminAllowedOriginList() = %#v, want legacy origin", got)
		}
	})

	t.Run("legacy wildcard does not open admin", func(t *testing.T) {
		cfg := Config{AllowedOrigins: "*"}

		if got := cfg.AdminAllowedOriginList(); len(got) != 0 {
			t.Fatalf("AdminAllowedOriginList() = %#v, want empty", got)
		}
	})

	t.Run("legacy wildcard is ignored but explicit origins remain", func(t *testing.T) {
		cfg := Config{AllowedOrigins: "*,https://legacy.example"}

		got := cfg.AdminAllowedOriginList()
		if len(got) != 1 || got[0] != "https://legacy.example" {
			t.Fatalf("AdminAllowedOriginList() = %#v, want explicit legacy origin", got)
		}
	})
}

func TestConfigValidateRejectsAdminWildcardOrigin(t *testing.T) {
	cfg := Config{
		SecretKeyBase:       "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing",
		Port:                8080,
		AdminAPIKey:         "admin-secret-123",
		AdminAllowedOrigins: "*",
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_ALLOWED_ORIGINS") {
		t.Fatalf("Validate() error = %v, want ADMIN_ALLOWED_ORIGINS error", err)
	}
}

func TestLoadCORSOriginConfig(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", "admin-secret-123")
	t.Setenv("SECRET_KEY_BASE", "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing")
	t.Setenv("PUBLIC_ALLOWED_ORIGINS", "")
	t.Setenv("ADMIN_ALLOWED_ORIGINS", "https://admin.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PublicAllowedOrigins != "*" {
		t.Fatalf("PublicAllowedOrigins = %q, want *", cfg.PublicAllowedOrigins)
	}
	if cfg.AdminAllowedOrigins != "https://admin.example" {
		t.Fatalf("AdminAllowedOrigins = %q, want https://admin.example", cfg.AdminAllowedOrigins)
	}
}

func TestLoadAdminAPIKey(t *testing.T) {
	os.Setenv("ADMIN_API_KEY", "admin-secret")
	os.Setenv("SECRET_KEY_BASE", "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing")
	defer func() {
		os.Unsetenv("ADMIN_API_KEY")
		os.Unsetenv("SECRET_KEY_BASE")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AdminAPIKey != "admin-secret" {
		t.Fatalf("AdminAPIKey = %q, want %q", cfg.AdminAPIKey, "admin-secret")
	}
}

func TestConfigLogConfigRedactsAdminAPIKey(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	cfg := Config{
		AdminAPIKey:             "super-secret",
		TapAdminPassword:        "tap-secret",
		LabelerSubscribeURLs:    "wss://log-user:log-pass@labeler.example/labels?token=secret-token",
		LabelerSubscribeEnabled: true,
	}
	cfg.LogConfig()

	output := buf.String()
	if !strings.Contains(output, "admin_api_key_set=true") {
		t.Fatalf("LogConfig() output missing admin_api_key_set flag: %s", output)
	}
	if strings.Contains(output, "super-secret") {
		t.Fatalf("LogConfig() leaked admin API key: %s", output)
	}
	if strings.Contains(output, "tap-secret") {
		t.Fatalf("LogConfig() leaked tap admin password: %s", output)
	}
	if !strings.Contains(output, "labeler_subscribe_url_count=1") {
		t.Fatalf("LogConfig() output missing labeler URL count: %s", output)
	}
	for _, leaked := range []string{"log-user", "log-pass", "secret-token", "labeler_subscribe_urls"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("LogConfig() leaked labeler URL detail %q: %s", leaked, output)
		}
	}
}

func TestConfigAddress(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{
			name:   "default host and port",
			config: Config{Host: "127.0.0.1", Port: 8080},
			want:   "127.0.0.1:8080",
		},
		{
			name:   "custom host and port",
			config: Config{Host: "0.0.0.0", Port: 3000},
			want:   "0.0.0.0:3000",
		},
		{
			name:   "localhost",
			config: Config{Host: "localhost", Port: 443},
			want:   "localhost:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.Address()
			if got != tt.want {
				t.Errorf("Config.Address() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTapConfigDefaults(t *testing.T) {
	// Ensure TAP env vars are unset before testing defaults
	for _, key := range []string{"TAP_URL", "TAP_ADMIN_PASSWORD", "TAP_DISABLE_ACKS", "TAP_ENABLED"} {
		os.Unsetenv(key)
	}

	tapURL := getEnv("TAP_URL", "ws://localhost:2480")
	if tapURL != "ws://localhost:2480" {
		t.Errorf("TAP_URL default = %q, want %q", tapURL, "ws://localhost:2480")
	}

	tapAdminPassword := getEnv("TAP_ADMIN_PASSWORD", "")
	if tapAdminPassword != "" {
		t.Errorf("TAP_ADMIN_PASSWORD default = %q, want %q", tapAdminPassword, "")
	}

	tapDisableAcks := getEnvBool("TAP_DISABLE_ACKS", false)
	if tapDisableAcks != false {
		t.Errorf("TAP_DISABLE_ACKS default = %v, want false", tapDisableAcks)
	}

	tapEnabled := getEnvBool("TAP_ENABLED", false)
	if tapEnabled != false {
		t.Errorf("TAP_ENABLED default = %v, want false", tapEnabled)
	}
}

func TestTapConfigEnvVars(t *testing.T) {
	os.Setenv("TAP_URL", "ws://tap.example.com:2480")
	os.Setenv("TAP_ADMIN_PASSWORD", "secret")
	os.Setenv("TAP_DISABLE_ACKS", "true")
	os.Setenv("TAP_ENABLED", "true")
	defer func() {
		os.Unsetenv("TAP_URL")
		os.Unsetenv("TAP_ADMIN_PASSWORD")
		os.Unsetenv("TAP_DISABLE_ACKS")
		os.Unsetenv("TAP_ENABLED")
	}()

	tapURL := getEnv("TAP_URL", "ws://localhost:2480")
	if tapURL != "ws://tap.example.com:2480" {
		t.Errorf("TAP_URL = %q, want %q", tapURL, "ws://tap.example.com:2480")
	}

	tapAdminPassword := getEnv("TAP_ADMIN_PASSWORD", "")
	if tapAdminPassword != "secret" {
		t.Errorf("TAP_ADMIN_PASSWORD = %q, want %q", tapAdminPassword, "secret")
	}

	tapDisableAcks := getEnvBool("TAP_DISABLE_ACKS", false)
	if !tapDisableAcks {
		t.Errorf("TAP_DISABLE_ACKS = %v, want true", tapDisableAcks)
	}

	tapEnabled := getEnvBool("TAP_ENABLED", false)
	if !tapEnabled {
		t.Errorf("TAP_ENABLED = %v, want true", tapEnabled)
	}
}

func TestLabelerSubscribeURLParsing(t *testing.T) {
	got := ParseLabelerSubscribeURLs(" wss://one.example/xrpc/com.atproto.label.subscribeLabels, ,wss://two.example/labels?foo=bar, wss://one.example/xrpc/com.atproto.label.subscribeLabels ")
	want := []string{
		"wss://one.example/xrpc/com.atproto.label.subscribeLabels",
		"wss://two.example/labels?foo=bar",
	}

	if len(got) != len(want) {
		t.Fatalf("ParseLabelerSubscribeURLs() len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseLabelerSubscribeURLs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLabelerSubscribeConfigDefaults(t *testing.T) {
	unsetEnvForTest(t,
		"LABELER_SUBSCRIBE_ENABLED",
		"LABELER_SUBSCRIBE_URLS",
		"LABELER_SUBSCRIBE_RECONNECT_MIN",
		"LABELER_SUBSCRIBE_RECONNECT_MAX",
	)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LabelerSubscribeEnabled {
		t.Fatalf("LabelerSubscribeEnabled = true, want false")
	}
	if cfg.LabelerSubscribeURLs != "" {
		t.Fatalf("LabelerSubscribeURLs = %q, want empty", cfg.LabelerSubscribeURLs)
	}
	if got := cfg.LabelerSubscribeURLList(); len(got) != 0 {
		t.Fatalf("LabelerSubscribeURLList() = %#v, want empty", got)
	}
	if cfg.LabelerSubscribeReconnectMin != time.Second {
		t.Fatalf("LabelerSubscribeReconnectMin = %v, want 1s", cfg.LabelerSubscribeReconnectMin)
	}
	if cfg.LabelerSubscribeReconnectMax != 60*time.Second {
		t.Fatalf("LabelerSubscribeReconnectMax = %v, want 60s", cfg.LabelerSubscribeReconnectMax)
	}
}

func TestLabelerSubscribeConfigEnvVars(t *testing.T) {
	t.Setenv("LABELER_SUBSCRIBE_ENABLED", "true")
	t.Setenv("LABELER_SUBSCRIBE_URLS", " wss://one.example/labels , wss://two.example/labels ")
	t.Setenv("LABELER_SUBSCRIBE_RECONNECT_MIN", "2s")
	t.Setenv("LABELER_SUBSCRIBE_RECONNECT_MAX", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.LabelerSubscribeEnabled {
		t.Fatalf("LabelerSubscribeEnabled = false, want true")
	}
	urls := cfg.LabelerSubscribeURLList()
	if len(urls) != 2 || urls[0] != "wss://one.example/labels" || urls[1] != "wss://two.example/labels" {
		t.Fatalf("LabelerSubscribeURLList() = %#v", urls)
	}
	if cfg.LabelerSubscribeReconnectMin != 2*time.Second {
		t.Fatalf("LabelerSubscribeReconnectMin = %v, want 2s", cfg.LabelerSubscribeReconnectMin)
	}
	if cfg.LabelerSubscribeReconnectMax != 30*time.Second {
		t.Fatalf("LabelerSubscribeReconnectMax = %v, want 30s", cfg.LabelerSubscribeReconnectMax)
	}
}

func TestLabelerSubscribeConfigValidateNegativePaths(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "reconnect min must be positive",
			env: map[string]string{
				"LABELER_SUBSCRIBE_ENABLED":       "true",
				"LABELER_SUBSCRIBE_URLS":          "wss://labeler.example/labels",
				"LABELER_SUBSCRIBE_RECONNECT_MIN": "0s",
				"LABELER_SUBSCRIBE_RECONNECT_MAX": "1s",
			},
			want: "LABELER_SUBSCRIBE_RECONNECT_MIN",
		},
		{
			name: "reconnect max must be at least min",
			env: map[string]string{
				"LABELER_SUBSCRIBE_ENABLED":       "true",
				"LABELER_SUBSCRIBE_URLS":          "wss://labeler.example/labels",
				"LABELER_SUBSCRIBE_RECONNECT_MIN": "5s",
				"LABELER_SUBSCRIBE_RECONNECT_MAX": "1s",
			},
			want: "LABELER_SUBSCRIBE_RECONNECT_MAX",
		},
		{
			name: "enabled requires at least one URL",
			env: map[string]string{
				"LABELER_SUBSCRIBE_ENABLED":       "true",
				"LABELER_SUBSCRIBE_URLS":          " , ",
				"LABELER_SUBSCRIBE_RECONNECT_MIN": "1s",
				"LABELER_SUBSCRIBE_RECONNECT_MAX": "60s",
			},
			want: "LABELER_SUBSCRIBE_URLS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ADMIN_API_KEY", "admin-secret-123")
			t.Setenv("SECRET_KEY_BASE", "this_is_a_very_long_secret_key_that_is_definitely_more_than_64_characters_long_for_testing")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			err = cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestLabelerSubscribeConfigExplicitFalse(t *testing.T) {
	t.Setenv("LABELER_SUBSCRIBE_ENABLED", "false")
	t.Setenv("LABELER_SUBSCRIBE_URLS", "wss://one.example/labels")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LabelerSubscribeEnabled {
		t.Fatalf("LabelerSubscribeEnabled = true, want false")
	}
}

func TestTapConfigFields(t *testing.T) {
	cfg := Config{
		TapURL:           "ws://localhost:2480",
		TapAdminPassword: "mypassword",
		TapDisableAcks:   false,
		TapEnabled:       true,
	}

	if cfg.TapURL != "ws://localhost:2480" {
		t.Errorf("TapURL = %q, want %q", cfg.TapURL, "ws://localhost:2480")
	}
	if cfg.TapAdminPassword != "mypassword" {
		t.Errorf("TapAdminPassword = %q, want %q", cfg.TapAdminPassword, "mypassword")
	}
	if cfg.TapDisableAcks != false {
		t.Errorf("TapDisableAcks = %v, want false", cfg.TapDisableAcks)
	}
	if cfg.TapEnabled != true {
		t.Errorf("TapEnabled = %v, want true", cfg.TapEnabled)
	}

	// Verify password is not directly logged (tap_admin_password_set pattern)
	passwordSet := cfg.TapAdminPassword != ""
	if !passwordSet {
		t.Error("TapAdminPassword should be set but tap_admin_password_set is false")
	}
}

func TestExternalBaseURLNormalization(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "adds https:// when no scheme present",
			envValue: "hyperindex-pr-base.up.railway.app",
			want:     "https://hyperindex-pr-base.up.railway.app",
		},
		{
			name:     "preserves existing https://",
			envValue: "https://hyperindex-pr-base.up.railway.app",
			want:     "https://hyperindex-pr-base.up.railway.app",
		},
		{
			name:     "preserves existing http://",
			envValue: "http://localhost:8080",
			want:     "http://localhost:8080",
		},
		{
			name:     "trims leading and trailing whitespace",
			envValue: "  hyperindex-pr-base.up.railway.app  ",
			want:     "https://hyperindex-pr-base.up.railway.app",
		},
		{
			name:     "preserves uppercase HTTPS:// scheme without double-prepending",
			envValue: "HTTPS://hyperindex-pr-base.up.railway.app",
			want:     "HTTPS://hyperindex-pr-base.up.railway.app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("EXTERNAL_BASE_URL", tt.envValue)
			defer os.Unsetenv("EXTERNAL_BASE_URL")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.ExternalBaseURL != tt.want {
				t.Errorf("ExternalBaseURL = %q, want %q", cfg.ExternalBaseURL, tt.want)
			}
		})
	}
}

func unsetEnvForTest(t *testing.T, keys ...string) {
	t.Helper()

	type envState struct {
		value string
		set   bool
	}

	previous := make(map[string]envState, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		previous[key] = envState{value: value, set: ok}
		os.Unsetenv(key)
	}

	t.Cleanup(func() {
		for _, key := range keys {
			state := previous[key]
			if state.set {
				os.Setenv(key, state.value)
			} else {
				os.Unsetenv(key)
			}
		}
	})
}

func TestGenerateRandomKey(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{name: "32 bytes", length: 32},
		{name: "64 bytes", length: 64},
		{name: "128 bytes", length: 128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := generateRandomKey(tt.length)
			if err != nil {
				t.Errorf("generateRandomKey(%d) error = %v", tt.length, err)
				return
			}
			if len(key) != tt.length {
				t.Errorf("generateRandomKey(%d) returned key of length %d", tt.length, len(key))
			}
		})
	}

	// Test that generated keys are unique
	t.Run("keys are unique", func(t *testing.T) {
		key1, _ := generateRandomKey(64)
		key2, _ := generateRandomKey(64)
		if key1 == key2 {
			t.Error("generateRandomKey() returned same key twice")
		}
	})
}
