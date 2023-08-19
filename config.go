package httplog

import (
	"log/slog"
	"os"
)

const (
	LevelTrace slog.Level = -8
	LevelDebug slog.Level = -4
	LevelInfo  slog.Level = 0
	LevelWarn  slog.Level = 4
	LevelError slog.Level = 8
	LevelFatal slog.Level = 12
)

var LevelNames = map[slog.Leveler]string{
	LevelTrace: "TRACE",
	LevelFatal: "FATAL",
}

type Format string

const (
	FormatJSON = Format("json")
	FormatText = Format("text")
)

type Options struct {
	// Value associated with the log key "service". Can be omitted.
	ServiceName string

	// Defines the minimum level of severity that app should log.
	//
	// Must be one of type slog.Level: LevelTrace, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal
	Level slog.Level

	// Defines the log format. Use FormatJSON in production mode so log aggregators can
	// receive data in parsable format. In local development mode, Use FormatText to
	// receive pretty output and stacktraces to stdout. For custom slog.Handler, create a new httplog.Format.
	//
	// FormatJSON will use slog.NewJSONHandler, FormatText will use slog.NewTextHandler.
	// Must be one of type Format: FormatJSON, FormatText
	Format Format

	// Concise mode includes fewer log details during the request flow. For example
	// excluding details like request content length, user-agent and other details.
	// This is useful if during development your console is too noisy.
	Concise bool

	// Additional fields included at the root level of all logs.
	// These can be useful for example the commit hash of a build, or an environment
	// name like prod/stg/dev
	Tags map[string]string

	// Set containing headers which values are considered sensitive. Will be omitted if LeakSensitiveValues is false.
	//
	// Default key automatically added: "authorization", "cookie", "set-cookie"
	SensitiveHeaders map[string]struct{}

	// Should always be set to false. If set to true, sensitive data depending on the implementation
	// will be displayed in the logs (i.e SensitiveHeaders).
	LeakSensitiveValues bool
}

var Opt = Options{
	Level:               LevelInfo,
	Format:              FormatJSON,
	Concise:             false,
	Tags:                nil,
	SensitiveHeaders:    nil,
	LeakSensitiveValues: false,
}

// Create a slog.Handler based on the options given in opt.
func NewLogger(opt Options) *slog.Logger {
	if opt.SensitiveHeaders == nil {
		opt.SensitiveHeaders = make(map[string]struct{})
	}
	opt.SensitiveHeaders["authorization"] = struct{}{}
	opt.SensitiveHeaders["cookie"] = struct{}{}
	opt.SensitiveHeaders["set-cookie"] = struct{}{}

	Opt = opt

	var handler slog.Handler

	handlerOpt := slog.HandlerOptions{
		Level: opt.Level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				level := a.Value.Any().(slog.Level)
				levelLabel, exists := LevelNames[level]
				if !exists {
					levelLabel = level.String()
				}
				a.Value = slog.StringValue(levelLabel)
			}
			return a
		},
	}
	if opt.Format == FormatJSON {
		handler = slog.NewJSONHandler(os.Stdout, &handlerOpt)
	} else {
		handler = slog.NewTextHandler(os.Stdout, &handlerOpt)
	}

	l := slog.New(handler)

	if opt.ServiceName != "" {
		l = l.With(slog.String("service", opt.ServiceName))
	}

	if !opt.Concise {
		var tags []any
		for k, v := range opt.Tags {
			tags = append(tags, k, v)
		}
		tagsAttr := slog.Group("tags", tags...)
		l = l.With(tagsAttr)
	}

	return l
}
