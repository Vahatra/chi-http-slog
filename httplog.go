package httplog

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type loggerOptions struct {
	l         *slog.Logger
	concise   bool                // detailed or concise logs
	sensitive map[string]struct{} // a set for storing fields that should not be logged
	leak      bool                // ignore "sensitive" and log everything
}

type LoggerOption func(*loggerOptions)

func evaluateLoggerOptions(opts []LoggerOption) *loggerOptions {
	opt := &loggerOptions{
		l:         slog.Default(),
		concise:   false,
		sensitive: nil,
		leak:      false,
	}
	for _, o := range opts {
		o(opt)
	}
	return opt
}

func RequestLogger(opts ...LoggerOption) func(next http.Handler) http.Handler {
	o := evaluateLoggerOptions(opts)
	return middleware.RequestLogger(&loggerOptions{
		l:         o.l,
		concise:   o.concise,
		sensitive: o.sensitive,
		leak:      o.leak,
	})
}

func (o *loggerOptions) NewLogEntry(r *http.Request) middleware.LogEntry {
	entry := &LogEntry{
		l:         o.l,
		concise:   o.concise,
		sensitive: o.sensitive,
		leak:      o.leak,
	}
	entry.req(r)

	return entry
}

type LogEntry struct {
	l         *slog.Logger
	msg       string
	concise   bool                // detailed or concise logs
	sensitive map[string]struct{} // a set for storing fields that should not be logged
	leak      bool                // ignore "sensitive" and log everything
}

func (le *LogEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	responseAttr := make([]any, 0, 3) // slog.Attr
	responseAttr = append(responseAttr,
		slog.Int("size", bytes),
		slog.Group("status",
			slog.Int("code", status),
			slog.String("msg", http.StatusText(status)),
		))
	if !le.concise {
		responseAttr = append(responseAttr, httpHeaderAttrs(header, le.leak, le.sensitive))
	}
	le.l = le.l.With(slog.Group("response", responseAttr...))

	msg := fmt.Sprintf("%d %s", status, http.StatusText(status))
	if le.msg != "" {
		msg = fmt.Sprintf("%s - %s", msg, le.msg)
	}
	le.l.LogAttrs(
		nil,
		toLogLevel(status),
		msg,
	)
}

func (le *LogEntry) Panic(v interface{}, stack []byte) {
	stacktrace := "#"
	stacktrace = string(stack)
	le.l = le.l.With(slog.String("stacktrace", stacktrace), slog.String("panic", fmt.Sprintf("%+v", v)))
}

func (le *LogEntry) req(r *http.Request) {
	reqID := middleware.GetReqID(r.Context())
	if reqID != "" {
		le.l = le.l.With(slog.String("id", reqID))
	}

	requestAttr := make([]any, 0, 7) // slog.Attr
	requestAttr = append(requestAttr,
		slog.String("uri", r.RequestURI),
		slog.String("method", r.Method),
	)

	if !le.concise {
		requestAttr = append(requestAttr,
			slog.String("host", r.Host),
			slog.String("path", r.URL.Path),
			slog.String("proto", r.Proto),
			slog.String("remote", r.RemoteAddr),
			httpHeaderAttrs(r.Header, le.leak, le.sensitive),
		)
	}
	le.l = le.l.With(slog.Group("request", requestAttr...))
}

func httpHeaderAttrs(header http.Header, leak bool, sensitive map[string]struct{}) slog.Attr {
	hearderAttr := make([]any, 0, len(header)) // []slog.Attr

	for k, v := range header {
		k = strings.ToLower(k)
		_, ok := sensitive[k]

		switch {
		case ok && !leak: // filtering sensitive headers
			continue
		case len(v) == 0:
			continue
		case len(v) == 1:
			hearderAttr = append(hearderAttr, slog.String(k, v[0]))
		default:
			hearderAttr = append(hearderAttr, slog.String(k, fmt.Sprintf("[%s]", strings.Join(v, "], ["))))
		}
	}

	return slog.Group("headers", hearderAttr...)
}

func toLogLevel(status int) slog.Level {
	switch {
	case status <= 0:
		return slog.LevelWarn
	case status < 400: // for codes in 100s, 200s, 300s
		return slog.LevelInfo
	case status >= 400 && status < 500:
		return slog.LevelWarn
	case status >= 500:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Helper methods used by the application to get the request-scoped
// logger entry and set additional attrs between handlers.
//
// This is a useful pattern to use to set state on the entry as it
// passes through the handler chain, which at any point can be logged
// with a call to .Print(), .Info(), etc.

func GetLogEntry(r *http.Request) *slog.Logger {
	entry := middleware.GetLogEntry(r).(*LogEntry)
	return entry.l
}

func LogEntrySetAttr(r *http.Request, attr slog.Attr) {
	if entry, ok := r.Context().Value(middleware.LogEntryCtxKey).(*LogEntry); ok {
		entry.l = entry.l.With(attr)
	}
}

// Options

// WithLoggerLogger is a functional option to use another *slog.Logger
func WithLogger(l *slog.Logger) LoggerOption {
	return func(o *loggerOptions) {
		o.l = l
	}
}

func WithConcise(concise bool) LoggerOption {
	return func(o *loggerOptions) {
		o.concise = concise
	}
}

func WithSensitive(s map[string]struct{}) LoggerOption {
	return func(o *loggerOptions) {
		// https://github.com/uber-go/guide/blob/master/style.md#copy-slices-and-maps-at-boundaries
		if s == nil {
			o.sensitive = make(map[string]struct{}, 3)
		} else {
			o.sensitive = make(map[string]struct{}, len(s))
			for k := range s {
				o.sensitive[k] = struct{}{}
			}
		}
		s["authorization"] = struct{}{}
		s["cookie"] = struct{}{}
		s["set-cookie"] = struct{}{}
	}
}

// only for dev purposes
func WithLeak(leakSensitiveData bool) LoggerOption {
	return func(o *loggerOptions) {
		o.leak = leakSensitiveData
	}
}
