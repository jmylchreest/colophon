package serve

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestShutdownReleasesReloadStreams reproduces the goroutine-shutdown bug: a live-reload SSE
// handler parks in its event loop, and http.Server.Shutdown waits for in-flight handlers to
// return but never cancels their request context. Without beginShutdown signalling the stream,
// Shutdown blocks until its deadline and then returns an error — turning Ctrl-C into a multi-second
// hang. With the fix, the stream returns promptly and Shutdown drains in milliseconds.
func TestShutdownReleasesReloadStreams(t *testing.T) {
	s := &Server{
		clients:  map[chan struct{}]bool{},
		shutdown: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(reloadPath, s.handleReload)
	srv := &http.Server{Handler: mux}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()

	// Open the stream and read the initial ": connected" comment so the handler is parked in its
	// select loop (registered as an active connection) before we shut down.
	resp, err := http.Get("http://" + ln.Addr().String() + reloadPath)
	if err != nil {
		t.Fatalf("open reload stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := resp.Body.Read(make([]byte, 16)); err != nil {
		t.Fatalf("read connected frame: %v", err)
	}

	// Mirror ListenAndServe's shutdown branch: signal streams, then drain with a generous timeout.
	s.beginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("shutdown took %v with an open reload stream; the stream was not released", d)
	}
}

// TestBeginShutdownIdempotent guards the sync.Once: signalling shutdown more than once (e.g. a
// second Ctrl-C) must not panic on a double close.
func TestBeginShutdownIdempotent(t *testing.T) {
	s := &Server{shutdown: make(chan struct{})}
	s.beginShutdown()
	s.beginShutdown()
	select {
	case <-s.shutdown:
	default:
		t.Fatal("shutdown channel was not closed")
	}
}
