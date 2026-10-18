package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	exporterShutdownTimeout     = 10 * time.Second
	exporterMQTTShutdownTimeout = 15 * time.Second
)

type httpServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

// serve owns background publishing even when HTTP startup fails.
func serve(ctx context.Context, server httpServer, health *healthState, publish func(context.Context) error) error {
	runCtx, stop := context.WithCancel(ctx)
	var publisherDone <-chan struct{}
	defer func() {
		health.ready.Store(false)
		stop()
		waitForPublisher(publisherDone, exporterMQTTShutdownTimeout)
	}()
	if publish != nil {
		done := make(chan struct{})
		publisherDone = done
		go func() {
			defer close(done)
			if err := publish(runCtx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error().Err(err).Msg("home assistant mqtt publisher stopped")
			}
		}()
	}
	health.ready.Store(true)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		health.ready.Store(false)
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), exporterShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down exporter HTTP server: %w", err)
		}
		err := <-serveErr
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func waitForPublisher(done <-chan struct{}, timeout time.Duration) {
	if done == nil {
		return
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		log.Warn().Dur("timeout", timeout).Msg("timed out waiting for MQTT publisher shutdown")
	}
}
