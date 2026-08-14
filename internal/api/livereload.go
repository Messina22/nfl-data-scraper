package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"nfl-data-scraper/web"
)

// liveReloadPoll is how often the server restats the static directory.
const liveReloadPoll = 300 * time.Millisecond

// liveReloadScript reloads the page on a static-asset change, and also when the
// boot ID changes — which happens when wgo rebuilds and restarts the server after
// a Go edit. EventSource reconnects on its own, so the restart needs no extra code.
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

func (s *Server) handleLiveReloadScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, liveReloadScript)
}

func (s *Server) handleLiveReload(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")

	fmt.Fprintf(w, "event: hello\ndata: %s\n\n", s.bootID)
	flusher.Flush()

	ticker := time.NewTicker(liveReloadPoll)
	defer ticker.Stop()
	prev := dirFingerprint(web.StaticDir)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			cur := dirFingerprint(web.StaticDir)
			if cur == prev {
				continue
			}
			prev = cur
			fmt.Fprint(w, "event: reload\ndata: 1\n\n")
			flusher.Flush()
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
