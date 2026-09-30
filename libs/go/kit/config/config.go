// Package config reads the environment. Every Encabulator service is configured
// by environment variables only (design section 7, "Configuration").
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// String returns the value of key, or def when unset or empty.
func String(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Int returns the value of key parsed as an int, or def when unset or unparseable.
func Int(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// Float returns the value of key parsed as a float64, or def.
func Float(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// Bool returns the value of key parsed as a bool, or def. "true", "1", "yes"
// and "on" (any case) are true.
func Bool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "":
		return def
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return def
	}
}

// Duration returns the value of key parsed by time.ParseDuration, or def.
func Duration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// Require returns the value of key, or an error when it is unset or empty.
func Require(key string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("config: required environment variable %s is not set", key)
}

// ServiceURL resolves the conventional <SERVICE_NAME_IN_UPPER_SNAKE>_URL variable
// for a service name such as "hydrocoptic-datastore". The returned value has any
// trailing slash trimmed. It is empty when the variable is unset.
func ServiceURL(service string) string {
	return strings.TrimRight(String(URLVar(service), ""), "/")
}

// URLVar returns the environment variable name that holds a service's base URL,
// for example "hydrocoptic-datastore" -> "HYDROCOPTIC_DATASTORE_URL".
func URLVar(service string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(service)) + "_URL"
}
