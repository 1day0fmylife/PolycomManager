package device

import (
	"errors"
	"net"
	"strings"
	"time"
)

type Device struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	Model        string    `json:"model"`
	Location     string    `json:"location,omitempty"`
	Username     string    `json:"username"`
	CredentialID string    `json:"-"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UpsertInput struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Model    string `json:"model"`
	Location string `json:"location"`
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  bool   `json:"enabled"`
}

func (i *UpsertInput) Normalize() {
	i.Name = strings.TrimSpace(i.Name)
	i.Host = strings.TrimSpace(i.Host)
	i.Username = strings.TrimSpace(i.Username)
	i.Location = strings.TrimSpace(i.Location)
	i.Model = strings.ToLower(strings.TrimSpace(i.Model))
	if i.Port == 0 {
		i.Port = 22
	}
	if i.Model == "" {
		i.Model = "auto"
	}
}

func (i *UpsertInput) Validate(requirePassword bool) error {
	i.Normalize()
	if i.Name == "" {
		return errors.New("name is required")
	}
	if i.Host == "" {
		return errors.New("host is required")
	}
	if i.Port < 1 || i.Port > 65535 {
		return errors.New("invalid SSH port")
	}
	if i.Username == "" {
		return errors.New("username is required")
	}
	if requirePassword && i.Password == "" {
		return errors.New("password is required")
	}
	switch i.Model {
	case "auto", "group300", "group500", "group700":
	default:
		return errors.New("model must be auto, group300, group500 or group700")
	}
	if strings.ContainsAny(i.Host, "\r\n\x00") {
		return errors.New("invalid host")
	}
	if ip := net.ParseIP(i.Host); ip == nil && strings.ContainsAny(i.Host, " /\\") {
		return errors.New("invalid host")
	}
	return nil
}

type RuntimeState struct {
	Connection      string    `json:"connection"`
	DetectedModel   string    `json:"detected_model,omitempty"`
	SystemName      string    `json:"system_name,omitempty"`
	Firmware        string    `json:"firmware,omitempty"`
	Serial          string    `json:"serial,omitempty"`
	CallState       string    `json:"call_state,omitempty"`
	RemoteParty     string    `json:"remote_party,omitempty"`
	Muted           *bool     `json:"muted,omitempty"`
	Volume          *int      `json:"volume,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	LastSeenAt      time.Time `json:"last_seen_at,omitempty"`
	LastConnectedAt time.Time `json:"last_connected_at,omitempty"`
}

type DeviceView struct {
	Device
	Runtime RuntimeState `json:"runtime"`
}

type AuditEntry struct {
	ID        int64     `json:"id"`
	DeviceID  string    `json:"device_id,omitempty"`
	Operation string    `json:"operation"`
	Command   string    `json:"command,omitempty"`
	Result    string    `json:"result"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
