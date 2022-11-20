package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"handler/function"
)

var (
	acceptingConnections int32
)

const defaultTimeout = 10 * time.Second

func main() {
	readTimeout := parseIntOrDurationValue(os.Getenv("read_timeout"), defaultTimeout)
	writeTimeout := parseIntOrDurationValue(os.Getenv("write_timeout"), defaultTimeout)
	healthInterval := parseIntOrDurationValue(os.Getenv("healthcheck_interval"), writeTimeout)

	s := &http.Server{
		Addr:           fmt.Sprintf(":%d", 8082),
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		MaxHeaderBytes: 1 << 20, // Max header of 1MB
	}

	http.HandleFunc("/", function.Handle)

	/*
	 * @dermatologist
	 * This is for supporting the use of the container in kubeflow.
	 * Implement the HandleFile function to support the use of the container in kubeflow.
	 * The call without commandline parameters will run in the server model
	 */
	if len(os.Args) < 2 {
		listenUntilShutdown(s, healthInterval, writeTimeout)
	} else {
		dat, _ := os.ReadFile(os.Args[1])
		_dat := function.HandleFile(dat)
		_ = os.WriteFile(os.Args[2], _dat, 0644)
	}

	listenUntilShutdown(s, healthInterval, writeTimeout)
}

func listenUntilShutdown(s *http.Server, shutdownTimeout time.Duration, writeTimeout time.Duration) {
	idleConnsClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM)

		<-sig

		log.Printf("[entrypoint] SIGTERM: no connections in: %s", shutdownTimeout.String())
		<-time.Tick(shutdownTimeout)

		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		defer cancel()

		if err := s.Shutdown(ctx); err != nil {
			log.Printf("[entrypoint] Error in Shutdown: %v", err)
		}

		log.Printf("[entrypoint] Exiting.")

		close(idleConnsClosed)
	}()

	// Run the HTTP server in a separate go-routine.
	go func() {
		if err := s.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("[entrypoint] Error ListenAndServe: %v", err)
			close(idleConnsClosed)
		}
	}()

	atomic.StoreInt32(&acceptingConnections, 1)

	<-idleConnsClosed
}

func parseIntOrDurationValue(val string, fallback time.Duration) time.Duration {
	if len(val) > 0 {
		parsedVal, parseErr := strconv.Atoi(val)
		if parseErr == nil && parsedVal >= 0 {
			return time.Duration(parsedVal) * time.Second
		}
	}

	duration, durationErr := time.ParseDuration(val)
	if durationErr != nil {
		return fallback
	}
	return duration
}
