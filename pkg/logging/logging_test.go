package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bethrou/bethrou/pkg/config"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]struct {
		in   string
		want int
	}{
		"trace":                   {"trace", int(LevelTrace)},
		"trace upper":             {"TRACE", int(LevelTrace)},
		"debug":                   {"debug", -4},
		"info":                    {"info", 0},
		"warn":                    {"warn", 4},
		"warning alias":           {"warning", 4},
		"error":                   {"error", 8},
		"unknown default to info": {"nonsense", 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := int(parseLevel(tc.in)); got != tc.want {
				t.Errorf("parseLevel(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestSetupOutputEmitsTraceLevelWhenConfigured(t *testing.T) {
	var buf bytes.Buffer
	SetupOutput(&config.LogConfig{Level: "trace", Format: "text"}, &buf)

	Trace("trace message", "key", "value")

	out := buf.String()
	if !strings.Contains(out, "trace message") {
		t.Fatalf("expected trace message in output, got: %q", out)
	}
	if !strings.Contains(out, "TRACE") {
		t.Fatalf("expected level to render as TRACE, got: %q", out)
	}
}

func TestSetupOutputSuppressesTraceBelowDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	SetupOutput(&config.LogConfig{Level: "debug", Format: "text"}, &buf)

	Trace("should not appear")

	if buf.Len() != 0 {
		t.Fatalf("expected no output for trace log below configured debug level, got: %q", buf.String())
	}
}

func TestSetupOutputJSONFormatsTraceLevel(t *testing.T) {
	var buf bytes.Buffer
	SetupOutput(&config.LogConfig{Level: "trace", Format: "json"}, &buf)

	Trace("trace in json")

	out := buf.String()
	if !strings.Contains(out, `"level":"TRACE"`) {
		t.Fatalf("expected JSON level field to be TRACE, got: %q", out)
	}
}
