package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/store"
)

type Server struct {
	store      store.Store
	socketPath string
	listener   net.Listener
	httpServer *http.Server
	startTime  time.Time
	version    string
}

type PingResponse struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	StartTime time.Time `json:"startTime"`
	Uptime    string    `json:"uptime"`
}

type ManifestSummary struct {
	Digest      string               `json:"digest"`
	Name        string               `json:"name"`
	Version     string               `json:"version"`
	Publisher   string               `json:"publisher"`
	Description string               `json:"description"`
	LayerCount  int                  `json:"layerCount"`
	Signatures  []manifest.Signature `json:"signatures"`
}

func NewServer(s store.Store, socketPath string, version string) *Server {
	srv := &Server{
		store:      s,
		socketPath: socketPath,
		startTime:  time.Now(),
		version:    version,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ping", srv.handlePing)
	mux.HandleFunc("/v1/manifests", srv.handleManifests)
	mux.HandleFunc("/v1/manifests/", srv.handleManifestByDigest)

	srv.httpServer = &http.Server{
		Handler: mux,
	}

	return srv
}

func (s *Server) Start() error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove any stale socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	l, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listed on socket %s: %w", s.socketPath, err)
	}

	s.listener = l

	if err := os.Chmod(s.socketPath, 0600); err != nil {
		l.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	return s.httpServer.Serve(l)
}

func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	if s.httpServer != nil {
		err = s.httpServer.Shutdown(ctx)
	}
	if s.socketPath != "" {
		_ = os.Remove(s.socketPath)
	}
	return err
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := PingResponse{
		Status:    "ok",
		Version:   s.version,
		StartTime: s.startTime,
		Uptime:    time.Since(s.startTime).String(),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleManifests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		digests, err := s.store.ListManifests(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		summaries := make([]ManifestSummary, 0, len(digests))
		for _, d := range digests {
			m, err := s.store.GetManifest(ctx, d)
			if err != nil {
				continue
			}
			summaries = append(summaries, ManifestSummary{
				Digest:      d.String(),
				Name:        m.Metadata.Name,
				Version:     m.Metadata.Version,
				Publisher:   m.Metadata.Publisher,
				Description: m.Metadata.Description,
				LayerCount:  len(m.Layers),
				Signatures:  m.Signatures,
			})
		}
		writeJSON(w, http.StatusOK, summaries)

	case http.MethodPost:
		var m manifest.Manifest
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, fmt.Sprintf("invalid manifest JSON: %v", err), http.StatusBadRequest)
			return
		}

		digest, err := s.store.PutManifest(ctx, &m)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to store manifest: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"digest": digest.String()})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleManifestByDigest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	prefix := "/v1/manifests/"
	path := r.URL.Path
	if path == "" {
		path = r.URL.RawPath
	}
	digestStr := strings.TrimPrefix(path, prefix)
	if digestStr == "" {
		http.Error(w, "digest is required", http.StatusBadRequest)
		return
	}

	d := store.Digest(digestStr)
	if err := d.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid digest format: %v", err), http.StatusBadRequest)
		return
	}

	m, err := s.store.GetManifest(r.Context(), d)
	if err != nil {
		if errors.Is(err, store.ErrManifestNotFound) {
			http.Error(w, "manifest not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, m)
}

func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
