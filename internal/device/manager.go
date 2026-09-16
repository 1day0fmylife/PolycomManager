package device

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"polycom-manager/internal/events"
	"polycom-manager/internal/polycom"
	"polycom-manager/internal/sshclient"
)

type Repository interface {
	ListDevices(context.Context) ([]Device, error)
	GetDevice(context.Context, string) (Device, error)
	GetPassword(context.Context, string) (string, error)
	AddAudit(context.Context, string, string, string, string, string) error
}

type Manager struct {
	repo     Repository
	hostKeys *sshclient.HostKeyStore
	events   *events.Hub
	log      *slog.Logger
	mu       sync.RWMutex
	workers  map[string]*Worker
	states   map[string]RuntimeState
}

func NewManager(repo Repository, hostKeys *sshclient.HostKeyStore, hub *events.Hub, log *slog.Logger) *Manager {
	return &Manager{repo: repo, hostKeys: hostKeys, events: hub, log: log, workers: map[string]*Worker{}, states: map[string]RuntimeState{}}
}

func (m *Manager) Start(ctx context.Context) error {
	devices, err := m.repo.ListDevices(ctx)
	if err != nil {
		return err
	}
	for _, d := range devices {
		m.states[d.ID] = RuntimeState{Connection: "disconnected"}
		if d.Enabled {
			m.startWorker(ctx, d)
		}
	}
	return nil
}

func (m *Manager) Upsert(ctx context.Context, d Device) {
	m.StopDevice(d.ID)
	m.mu.Lock()
	m.states[d.ID] = RuntimeState{Connection: "disconnected"}
	m.mu.Unlock()
	if d.Enabled {
		m.startWorker(ctx, d)
	}
	m.publish(d.ID, "device.updated", m.State(d.ID))
}

func (m *Manager) Delete(id string) {
	m.StopDevice(id)
	m.mu.Lock()
	delete(m.states, id)
	m.mu.Unlock()
	m.publish(id, "device.deleted", nil)
}

func (m *Manager) startWorker(parent context.Context, d Device) {
	ctx, cancel := context.WithCancel(parent)
	w := &Worker{device: d, repo: m.repo, hostKeys: m.hostKeys, log: m.log, update: m.updateState, publish: m.publish, cancel: cancel}
	m.mu.Lock()
	m.workers[d.ID] = w
	m.mu.Unlock()
	go w.Run(ctx)
}

func (m *Manager) StopDevice(id string) {
	m.mu.Lock()
	w := m.workers[id]
	delete(m.workers, id)
	m.mu.Unlock()
	if w != nil {
		w.Stop()
	}
	m.updateState(id, func(s *RuntimeState) { s.Connection = "disconnected" })
}

func (m *Manager) Restart(ctx context.Context, id string) error {
	d, err := m.repo.GetDevice(ctx, id)
	if err != nil {
		return err
	}
	m.StopDevice(id)
	m.startWorker(ctx, d)
	return nil
}

func (m *Manager) State(id string) RuntimeState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[id]
}

func (m *Manager) Views(devices []Device) []DeviceView {
	out := make([]DeviceView, 0, len(devices))
	for _, d := range devices {
		out = append(out, DeviceView{Device: d, Runtime: m.State(d.ID)})
	}
	return out
}

func (m *Manager) updateState(id string, fn func(*RuntimeState)) {
	m.mu.Lock()
	s := m.states[id]
	fn(&s)
	m.states[id] = s
	m.mu.Unlock()
	m.publish(id, "device.state", s)
}

func (m *Manager) publish(id, typ string, data any) {
	m.events.Publish(events.Event{Type: typ, DeviceID: id, Data: data})
}

func (m *Manager) worker(id string) (*Worker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w := m.workers[id]
	if w == nil {
		return nil, errors.New("device worker is not running")
	}
	return w, nil
}

func (m *Manager) Raw(ctx context.Context, id, cmd string) ([]string, error) {
	w, e := m.worker(id)
	if e != nil {
		return nil, e
	}
	return w.Raw(ctx, cmd)
}

func (m *Manager) Dial(ctx context.Context, id, dst string, speed int) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	return w.Do(ctx, "dial", func(c *polycom.Client) error { return c.Dial(ctx, dst, speed) })
}

func (m *Manager) Hangup(ctx context.Context, id string) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	return w.Do(ctx, "hangup", func(c *polycom.Client) error { return c.Hangup(ctx) })
}

func (m *Manager) SetMute(ctx context.Context, id string, v bool) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	return w.Do(ctx, "mute", func(c *polycom.Client) error { return c.SetMute(ctx, v) })
}

func (m *Manager) SetVolume(ctx context.Context, id string, v int) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	return w.Do(ctx, "volume", func(c *polycom.Client) error { return c.SetVolume(ctx, v) })
}

func (m *Manager) CameraSelect(ctx context.Context, id, site string, source int) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	return w.Do(ctx, "camera.select", func(c *polycom.Client) error { return c.SelectCamera(ctx, site, source) })
}

func (m *Manager) CameraMove(ctx context.Context, id, site, direction string) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	return w.Do(ctx, "camera.move", func(c *polycom.Client) error { return c.CameraMove(ctx, site, direction) })
}

func (m *Manager) CameraPreset(ctx context.Context, id, site, action string, preset int) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	return w.Do(ctx, "camera.preset", func(c *polycom.Client) error { return c.CameraPreset(ctx, site, action, preset) })
}

func (m *Manager) SendDTMF(ctx context.Context, id, digit string) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	return w.Do(ctx, "dtmf", func(c *polycom.Client) error { return c.SendDTMF(ctx, digit) })
}

func (m *Manager) Content(ctx context.Context, id, action string, source int) error {
	w, e := m.worker(id)
	if e != nil {
		return e
	}
	switch action {
	case "play":
		return w.Do(ctx, "content.play", func(c *polycom.Client) error { return c.StartContent(ctx, source) })
	case "stop":
		return w.Do(ctx, "content.stop", func(c *polycom.Client) error { return c.StopContent(ctx) })
	default:
		return errors.New("content action must be play or stop")
	}
}

type Worker struct {
	device   Device
	repo     Repository
	hostKeys *sshclient.HostKeyStore
	log      *slog.Logger
	update   func(string, func(*RuntimeState))
	publish  func(string, string, any)
	cancel   context.CancelFunc
	mu       sync.RWMutex
	ssh      *sshclient.Session
	client   *polycom.Client
}

func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.mu.Lock()
	if w.ssh != nil {
		_ = w.ssh.Close()
	}
	w.ssh = nil
	w.client = nil
	w.mu.Unlock()
}

func (w *Worker) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		w.update(w.device.ID, func(s *RuntimeState) { s.Connection = "connecting"; s.LastError = "" })
		err := w.connectAndServe(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			state := "reconnecting"
			if sshclient.IsAuthError(err) {
				state = "auth_error"
			}
			w.update(w.device.ID, func(s *RuntimeState) { s.Connection = state; s.LastError = err.Error() })
			_ = w.repo.AddAudit(context.Background(), w.device.ID, "connect", "", "error", err.Error())
			if state == "auth_error" {
				return
			}
		}
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		if backoff < 60*time.Second {
			backoff *= 2
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
		}
	}
}

func (w *Worker) connectAndServe(ctx context.Context) error {
	password, err := w.repo.GetPassword(ctx, w.device.CredentialID)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(w.device.Host, fmt.Sprintf("%d", w.device.Port))
	sess, err := sshclient.Dial(ctx, sshclient.DialConfig{Address: addr, Username: w.device.Username, Password: password, Timeout: 7 * time.Second, HostKeys: w.hostKeys, OnNotify: w.onNotification})
	if err != nil {
		return err
	}
	client := polycom.New(sess)
	if err := client.Init(ctx, "pm-"+shortID(w.device.ID)); err != nil {
		_ = sess.Close()
		return err
	}
	w.mu.Lock()
	w.ssh = sess
	w.client = client
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		if w.ssh == sess {
			w.ssh = nil
			w.client = nil
		}
		w.mu.Unlock()
		_ = sess.Close()
	}()
	now := time.Now().UTC()
	w.update(w.device.ID, func(s *RuntimeState) {
		s.Connection = "connected"
		s.LastConnectedAt = now
		s.LastSeenAt = now
		s.LastError = ""
	})
	_ = w.repo.AddAudit(context.Background(), w.device.ID, "connect", "", "success", "")
	if err := w.refresh(ctx); err != nil {
		w.log.Warn("initial device refresh failed", "device", w.device.ID, "error", err)
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.refresh(ctx); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) current() (*polycom.Client, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.client == nil {
		return nil, errors.New("device is not connected")
	}
	return w.client, nil
}

func (w *Worker) refresh(ctx context.Context) error {
	c, err := w.current()
	if err != nil {
		return err
	}
	info, err := c.SystemInfo(ctx)
	if err != nil {
		return err
	}
	calls, err := c.CallInfo(ctx)
	if err != nil {
		return err
	}
	muted, _ := c.MuteState(ctx)
	volume, _ := c.Volume(ctx)
	content, _ := c.ContentStatus(ctx)
	nearSource, _ := c.CameraSource(ctx, "near")
	farSource := 0
	if len(calls) > 0 {
		farSource, _ = c.CameraSource(ctx, "far")
	}
	now := time.Now().UTC()
	w.update(w.device.ID, func(s *RuntimeState) {
		oldStarts := make(map[string]time.Time, len(s.Calls))
		for _, call := range s.Calls {
			if !call.StartedAt.IsZero() {
				oldStarts[call.CallID] = call.StartedAt
			}
		}

		s.DetectedModel = info.Model
		s.SystemName = info.SystemName
		s.Firmware = info.Firmware
		s.Serial = info.Serial
		s.LastSeenAt = now
		s.Muted = muted
		s.Volume = volume
		s.ContentState = content.State
		s.ContentSource = content.Source
		s.NearCameraSource = nearSource
		s.FarCameraSource = farSource
		s.CallState = "idle"
		s.RemoteParty = ""
		s.Calls = make([]CallState, 0, len(calls))
		for _, call := range calls {
			started := oldStarts[call.CallID]
			if started.IsZero() {
				started = now
			}
			view := CallState{
				CallID:           call.CallID,
				FarSiteName:      call.FarSiteName,
				FarSiteNumber:    call.FarSiteNumber,
				Speed:            call.Speed,
				ConnectionStatus: call.ConnectionStatus,
				MuteStatus:       call.MuteStatus,
				Direction:        call.Direction,
				Type:             call.Type,
				Protocol:         inferProtocol(call.FarSiteNumber),
				StartedAt:        started,
				DurationSeconds:  int64(now.Sub(started).Seconds()),
			}
			s.Calls = append(s.Calls, view)
		}
		if len(s.Calls) > 0 {
			s.CallState = s.Calls[0].ConnectionStatus
			s.RemoteParty = s.Calls[0].FarSiteName
			if s.RemoteParty == "" {
				s.RemoteParty = s.Calls[0].FarSiteNumber
			}
		}
	})
	return nil
}

func (w *Worker) Raw(ctx context.Context, cmd string) ([]string, error) {
	c, e := w.current()
	if e != nil {
		return nil, e
	}
	lines, e := c.Raw(ctx, cmd)
	res := "success"
	detail := strings.Join(lines, "\n")
	if e != nil {
		res = "error"
		detail = e.Error()
	}
	_ = w.repo.AddAudit(context.Background(), w.device.ID, "raw", cmd, res, detail)
	return lines, e
}

func (w *Worker) Do(ctx context.Context, op string, fn func(*polycom.Client) error) error {
	c, e := w.current()
	if e != nil {
		return e
	}
	e = fn(c)
	res := "success"
	detail := ""
	if e != nil {
		res = "error"
		detail = e.Error()
	}
	_ = w.repo.AddAudit(context.Background(), w.device.ID, op, "", res, detail)
	if e == nil {
		go func() {
			cctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_ = w.refresh(cctx)
		}()
	}
	return e
}

func (w *Worker) onNotification(line string) {
	lower := strings.ToLower(strings.TrimSpace(line))
	w.update(w.device.ID, func(s *RuntimeState) {
		s.LastSeenAt = time.Now().UTC()
		if strings.HasPrefix(lower, "notification:callstatus:") {
			parts := strings.Split(line, ":")
			if len(parts) > 6 {
				s.CallState = parts[6]
			}
			if len(parts) > 4 {
				s.RemoteParty = parts[4]
			}
		}
		if strings.HasPrefix(lower, "notification:mutestatus:") {
			v := strings.Contains(lower, "muted") && !strings.Contains(lower, "unmuted")
			s.Muted = &v
		}
		if strings.HasPrefix(lower, "control event: vcbutton play") {
			s.ContentState = "play"
		}
		if strings.HasPrefix(lower, "control event: vcbutton stop") {
			s.ContentState = "stop"
			s.ContentSource = 0
		}
		if strings.HasPrefix(lower, "control event: vcbutton source ") {
			value := strings.TrimSpace(strings.TrimPrefix(lower, "control event: vcbutton source "))
			if n, err := strconv.Atoi(value); err == nil {
				s.ContentSource = n
			}
		}
	})
	w.publish(w.device.ID, "polycom.notification", line)
}

func inferProtocol(number string) string {
	v := strings.ToLower(strings.TrimSpace(number))
	switch {
	case strings.HasPrefix(v, "sip:") || strings.Contains(v, "@"):
		return "SIP"
	case strings.HasPrefix(v, "h323:"):
		return "H.323"
	case net.ParseIP(v) != nil:
		return "IP"
	default:
		return ""
	}
}

func shortID(v string) string {
	if len(v) > 10 {
		return v[:10]
	}
	return v
}
