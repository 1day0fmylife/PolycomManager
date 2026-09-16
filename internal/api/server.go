package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"polycom-manager/internal/db"
	"polycom-manager/internal/device"
	"polycom-manager/internal/events"
)

type Repository interface {
	ListDevices(context.Context) ([]device.Device, error)
	GetDevice(context.Context, string) (device.Device, error)
	CreateDevice(context.Context, device.UpsertInput) (device.Device, error)
	UpdateDevice(context.Context, string, device.UpsertInput) (device.Device, error)
	DeleteDevice(context.Context, string) error
	ListAudit(context.Context, int) ([]device.AuditEntry, error)
	AddAudit(context.Context, string, string, string, string, string) error
}

type Server struct {
	repo    Repository
	manager *device.Manager
	hub     *events.Hub
	log     *slog.Logger
	static  http.Handler
	rootCtx context.Context
}

func New(rootCtx context.Context, repo Repository, manager *device.Manager, hub *events.Hub, static http.Handler, log *slog.Logger) *Server {
	return &Server{rootCtx: rootCtx, repo: repo, manager: manager, hub: hub, static: static, log: log}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/devices", s.listDevices)
	mux.HandleFunc("POST /api/v1/devices", s.createDevice)
	mux.HandleFunc("GET /api/v1/devices/{id}", s.getDevice)
	mux.HandleFunc("PUT /api/v1/devices/{id}", s.updateDevice)
	mux.HandleFunc("PATCH /api/v1/devices/{id}", s.updateDevice)
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.deleteDevice)
	mux.HandleFunc("POST /api/v1/devices/{id}/connect", s.connectDevice)
	mux.HandleFunc("POST /api/v1/devices/{id}/disconnect", s.disconnectDevice)
	mux.HandleFunc("POST /api/v1/devices/{id}/dial", s.dial)
	mux.HandleFunc("POST /api/v1/devices/{id}/hangup", s.hangup)
	mux.HandleFunc("POST /api/v1/devices/{id}/mute", s.mute)
	mux.HandleFunc("POST /api/v1/devices/{id}/volume", s.volume)
	mux.HandleFunc("POST /api/v1/devices/{id}/camera/select", s.cameraSelect)
	mux.HandleFunc("POST /api/v1/devices/{id}/camera/move", s.cameraMove)
	mux.HandleFunc("POST /api/v1/devices/{id}/camera/preset", s.cameraPreset)
	mux.HandleFunc("POST /api/v1/devices/{id}/dtmf", s.dtmf)
	mux.HandleFunc("POST /api/v1/devices/{id}/content", s.content)
	mux.HandleFunc("POST /api/v1/devices/{id}/command", s.command)
	mux.HandleFunc("GET /api/v1/audit", s.audit)
	mux.HandleFunc("GET /api/v1/events", s.sse)
	mux.Handle("/", s.static)
	return s.logging(s.securityHeaders(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}
func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	ds, e := s.repo.ListDevices(r.Context())
	if e != nil {
		writeError(w, e, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Views(ds))
}
func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	d, e := s.repo.GetDevice(r.Context(), r.PathValue("id"))
	if e != nil {
		s.repoErr(w, e)
		return
	}
	writeJSON(w, http.StatusOK, device.DeviceView{Device: d, Runtime: s.manager.State(d.ID)})
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var in device.UpsertInput
	if !decode(w, r, &in) {
		return
	}
	in.Normalize()
	d, e := s.repo.CreateDevice(r.Context(), in)
	if e != nil {
		writeError(w, e, http.StatusBadRequest)
		return
	}
	s.manager.Upsert(s.rootCtx, d)
	_ = s.repo.AddAudit(r.Context(), d.ID, "device.create", "", "success", d.Host)
	writeJSON(w, http.StatusCreated, device.DeviceView{Device: d, Runtime: s.manager.State(d.ID)})
}
func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	var in device.UpsertInput
	if !decode(w, r, &in) {
		return
	}
	in.Normalize()
	d, e := s.repo.UpdateDevice(r.Context(), r.PathValue("id"), in)
	if e != nil {
		s.repoErr(w, e)
		return
	}
	s.manager.Upsert(s.rootCtx, d)
	_ = s.repo.AddAudit(r.Context(), d.ID, "device.update", "", "success", d.Host)
	writeJSON(w, http.StatusOK, device.DeviceView{Device: d, Runtime: s.manager.State(d.ID)})
}
func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if e := s.repo.DeleteDevice(r.Context(), id); e != nil {
		s.repoErr(w, e)
		return
	}
	s.manager.Delete(id)
	_ = s.repo.AddAudit(r.Context(), id, "device.delete", "", "success", "")
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) connectDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if e := s.manager.Restart(s.rootCtx, id); e != nil {
		s.repoErr(w, e)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}
func (s *Server) disconnectDevice(w http.ResponseWriter, r *http.Request) {
	s.manager.StopDevice(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
}

func (s *Server) dial(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Destination string `json:"destination"`
		Speed       int    `json:"speed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := s.manager.Dial(r.Context(), r.PathValue("id"), in.Destination, in.Speed); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (s *Server) hangup(w http.ResponseWriter, r *http.Request) {
	if e := s.manager.Hangup(r.Context(), r.PathValue("id")); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (s *Server) mute(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Muted bool `json:"muted"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := s.manager.SetMute(r.Context(), r.PathValue("id"), in.Muted); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (s *Server) volume(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Volume int `json:"volume"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := s.manager.SetVolume(r.Context(), r.PathValue("id"), in.Volume); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (s *Server) cameraSelect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Site   string `json:"site"`
		Source int    `json:"source"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := s.manager.CameraSelect(r.Context(), r.PathValue("id"), strings.ToLower(strings.TrimSpace(in.Site)), in.Source); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) cameraMove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Site      string `json:"site"`
		Direction string `json:"direction"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := s.manager.CameraMove(r.Context(), r.PathValue("id"), strings.ToLower(strings.TrimSpace(in.Site)), strings.ToLower(strings.TrimSpace(in.Direction))); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) cameraPreset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Site   string `json:"site"`
		Action string `json:"action"`
		Preset int    `json:"preset"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := s.manager.CameraPreset(r.Context(), r.PathValue("id"), strings.ToLower(strings.TrimSpace(in.Site)), strings.ToLower(strings.TrimSpace(in.Action)), in.Preset); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) dtmf(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Digit string `json:"digit"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := s.manager.SendDTMF(r.Context(), r.PathValue("id"), strings.TrimSpace(in.Digit)); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) content(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
		Source int    `json:"source"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := s.manager.Content(r.Context(), r.PathValue("id"), strings.ToLower(strings.TrimSpace(in.Action)), in.Source); e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Command string `json:"command"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Command) > 512 || strings.ContainsAny(in.Command, "\r\n\x00") {
		writeError(w, errors.New("invalid command"), http.StatusBadRequest)
		return
	}
	lines, e := s.manager.Raw(r.Context(), r.PathValue("id"), in.Command)
	if e != nil {
		writeError(w, e, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"output": lines})
}
func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, e := s.repo.ListAudit(r.Context(), limit)
	if e != nil {
		writeError(w, e, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) sse(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, errors.New("stream unsupported"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, ch, unsubscribe := s.hub.Subscribe()
	defer unsubscribe()
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()
	keep := time.NewTicker(20 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			_, _ = w.Write(events.EncodeSSE(e))
			flusher.Flush()
		case <-keep.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if e := dec.Decode(v); e != nil {
		writeError(w, e, http.StatusBadRequest)
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, e error, status int) {
	writeJSON(w, status, map[string]string{"error": e.Error()})
}
func (s *Server) repoErr(w http.ResponseWriter, e error) {
	if db.IsNotFound(e) {
		writeError(w, e, http.StatusNotFound)
		return
	}
	writeError(w, e, http.StatusBadRequest)
}
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/api/v1/events") {
			s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
		}
	})
}
