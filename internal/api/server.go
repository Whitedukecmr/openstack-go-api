// Package api exposes the Cloud interface as a JSON REST API.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Whitedukecmr/openstack-go-api/internal/cloud"
)

type Server struct {
	cloud cloud.Cloud
	token string
	log   *slog.Logger
}

// New returns an http.Handler. If token is non-empty, /api/* requires "Authorization: Bearer <token>".
func New(c cloud.Cloud, token string, log *slog.Logger) http.Handler {
	s := &Server{cloud: c, token: token, log: log}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/servers", s.listServers)
	api.HandleFunc("POST /api/v1/servers", s.createServer)
	api.HandleFunc("GET /api/v1/servers/{id}", s.getServer)
	api.HandleFunc("DELETE /api/v1/servers/{id}", s.deleteServer)
	api.HandleFunc("POST /api/v1/servers/{id}/{action}", s.serverAction)

	api.HandleFunc("GET /api/v1/flavors", s.listFlavors)
	api.HandleFunc("GET /api/v1/images", s.listImages)

	api.HandleFunc("GET /api/v1/networks", s.listNetworks)
	api.HandleFunc("POST /api/v1/networks", s.createNetwork)
	api.HandleFunc("DELETE /api/v1/networks/{id}", s.deleteNetwork)

	api.HandleFunc("GET /api/v1/volumes", s.listVolumes)
	api.HandleFunc("POST /api/v1/volumes", s.createVolume)
	api.HandleFunc("DELETE /api/v1/volumes/{id}", s.deleteVolume)

	mux.Handle("/api/", s.auth(api))
	return s.logging(mux)
}

// ---------- middleware ----------

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(), "remote", ip)
	})
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeErr(w, http.StatusUnauthorized, "missing or invalid bearer token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, cloud.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.log.Error("backend error", "err", err)
	writeErr(w, http.StatusBadGateway, err.Error())
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// ---------- servers ----------

func (s *Server) listServers(w http.ResponseWriter, r *http.Request) {
	out, err := s.cloud.ListServers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getServer(w http.ResponseWriter, r *http.Request) {
	out, err := s.cloud.GetServer(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createServer(w http.ResponseWriter, r *http.Request) {
	var req cloud.CreateServerRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Name == "" || req.FlavorID == "" || req.ImageID == "" || req.NetworkID == "" {
		writeErr(w, http.StatusBadRequest, "name, flavor_id, image_id and network_id are required")
		return
	}
	out, err := s.cloud.CreateServer(r.Context(), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request) {
	if err := s.cloud.DeleteServer(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serverAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	switch action {
	case "start", "stop", "reboot":
	default:
		writeErr(w, http.StatusBadRequest, "action must be start, stop or reboot")
		return
	}
	if err := s.cloud.ServerAction(r.Context(), r.PathValue("id"), action); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": r.PathValue("id"), "action": action})
}

// ---------- catalog ----------

func (s *Server) listFlavors(w http.ResponseWriter, r *http.Request) {
	out, err := s.cloud.ListFlavors(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listImages(w http.ResponseWriter, r *http.Request) {
	out, err := s.cloud.ListImages(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------- networks ----------

func (s *Server) listNetworks(w http.ResponseWriter, r *http.Request) {
	out, err := s.cloud.ListNetworks(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createNetwork(w http.ResponseWriter, r *http.Request) {
	var req cloud.CreateNetworkRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if _, _, err := net.ParseCIDR(req.CIDR); err != nil {
		writeErr(w, http.StatusBadRequest, "cidr must be a valid CIDR, e.g. 10.10.0.0/24")
		return
	}
	out, err := s.cloud.CreateNetwork(r.Context(), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	if err := s.cloud.DeleteNetwork(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- volumes ----------

func (s *Server) listVolumes(w http.ResponseWriter, r *http.Request) {
	out, err := s.cloud.ListVolumes(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createVolume(w http.ResponseWriter, r *http.Request) {
	var req cloud.CreateVolumeRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Name == "" || req.SizeGB <= 0 {
		writeErr(w, http.StatusBadRequest, "name and size_gb > 0 are required")
		return
	}
	out, err := s.cloud.CreateVolume(r.Context(), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (s *Server) deleteVolume(w http.ResponseWriter, r *http.Request) {
	if err := s.cloud.DeleteVolume(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
