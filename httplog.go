package httplog

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func RequestLogger(l *slog.Logger) func(next http.Handler) http.Handler {
	return middleware.RequestLogger(&requestLogger{Logger: l})
}

type requestLogger struct {
	Logger *slog.Logger
}

func (l *requestLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	entry := &RequestLoggerEntry{}
	entry.Logger = l.Logger.With(requestLogAttrs(r))

	return entry
}

type RequestLoggerEntry struct {
	Logger *slog.Logger
	msg    string
}

func (l *RequestLoggerEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	// responseLogAttrs
	msg := fmt.Sprintf("%d %s", status, statusLabel(status))
	if l.msg != "" {
		msg = fmt.Sprintf("%s - %s", msg, l.msg)
	}

	responseAttr := []any{ // slog.Attr
		slog.Int("status", status),
	}

	if Opt.Concise {
		l.Logger.LogAttrs(nil, statusLevel(status), msg, slog.Group("response", responseAttr...))
		return
	}

	responseAttr = append(responseAttr,
		slog.Int("bytes", bytes),
		slog.Float64("elapsed", float64(elapsed.Nanoseconds())/1000000.0),
	)

	if len(header) > 0 {
		responseAttr = append(responseAttr, headerLogAttrs(header))
	}

	l.Logger.LogAttrs(nil, statusLevel(status), msg, slog.Group("response", responseAttr...))
}

func (l *RequestLoggerEntry) Panic(v interface{}, stack []byte) {
	stacktrace := "#"
	if Opt.Format == FormatJSON {
		stacktrace = string(stack)
	}

	l.Logger = l.Logger.With(slog.String("stacktrace", stacktrace), slog.String("panic", fmt.Sprintf("%+v", v)))

	if Opt.Format == FormatText {
		middleware.PrintPrettyStack(v)
	}
}

func requestLogAttrs(r *http.Request) slog.Attr {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	requestURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)
	requestAttr := []any{ // []slog.Attr
		slog.String("method", r.Method),
		slog.String("url", requestURL),
	}

	if Opt.Concise {
		return slog.Group("request", requestAttr...)
	}

	requestAttr = append(requestAttr,
		headerLogAttrs(r.Header),
		slog.String("proto", r.Proto),
		slog.String("scheme", scheme),
		slog.String("path", r.URL.Path),
		slog.String("remote", r.RemoteAddr),
	)
	if reqID := middleware.GetReqID(r.Context()); reqID != "" {
		requestAttr = append(requestAttr, slog.String("id", reqID))
	}

	return slog.Group("request", requestAttr...)
}

func headerLogAttrs(header http.Header) slog.Attr {
	var hearderAttr []any // []slog.Attr

	for k, v := range header {
		k = strings.ToLower(k)
		_, ok := Opt.SensitiveHeaders[k]

		switch {
		case ok && !Opt.LeakSensitiveValues: // filtering sensitive headers
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

func statusLevel(status int) slog.Level {
	switch {
	case status <= 0:
		return LevelWarn
	case status < 400: // for codes in 100s, 200s, 300s
		return LevelInfo
	case status >= 400 && status < 500:
		return LevelWarn
	case status >= 500:
		return LevelFatal
	default:
		return LevelInfo
	}
}

func statusLabel(status int) string {
	switch {
	case status >= 100 && status < 300:
		return "OK"
	case status >= 300 && status < 400:
		return "Redirect"
	case status >= 400 && status < 500:
		return "Client Error"
	case status >= 500:
		return "Server Error"
	default:
		return "Unknown"
	}
}

// Helper methods used by the application to get the request-scoped
// logger entry and set additional attrs between handlers.
//
// This is a useful pattern to use to set state on the entry as it
// passes through the handler chain, which at any point can be logged
// with a call to .Print(), .Info(), etc.

func GetLogEntry(r *http.Request) *slog.Logger {
	entry := middleware.GetLogEntry(r).(*RequestLoggerEntry)
	return entry.Logger
}

func LogEntrySetAttr(r *http.Request, attr slog.Attr) {
	if entry, ok := r.Context().Value(middleware.LogEntryCtxKey).(*RequestLoggerEntry); ok {
		entry.Logger = entry.Logger.With(attr)
	}
}
