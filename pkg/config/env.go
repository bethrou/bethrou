package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadDotEnv reads a `.env`-style file (KEY=VALUE per line; blank lines and
// lines starting with '#' are ignored; values may be wrapped in matching
// single or double quotes) and applies each entry to the process
// environment via os.Setenv, without overwriting a variable the process
// already has set — real environment variables (e.g. from the shell,
// systemd, or a container orchestrator's secret injection) always take
// precedence over the file. Missing files are not an error: `.env` is
// optional local/dev convenience, not a required config source.
//
// Call this once, as early as possible in main(), before any flag
// defaults are computed with EnvOr/EnvBoolOr/etc. below — those read
// os.Getenv at flag-registration time, so the file must already be loaded.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=VALUE, got %q", path, lineNo, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("%s:%d: empty key", path, lineNo)
		}

		value = unquote(value)

		if _, alreadySet := os.LookupEnv(key); alreadySet {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("%s:%d: failed to set %s: %w", path, lineNo, key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	return nil
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// EnvOr returns the environment variable key's value, or def if unset.
// Intended for computing CLI flag defaults, so precedence ends up being
// flag > env var (including one loaded from .env via LoadDotEnv) >
// hardcoded default.
func EnvOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// EnvBoolOr is EnvOr for a boolean flag default. An unparseable value
// falls back to def rather than panicking, since this runs at flag
// registration time, before any error-returning code path exists.
func EnvBoolOr(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// EnvIntOr is EnvOr for an integer flag default.
func EnvIntOr(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// EnvDurationOr is EnvOr for a time.Duration flag default (e.g. "30s").
func EnvDurationOr(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// EnvStringSliceOr is EnvOr for a comma-separated list flag default.
func EnvStringSliceOr(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}
