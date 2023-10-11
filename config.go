package httplog

import (
	"log/slog"
	"os"
)

// TODO once https://github.com/uber-go/zap/issues/1333 is done, consider adding zap as a backend.

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

type options struct {
	// serviceName is the value associated with the log key "service". Can be omitted.
	serviceName string

	// level defines the minimum level of severity that app should log.
	//
	// Must be one of type slog.level: LevelTrace, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal
	level slog.Level

	// format defines the log format. Use FormatJSON in production mode so log aggregators can
	// receive data in parsable format. In local development mode, Use FormatText to
	// receive pretty output and stacktraces to stdout. For custom slog.Handler, create a new httplog.format.
	//
	// "json" will use slog.NewJSONHandler, otherwise slog.NewTextHandler will be used
	format string

	// tags are additional fields included at the root level of all logs.
	// These can be useful for example the commit hash of a build, or an environment
	// name like prod/stg/dev
	tags map[string]string
}

var defaultOptions = &options{
	level:  LevelInfo,
	format: "json",
	tags:   nil,
}

type Option func(*options)

func evaluateOptions(opts []Option) *options {
	optCopy := &options{}
	*optCopy = *defaultOptions
	for _, o := range opts {
		o(optCopy)
	}
	return optCopy
}

// Create a slog.Handler based on the options given in opt.
func NewLogger(options ...Option) *slog.Logger {
	o := evaluateOptions(options)
	var handler slog.Handler
	handlerOpt := slog.HandlerOptions{
		Level: o.level,
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
	if o.format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &handlerOpt)
	} else {
		handler = slog.NewTextHandler(os.Stdout, &handlerOpt)
	}

	l := slog.New(handler)

	if o.serviceName != "" {
		l = l.With(slog.String("service", o.serviceName))
	}

	if len(o.tags) > 0 {
		var tags []any // slog.Attr
		for k, v := range o.tags {
			tags = append(tags, slog.String(k, v))
		}
		l = l.With(slog.Group("tags", tags...))
	}

	return l
}

func WithServiceName(serviceName string) Option {
	return func(o *options) {
		o.serviceName = serviceName
	}
}

func WithLevel(level slog.Level) Option {
	return func(o *options) {
		o.level = level
	}
}

func WithFormat(format string) Option {
	return func(o *options) {
		if format == "json" {
			o.format = format
		} else {
			o.format = "text"
		}
	}
}

func WithTags(t map[string]string) Option {
	return func(o *options) {
		// https://github.com/uber-go/guide/blob/master/style.md#copy-slices-and-maps-at-boundaries
		if t == nil {
			o.tags = make(map[string]string, 0)
		} else {
			o.tags = make(map[string]string, len(t))
			for k, v := range t {
				o.tags[k] = v
			}
		}
	}
}
