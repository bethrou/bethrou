package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/henrybarreto/bethrou/pkg/config"
)

var Logger *slog.Logger

// Setup configures the package-level Logger according to cfg. If cfg is nil,
// a default text logger at info level is created.
func Setup(cfg *config.LogConfig) {
	if cfg == nil {
		cfg = &config.LogConfig{Level: "info", Format: "text"}
	}

	level := parseLevel(cfg.Level)

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	Logger = slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
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
				if Logger != nil {
					Logger.Info(string(buf[:n]))
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return w
}
