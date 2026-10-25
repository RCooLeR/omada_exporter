package session

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func controlSessionTestClient(transport http.RoundTripper) *Client {
	client := &Client{Config: &config.Config{Host: "https://omada.example"}, httpClient: &http.Client{Transport: transport}}
	client.SetContextIDs("cid", "site")
	client.setWebToken("test-csrf")
	return client
}

func TestControlSessionDoesNotReplayAmbiguousOrUnauthorizedMutation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		err        error
	}{
		{"network failure", "", errors.New("connection interrupted")},
		{"expired authentication", `{"errorCode":-1200,"msg":"invalid token"}`, nil},
		{"invalid acknowledgement", `{}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client := controlSessionTestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/loginStatus") {
					return jsonResponse(`{"errorCode":0,"result":{"login":true}}`), nil
				}
				calls.Add(1)
				if req.URL.Path != "/cid/api/v2/sites/site/cmd/devices/AA-BB-CC-DD-EE-FF/reboot" || req.Method != http.MethodPost {
					t.Errorf("unexpected command %s %s", req.Method, req.URL)
				}
				if req.GetBody != nil {
					t.Error("command must not be replayable")
				}
				if req.Header.Get("Csrf-Token") != "test-csrf" {
					t.Error("missing session token")
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return jsonResponse(tc.body), nil
			}))
			err := client.WithWebControlSession(t.Context(), "site", func(session *WebControlSession) error {
				return session.PostJSON("/cmd/devices/AA-BB-CC-DD-EE-FF/reboot", map[string]string{"mac": "AA-BB-CC-DD-EE-FF"})
			})
			if err == nil {
				t.Fatal("failed command reported success")
			}
			if calls.Load() != 1 {
				t.Fatalf("mutation attempts=%d, want1", calls.Load())
			}
		})
	}
}

func TestControlSessionRejectsContextChangeDuringAuthentication(t *testing.T) {
	var client *Client
	client = controlSessionTestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/loginStatus") {
			client.SetContextIDs("new-cid", "other-site")
			return jsonResponse(`{"errorCode":0,"result":{"login":true}}`), nil
		}
		t.Errorf("unexpected request after context change: %s", req.URL)
		return jsonResponse(`{"errorCode":0}`), nil
	}))
	called := false
	err := client.WithWebControlSession(t.Context(), "site", func(*WebControlSession) error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("context change allowed callback: err=%v called=%v", err, called)
	}
}

func TestControlSessionPinsContextAndAuthAcrossValidationAndCommand(t *testing.T) {
	func(t *testing.T) {
		var commandCalls atomic.Int32
		client := controlSessionTestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.HasSuffix(req.URL.Path, "/loginStatus"):
				return jsonResponse(`{"errorCode":0,"result":{"login":true}}`), nil
			case strings.HasSuffix(req.URL.Path, "/login"):
				return jsonResponse(`{"errorCode":0,"result":{"token":"new-token"}}`), nil
			case strings.HasSuffix(req.URL.Path, "/devices"):
				return jsonResponse(`{"errorCode":0,"result":[]}`), nil
			default:
				commandCalls.Add(1)
				if req.URL.Path != "/cid/api/v2/sites/site/cmd/devices/AA-BB-CC-DD-EE-FF/reboot" || req.Header.Get("Csrf-Token") != "test-csrf" {
					t.Errorf("command was retargeted: %s token=%s", req.URL, req.Header.Get("Csrf-Token"))
				}
				return jsonResponse(`{"errorCode":0}`), nil
			}
		}))
		changed := make(chan struct{})
		loggedIn := make(chan error, 1)
		err := client.WithWebControlSession(t.Context(), "site", func(session *WebControlSession) error {
			go func() { client.SetContextIDs("changed-cid", "changed-site"); close(changed) }()
			go func() { loggedIn <- client.Login() }()
			// A waiting RWMutex writer is not a durable block in synctest.
			// Wait explicitly until its presence blocks an additional reader.
			deadline := time.Now().Add(5 * time.Second)
			for client.contextMu.TryRLock() {
				client.contextMu.RUnlock()
				if time.Now().After(deadline) {
					t.Fatal("context writer did not start")
				}
				runtime.Gosched()
			}
			select {
			case <-changed:
				t.Fatal("context changed while control session held")
			default:
			}
			select {
			case err := <-loggedIn:
				t.Fatalf("login ran inside pinned session: %v", err)
			default:
			}
			// With a queued writer, a nested ContextIDs/RLock would deadlock here.
			var inventory any
			if err := session.ReadJSON("/devices", &inventory); err != nil {
				return err
			}
			return session.PostJSON("/cmd/devices/AA-BB-CC-DD-EE-FF/reboot", nil)
		})
		if err != nil {
			t.Fatal(err)
		}
		<-changed
		if err := <-loggedIn; err != nil {
			t.Fatal(err)
		}
		if commandCalls.Load() != 1 {
			t.Errorf("commands=%d", commandCalls.Load())
		}
	}(t)
}

func TestControlSessionDoesNotFollowHTTPRedirect(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		_, _ = io.WriteString(w, `{"errorCode":0}`)
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/loginStatus") {
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"login":true}}`)
			return
		}
		w.Header().Set("Location", destination.URL+"/command")
		w.WriteHeader(http.StatusTemporaryRedirect)
		_, _ = io.WriteString(w, `{"errorCode":0}`)
	}))
	defer source.Close()
	client := controlSessionTestClient(http.DefaultTransport)
	client.Config.Host = source.URL
	err := client.WithWebControlSession(t.Context(), "site", func(session *WebControlSession) error {
		return session.PostJSON("/cmd/devices/AA-BB-CC-DD-EE-FF/reboot", nil)
	})
	if err == nil || redirected.Load() != 0 {
		t.Fatalf("redirect followed or reported success: err=%v requests=%d", err, redirected.Load())
	}
}

func TestControlSessionRejectsUnsafeRelativePaths(t *testing.T) {
	client := controlSessionTestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/loginStatus") {
			t.Errorf("unsafe request %s", req.URL)
		}
		return jsonResponse(`{"errorCode":0,"result":{"login":true}}`), nil
	}))
	err := client.WithWebControlSession(t.Context(), "site", func(session *WebControlSession) error {
		for _, path := range []string{"https://other.example/cmd", "//other.example/cmd", "/../other/cmd", "/%2e%2e/other", "/devices#ignore", "/\\other"} {
			if err := session.PostJSON(path, nil); err == nil {
				t.Errorf("unsafe relative path accepted: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestControlSessionHonorsCancelledContext(t *testing.T) {
	client := controlSessionTestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Error("cancelled control made HTTP request")
		return nil, context.Canceled
	}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := client.WithWebControlSession(ctx, "site", func(*WebControlSession) error { t.Error("cancelled callback ran"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestCurrentControlSessionDoesNotCheckOrRefreshAuthentication(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "authenticated"
		if expired {
			name = "expired"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var authCalls, inventoryCalls int
				client := controlSessionTestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if strings.HasSuffix(req.URL.Path, "/loginStatus") || strings.HasSuffix(req.URL.Path, "/login") {
						authCalls++
						// Model authentication that ignores the optional inventory's
						// deadline. This path must never be entered for the read.
						time.Sleep(time.Minute)
						return jsonResponse(`{"errorCode":0,"result":{"login":true}}`), nil
					}
					inventoryCalls++
					if req.Method != http.MethodGet || req.URL.Path != "/cid/api/v2/sites/site/devices" || req.Header.Get("Csrf-Token") != "test-csrf" {
						t.Errorf("read lost its pinned session: %s %s", req.Method, req.URL)
					}
					if expired {
						return jsonResponse(`{"errorCode":-1200,"msg":"invalid token"}`), nil
					}
					return jsonResponse(`{"errorCode":0,"result":[]}`), nil
				}))
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				start := time.Now()
				err := client.WithCurrentWebControlSession(ctx, "site", func(session *WebControlSession) error {
					var inventory any
					return session.ReadJSON("/devices", &inventory)
				})
				if (err != nil) != expired || authCalls != 0 || inventoryCalls != 1 || time.Since(start) != 0 {
					t.Fatalf("current-session read: err=%v auth=%d inventory=%d elapsed=%s", err, authCalls, inventoryCalls, time.Since(start))
				}
			})
		})
	}
}

func TestCurrentControlSessionHonorsDeadlineWhileWaitingForAuthenticationGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := controlSessionTestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Errorf("blocked optional inventory made request: %s", req.URL)
			return nil, context.DeadlineExceeded
		}))
		unlock := client.webAuthTransition.lock()
		defer unlock()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		start := time.Now()
		err := client.WithCurrentWebControlSession(ctx, "site", func(*WebControlSession) error {
			t.Error("callback ran without the authentication gate")
			return nil
		})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 5*time.Second {
			t.Fatalf("gate wait exceeded optional budget: err=%v elapsed=%s", err, time.Since(start))
		}
	})
}

func TestCurrentControlSessionHonorsDeadlineDuringInventoryRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var requests int
		client := controlSessionTestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			if req.URL.Path != "/cid/api/v2/sites/site/devices" {
				t.Errorf("unexpected optional inventory request: %s", req.URL)
			}
			<-req.Context().Done()
			return nil, req.Context().Err()
		}))
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		start := time.Now()
		err := client.WithCurrentWebControlSession(ctx, "site", func(session *WebControlSession) error {
			var inventory any
			return session.ReadJSON("/devices", &inventory)
		})
		if !errors.Is(err, context.DeadlineExceeded) || requests != 1 || time.Since(start) != 5*time.Second {
			t.Fatalf("inventory read exceeded optional budget or retried: err=%v requests=%d elapsed=%s", err, requests, time.Since(start))
		}
	})
}
