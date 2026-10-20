// Package config validates process configuration without exposing secret values.
package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL  string
	PublicURL    string
	ListenAddr   string
	MediaRoot    string
	Registration bool
	SessionTTL   time.Duration
}

func Load(getenv func(string) string, command string) (Config, error) {
	var c Config
	if command != "serve" && command != "migrate" && command != "create-admin" {
		return c, errors.New("usage: wikit serve|migrate|create-admin")
	}
	c.DatabaseURL = getenv("WIKIT_DATABASE_URL")
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || strings.Trim(u.Path, "/") == "" {
		return Config{}, errors.New("WIKIT_DATABASE_URL must be a PostgreSQL URL with host and database")
	}
	if command != "serve" {
		return c, nil
	}
	c.PublicURL = getenv("WIKIT_PUBLIC_URL")
	u, err = url.Parse(c.PublicURL)
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return Config{}, errors.New("WIKIT_PUBLIC_URL must be an origin without credentials, path, query or fragment")
	}
	local := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return Config{}, errors.New("WIKIT_PUBLIC_URL requires HTTPS or loopback HTTP")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return Config{}, errors.New("WIKIT_PUBLIC_URL port must be 1..65535")
		}
	}
	c.ListenAddr = getenv("WIKIT_LISTEN_ADDR")
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	_, port, err := net.SplitHostPort(c.ListenAddr)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 {
		return Config{}, errors.New("WIKIT_LISTEN_ADDR requires host:port with port 1..65535")
	}
	c.MediaRoot = getenv("WIKIT_MEDIA_ROOT")
	if c.MediaRoot == "" {
		c.MediaRoot = "/var/lib/wikit/media"
	}
	c.Registration = true
	if value := getenv("WIKIT_REGISTRATION_ENABLED"); value != "" {
		if value != "true" && value != "false" {
			return Config{}, errors.New("WIKIT_REGISTRATION_ENABLED must be true or false")
		}
		c.Registration = value == "true"
	}
	c.SessionTTL = 24 * time.Hour
	if value := getenv("WIKIT_SESSION_TTL"); value != "" {
		c.SessionTTL, err = time.ParseDuration(value)
		if err != nil || c.SessionTTL < 5*time.Minute || c.SessionTTL > 168*time.Hour {
			return Config{}, errors.New("WIKIT_SESSION_TTL must be between 5m and 168h")
		}
	}
	return c, nil
}
