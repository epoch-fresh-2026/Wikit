package config

import (
	"strings"
	"testing"
)

func TestConfiguration(t *testing.T) {
	base := map[string]string{"WIKIT_DATABASE_URL": "postgres://user:secret@localhost/wikit", "WIKIT_PUBLIC_URL": "http://localhost:8080", "WIKIT_MEILI_URL": "http://localhost:7700", "WIKIT_MEILI_KEY": "local_search_test_key_2026"}
	for _, tc := range []struct {
		name, key, value string
		valid            bool
	}{
		{"defaults", "", "", true},
		{"missing search URL", "WIKIT_MEILI_URL", "", false},
		{"search credentials", "WIKIT_MEILI_URL", "http://user:secret@localhost:7700", false},
		{"search query", "WIKIT_MEILI_URL", "http://localhost:7700/?secret=1", false},
		{"search path", "WIKIT_MEILI_URL", "http://localhost:7700/indexes", false},
		{"search invalid port", "WIKIT_MEILI_URL", "http://localhost:99999", false},
		{"missing search key", "WIKIT_MEILI_KEY", "", false},
		{"invalid search key", "WIKIT_MEILI_KEY", "local_key_with_line\nbreak", false},
		{"https", "WIKIT_PUBLIC_URL", "https://wiki.example", true},
		{"loopback", "WIKIT_PUBLIC_URL", "http://[::1]:8080", true},
		{"missing database", "WIKIT_DATABASE_URL", "", false},
		{"bad database", "WIKIT_DATABASE_URL", "postgres://user:secret@%zz/db", false},
		{"no database name", "WIKIT_DATABASE_URL", "postgres://localhost/", false},
		{"public http", "WIKIT_PUBLIC_URL", "http://wiki.example", false},
		{"invalid public port", "WIKIT_PUBLIC_URL", "https://wiki.example:65536", false},
		{"credentials", "WIKIT_PUBLIC_URL", "https://user:secret@wiki.example", false},
		{"path", "WIKIT_PUBLIC_URL", "https://wiki.example/path", false},
		{"query", "WIKIT_PUBLIC_URL", "https://wiki.example/?secret=1", false},
		{"fragment", "WIKIT_PUBLIC_URL", "https://wiki.example/#secret", false},
		{"invalid listen", "WIKIT_LISTEN_ADDR", ":70000", false},
		{"port zero", "WIKIT_LISTEN_ADDR", ":0", false},
		{"registration off", "WIKIT_REGISTRATION_ENABLED", "false", true},
		{"invalid registration", "WIKIT_REGISTRATION_ENABLED", "yes", false},
		{"invalid session ttl", "WIKIT_SESSION_TTL", "1s", false},
		{"valid session ttl", "WIKIT_SESSION_TTL", "12h", true},
		{"invalid upload limit", "WIKIT_MAX_UPLOAD_BYTES", "0", false},
		{"overflow upload limit", "WIKIT_MAX_UPLOAD_BYTES", "9223372036854775807", false},
		{"invalid GC grace", "WIKIT_BLOB_GC_GRACE", "0s", false},
		{"valid GC grace", "WIKIT_BLOB_GC_GRACE", "200h", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			get := func(key string) string {
				if tc.key == key {
					return tc.value
				}
				return base[key]
			}
			c, err := Load(get, "serve")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("secret leaked")
			}
			if tc.name == "defaults" && (c.ListenAddr != ":8080" || c.MediaRoot != "/var/lib/wikit/media") {
				t.Fatal("incorrect defaults")
			}
		})
	}
	if _, err := Load(func(key string) string {
		if key == "WIKIT_DATABASE_URL" {
			return base[key]
		}
		return ""
	}, "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(func(string) string { return "" }, "unknown"); err == nil {
		t.Fatal("accepted unknown command")
	}
}
