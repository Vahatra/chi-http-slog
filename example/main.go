package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httplog "github.com/Vahatra/chi-http-slog"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	r := chi.NewRouter()

	sensitiveHeaders := map[string]struct{}{"token": {}}
	l := httplog.NewLogger(httplog.Options{
		ServiceName: "hello",
		Level:       httplog.LevelTrace,
		Format:      httplog.FormatJSON,
		Concise:     false,
		Tags: map[string]string{
			"version": "v1.0-81aa4244d9fc8076a",
			"env":     "dev",
		},
		SensitiveHeaders:    sensitiveHeaders,
		LeakSensitiveValues: false,
	})

	r.Use(middleware.RequestID)
	r.Use(httplog.RequestLogger(l))
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)

	// New headers will be added under response.headers on the log entry
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("new", "header")
	})
	// Stacktrace will be added to the log entry of level fatal
	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("panic")
	})
	// Adding new attr to the log entry
	r.Get("/attr", func(w http.ResponseWriter, r *http.Request) {
		httplog.LogEntrySetAttr(r, slog.String("new", "attr"))
	})
	// For trying graceful shutdown
	r.Get("/wait", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})

	// The HTTP server
	server := &http.Server{
		Addr:         "127.0.0.1:8080",
		Handler:      r,
		ReadTimeout:  5 * time.Second,   // max time to read request from the client
		WriteTimeout: 10 * time.Second,  // max time to write response to the client
		IdleTimeout:  120 * time.Second, // max time for connections using TCP Keep-Alive
	}

	// Server run context
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	// Listen for syscall signals for process to interrupt/quit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-sig

		// Shutdown signal with grace period of 10 seconds
		shutdownCtx, cancel := context.WithTimeout(serverCtx, 10*time.Second)
		defer cancel()

		// Trigger graceful shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			l.Error(err.Error())
		}
		serverStopCtx()
	}()

	// Run the server
	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		l.Error(err.Error())
	}

	// Wait for the server context to be stopped
	<-serverCtx.Done()
}
