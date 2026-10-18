package app

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type fakeHTTPServer struct {
	listen   func() error
	shutdown func(context.Context) error
}

func (server fakeHTTPServer) ListenAndServe() error { return server.listen() }
func (server fakeHTTPServer) Shutdown(ctx context.Context) error {
	return server.shutdown(ctx)
}

func TestServeStopsPublisherWhenHTTPStartupFails(t *testing.T) {
	wantErr := errors.New("listener unavailable")
	publisherStopped := make(chan struct{})
	health := &healthState{}
	server := fakeHTTPServer{
		listen: func() error { return wantErr },
		shutdown: func(context.Context) error {
			t.Error("Shutdown called after listener startup failure")
			return nil
		},
	}
	err := serve(t.Context(), server, health, func(ctx context.Context) error {
		<-ctx.Done()
		close(publisherStopped)
		return ctx.Err()
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("serve error = %v, want %v", err, wantErr)
	}
	select {
	case <-publisherStopped:
	default:
		t.Fatal("serve returned before the publisher stopped")
	}
	if health.ready.Load() {
		t.Fatal("failed server remains ready")
	}
	if t.Context().Err() != nil {
		t.Fatal("runtime canceled its caller's context")
	}
}

func TestServeCancellationShutsDownHTTPAndPublisher(t *testing.T) {
	for _, shutdownErr := range []error{nil, errors.New("shutdown failed")} {
		name := "success"
		if shutdownErr != nil {
			name = "shutdown error"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			listening := make(chan struct{})
			serverStopped := make(chan struct{})
			publisherStarted := make(chan struct{})
			publisherStopped := make(chan struct{})
			health := &healthState{}
			server := fakeHTTPServer{
				listen: func() error {
					close(listening)
					<-serverStopped
					return http.ErrServerClosed
				},
				shutdown: func(ctx context.Context) error {
					if _, ok := ctx.Deadline(); !ok {
						t.Error("HTTP shutdown has no deadline")
					}
					if health.ready.Load() {
						t.Error("server remains ready during shutdown")
					}
					close(serverStopped)
					return shutdownErr
				},
			}
			result := make(chan error, 1)
			go func() {
				result <- serve(ctx, server, health, func(ctx context.Context) error {
					close(publisherStarted)
					<-ctx.Done()
					close(publisherStopped)
					return ctx.Err()
				})
			}()
			<-listening
			<-publisherStarted
			if !health.ready.Load() {
				t.Error("server was not marked ready")
			}
			cancel()
			if err := <-result; !errors.Is(err, shutdownErr) {
				t.Fatalf("serve error = %v, want %v", err, shutdownErr)
			}
			select {
			case <-publisherStopped:
			default:
				t.Fatal("serve returned before the publisher stopped")
			}
		})
	}
}
