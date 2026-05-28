// Package dashboard serves the koto web UI and its REST + SSE API. It is
// embedded in the koto binary so the whole dashboard ships as zero-dep static
// assets plus a small Go HTTP server.
package dashboard

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

//go:embed all:web
var webFS embed.FS

// Server is the dashboard HTTP server.
type Server struct {
	addr  string
	mux   *http.ServeMux
	srv   *http.Server
	open  bool
	rootD string // project root (where .koto/runs/ lives)
}

// New builds a Server bound to addr. rootDir defaults to the current working dir.
func New(addr, rootDir string, openBrowser bool) (*Server, error) {
	if rootDir == "" {
		d, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		rootDir = d
	}
	s := &Server{addr: addr, mux: http.NewServeMux(), open: openBrowser, rootD: rootDir}
	s.routes()
	return s, nil
}

// Start binds the listener and serves until ctx is cancelled or Stop is called.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("bind %s: %w", s.addr, err)
	}
	s.srv = &http.Server{Handler: withCORS(s.mux), ReadHeaderTimeout: 5 * time.Second}

	url := fmt.Sprintf("http://%s/", ln.Addr().String())
	fmt.Fprintf(os.Stderr, "koto dashboard: %s  (root: %s)\n", url, s.rootD)
	if s.open {
		go openInBrowser(url)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- s.srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// routes wires HTTP handlers. Static assets fall back to / for SPA routing.
func (s *Server) routes() {
	// API
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/runs", s.handleRuns)               // GET list
	s.mux.HandleFunc("/api/runs/", s.handleRunDetail)         // GET /api/runs/{id} or /stream
	s.mux.HandleFunc("/api/workflows", s.handleWorkflowsList) // GET list
	s.mux.HandleFunc("/api/workflows/", s.handleWorkflowItem) // GET/PUT/DELETE/POST per workflow
	s.mux.HandleFunc("/api/validate", s.handleValidate)       // POST yaml → result
	s.mux.HandleFunc("/api/run", s.handleStartRun)            // POST start a new run
	s.mux.HandleFunc("/api/stats", s.handleStats)             // GET aggregate run stats

	// Static UI (embedded; SPA fallback to index.html)
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err) // build-time embed should never fail
	}
	s.mux.Handle("/", spaHandler(http.FS(sub)))
}

// handleHealth is a tiny liveness endpoint.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "root": s.rootD})
}

// openInBrowser is best-effort; failures are silent.
func openInBrowser(url string) {
	time.Sleep(150 * time.Millisecond) // let the server start
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
