package config

import (
	"strings"
	"testing"
)

func TestConfiguration(t *testing.T) {
	base := map[string]string{"WIKIT_DATABASE_URL": "postgres://user:secret@localhost/wikit", "WIKIT_PUBLIC_URL": "http://localhost:8080"}
	for _, tc := range []struct {
		name, key, value string
		valid            bool
	}{
		{"defaults", "", "", true},
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
