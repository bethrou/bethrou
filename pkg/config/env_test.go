package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "" +
		"# a comment\n" +
		"\n" +
		"FOO=bar\n" +
		"QUOTED=\"hello world\"\n" +
		"SINGLE='single quoted'\n" +
		"  SPACED  =   spaced value  \n" +
		"EMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	for _, k := range []string{"FOO", "QUOTED", "SINGLE", "SPACED", "EMPTY", "PREEXISTING"} {
		t.Cleanup(func(k string) func() {
			return func() { os.Unsetenv(k) }
		}(k))
	}
	t.Setenv("PREEXISTING", "from-shell")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	cases := map[string]string{
		"FOO":    "bar",
		"QUOTED": "hello world",
		"SINGLE": "single quoted",
		"SPACED": "spaced value",
		"EMPTY":  "",
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s: expected %q, got %q", k, want, got)
		}
	}
}

func TestLoadDotEnvDoesNotOverrideExistingEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PREEXISTING=from-file\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Setenv("PREEXISTING", "from-shell")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	if got := os.Getenv("PREEXISTING"); got != "from-shell" {
		t.Errorf("expected real environment variable to win over .env, got %q", got)
	}
}

func TestLoadDotEnvMissingFileIsNotAnError(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env")); err != nil {
		t.Fatalf("expected no error for missing .env file, got %v", err)
	}
}

func TestLoadDotEnvRejectsMalformedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("not-a-valid-line\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := LoadDotEnv(path); err == nil {
		t.Fatal("expected an error for a malformed .env line")
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Setenv("TEST_STR", "hello")
	t.Setenv("TEST_BOOL", "true")
	t.Setenv("TEST_INT", "42")
	t.Setenv("TEST_DURATION", "30s")
	t.Setenv("TEST_LIST", "a, b ,c")

	if got := EnvOr("TEST_STR", "default"); got != "hello" {
		t.Errorf("EnvOr: expected %q, got %q", "hello", got)
	}
	if got := EnvOr("TEST_STR_UNSET", "default"); got != "default" {
		t.Errorf("EnvOr unset: expected %q, got %q", "default", got)
	}
	if got := EnvBoolOr("TEST_BOOL", false); got != true {
		t.Errorf("EnvBoolOr: expected true, got %v", got)
	}
	if got := EnvBoolOr("TEST_BOOL_UNSET", true); got != true {
		t.Errorf("EnvBoolOr unset: expected true, got %v", got)
	}
	if got := EnvIntOr("TEST_INT", 0); got != 42 {
		t.Errorf("EnvIntOr: expected 42, got %d", got)
	}
	if got := EnvIntOr("TEST_INT_UNSET", 7); got != 7 {
		t.Errorf("EnvIntOr unset: expected 7, got %d", got)
	}
	if got := EnvDurationOr("TEST_DURATION", 0); got != 30*time.Second {
		t.Errorf("EnvDurationOr: expected 30s, got %v", got)
	}
	if got := EnvDurationOr("TEST_DURATION_UNSET", time.Minute); got != time.Minute {
		t.Errorf("EnvDurationOr unset: expected 1m, got %v", got)
	}
	list := EnvStringSliceOr("TEST_LIST", nil)
	if len(list) != 3 || list[0] != "a" || list[1] != "b" || list[2] != "c" {
		t.Errorf("EnvStringSliceOr: expected [a b c], got %v", list)
	}
	def := []string{"x"}
	if got := EnvStringSliceOr("TEST_LIST_UNSET", def); len(got) != 1 || got[0] != "x" {
		t.Errorf("EnvStringSliceOr unset: expected %v, got %v", def, got)
	}
}
