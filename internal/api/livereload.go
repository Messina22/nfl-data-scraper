package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

// liveReloadPoll is how often the server re-stats the static directory.
const liveReloadPoll = 300 * time.Millisecond

// liveReloadScript reloads the page on a static-asset change, and also when the
// boot ID changes — which happens when wgo rebuilds and restarts the server after
// a Go edit. EventSource reconnects on its own; retry: 500 makes that reconnect
// faster than the browser's ~3s default.
const liveReloadScript = `(() => {
  let bootID = null;
  const es = new EventSource("/api/livereload");
  es.addEventListener("hello", (e) => {
    if (bootID === null) {
      bootID = e.data;
      return;
    }
    if (e.data !== bootID) location.reload();
  });
  es.addEventListener("reload", () => location.reload());
})();
`

// newBootID identifies one server process, so the browser can tell a reconnect
// to the same process from a reconnect to a freshly restarted one.
func newBootID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (s *Server) liveReloadBootID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bootID == "" {
		s.bootID = newBootID()
	}
	return s.bootID
}

func (s *Server) handleLiveReloadScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript")
	if !s.Dev {
		// Empty no-op so the dashboard <script> tag does not 404 without -dev.
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, liveReloadScript)
}

func (s *Server) handleLiveReload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	if err := rc.Flush(); err != nil {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Baseline before hello so a client that writes on connect cannot
	// sneak a change into prev and miss the reload event.
	prev := dirFingerprint(s.assetDir())

	// Tighten the browser's reconnect delay from its ~3s default so a
	// wgo-triggered restart is noticed quickly.
	if _, err := fmt.Fprint(w, "retry: 500\n\n"); err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "event: hello\ndata: %s\n\n", s.liveReloadBootID()); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		return
	}

	ticker := time.NewTicker(liveReloadPoll)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			cur := dirFingerprint(s.assetDir())
			if cur == prev {
				continue
			}
			if _, err := fmt.Fprint(w, "event: reload\ndata: 1\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
			prev = cur
		}
	}
}

// noCache stops the browser reusing a cached asset after an edit.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
