package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/bethrou/bethrou/pkg/config"
)

// LevelTrace is one tier below slog.LevelDebug (-4), following the same
// spacing slog itself uses between Debug/Info/Warn/Error (0/4/8 apart) —
// log/slog has no built-in trace level, so this is how "trace" (the most
// verbose, highest-volume detail — below what's useful even at debug) is
// represented. Use Trace(...) below rather than Logger.Log directly.
const LevelTrace slog.Level = slog.LevelDebug - 4

// Trace logs at LevelTrace — the log/slog equivalent of Logger.Debug(...),
// which slog itself doesn't provide a method for since LevelTrace isn't
// one of its four built-in levels.
func Trace(msg string, args ...any) {
	Logger.Log(context.Background(), LevelTrace, msg, args...)
}

// Debug, Info, Warn, and Error are the preferred way to log — thin
// wrappers over the package-level Logger, so call sites write
// logging.Info(...) rather than logging.Logger.Info(...). Logger itself
// stays exported for the rare caller that needs the raw *slog.Logger
// (e.g. StdLog below, or handing it to something that takes one), but
// these are the common-case entry points.
func Debug(msg string, args ...any) { Logger.Debug(msg, args...) }
func Info(msg string, args ...any)  { Logger.Info(msg, args...) }
func Warn(msg string, args ...any)  { Logger.Warn(msg, args...) }
func Error(msg string, args ...any) { Logger.Error(msg, args...) }

// Logger is never nil: it starts with a sensible default (text, info
// level, stdout) so callers — notably each service's main(), which may
// need to report a fatal error before Setup has run (e.g. a malformed
// .env or config file) — can always use it safely, without a nil check.
// Setup/SetupOutput replace it once real config is available.
var Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

// Setup configures the package-level Logger according to cfg, writing to
// os.Stdout. If cfg is nil, a default text logger at info level is
// created.
func Setup(cfg *config.LogConfig) {
	SetupOutput(cfg, os.Stdout)
}

// SetupOutput is Setup with an explicit destination — for callers that
// can't share stdout with their logs (e.g. a full-screen terminal UI),
// pointing it at a file instead.
func SetupOutput(cfg *config.LogConfig, w io.Writer) {
	if cfg == nil {
		cfg = &config.LogConfig{Level: "info", Format: "text"}
	}

	level := parseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		Level: level,
		// Without this, a handler asked to print LevelTrace (-8) renders
		// it as "DEBUG-4" (slog's default: nearest named level + offset)
		// — same for any log written directly at slog.LevelDebug-1..-3.
		// This maps every level below Debug to the literal "TRACE" label.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				if lvl, ok := a.Value.Any().(slog.Level); ok && lvl < slog.LevelDebug {
					a.Value = slog.StringValue("TRACE")
				}
			}
			return a
		},
	}

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	Logger = slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "trace":
		return LevelTrace
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// StdLog returns an io.Writer that can be used with the standard library log
// package to forward logs into the structured logger. Callers can use it with
// log.SetOutput(logging.StdLog()).
func StdLog() io.Writer {
	// simple adapter that writes to Logger at Info level
	r, w, _ := os.Pipe()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				Logger.Info(string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()
	return w
}
