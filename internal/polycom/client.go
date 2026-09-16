package polycom

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"polycom-manager/internal/sshclient"
)

type Client struct{ ssh *sshclient.Session }

func New(s *sshclient.Session) *Client { return &Client{ssh: s} }

func (c *Client) Raw(ctx context.Context, command string) ([]string, error) {
	return c.ssh.Execute(ctx, command)
}

func (c *Client) Init(ctx context.Context, sessionName string) error {
	if _, err := c.Raw(ctx, fmt.Sprintf("session name %s", safeToken(sessionName))); err != nil {
		return fmt.Errorf("set session name: %w", err)
	}
	for _, cmd := range []string{
		"notify callstatus",
		"notify mutestatus",
		"notify sysstatus",
		"notify vidsourcechanges",
		"vcbutton register",
	} {
		_, _ = c.Raw(ctx, cmd)
	}
	return nil
}

func (c *Client) SystemInfo(ctx context.Context) (SystemInfo, error) {
	var info SystemInfo
	queries := []struct {
		cmd string
		set func([]string)
	}{
		{"systemsetting get model", func(v []string) { info.Model = extractQuotedOrTail(v, "systemsetting model") }},
		{"systemname get", func(v []string) { info.SystemName = extractQuotedOrTail(v, "systemname") }},
		{"version", func(v []string) { info.Firmware = extractQuotedOrTail(v, "version") }},
		{"serialnum", func(v []string) { info.Serial = extractQuotedOrTail(v, "serialnum") }},
	}
	for _, q := range queries {
		lines, err := c.Raw(ctx, q.cmd)
		if err != nil {
			return info, err
		}
		q.set(lines)
	}
	return info, nil
}

func (c *Client) CallInfo(ctx context.Context) ([]CallInfo, error) {
	lines, err := c.Raw(ctx, "callinfo all")
	if err != nil {
		return nil, err
	}
	return ParseCallInfo(lines), nil
}

func (c *Client) MuteState(ctx context.Context) (*bool, error) {
	lines, err := c.Raw(ctx, "mute near get")
	if err != nil {
		return nil, err
	}
	joined := strings.ToLower(strings.Join(lines, " "))
	if strings.Contains(joined, "unmuted") || strings.Contains(joined, " off") {
		v := false
		return &v, nil
	}
	if strings.Contains(joined, "muted") || strings.Contains(joined, " on") {
		v := true
		return &v, nil
	}
	return nil, nil
}

func (c *Client) Volume(ctx context.Context) (*int, error) {
	lines, err := c.Raw(ctx, "volume get")
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?i)volume\s+(\d+)`)
	for _, l := range lines {
		if m := re.FindStringSubmatch(l); len(m) == 2 {
			n, _ := strconv.Atoi(m[1])
			return &n, nil
		}
	}
	return nil, nil
}

func (c *Client) ContentStatus(ctx context.Context) (ContentStatus, error) {
	var out ContentStatus
	stateLines, err := c.Raw(ctx, "vcbutton get")
	if err != nil {
		return out, err
	}
	for _, line := range stateLines {
		lower := strings.ToLower(strings.TrimSpace(line))
		switch {
		case lower == "vcbutton play" || strings.HasPrefix(lower, "vcbutton play "):
			out.State = "play"
		case lower == "vcbutton stop":
			out.State = "stop"
		}
	}
	if out.State == "" {
		out.State = "unknown"
	}

	sourceLines, err := c.Raw(ctx, "vcbutton source get")
	if err != nil {
		return out, err
	}
	re := regexp.MustCompile(`(?i)^vcbutton\s+source\s+get\s+(\d+|none)$`)
	for _, line := range sourceLines {
		m := re.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) != 2 || strings.EqualFold(m[1], "none") {
			continue
		}
		out.Source, _ = strconv.Atoi(m[1])
	}
	return out, nil
}

func (c *Client) CameraSource(ctx context.Context, site string) (int, error) {
	if err := validateSite(site); err != nil {
		return 0, err
	}
	lines, err := c.Raw(ctx, "camera "+site+" source")
	if err != nil {
		return 0, err
	}
	re := regexp.MustCompile(`(?i)camera\s+(?:near|far)\s+source\s+(\d+)`)
	for _, line := range lines {
		if m := re.FindStringSubmatch(line); len(m) == 2 {
			n, _ := strconv.Atoi(m[1])
			return n, nil
		}
	}
	return 0, nil
}

func (c *Client) Dial(ctx context.Context, destination string, speed int) error {
	if speed <= 0 {
		speed = 512
	}
	if err := validateDialString(destination); err != nil {
		return err
	}
	return c.execOK(ctx, fmt.Sprintf(`dial manual %d "%s" sip`, speed, destination))
}

func (c *Client) Hangup(ctx context.Context) error { return c.execOK(ctx, "hangup all") }

func (c *Client) SetMute(ctx context.Context, muted bool) error {
	v := "off"
	if muted {
		v = "on"
	}
	return c.execOK(ctx, "mute near "+v)
}

func (c *Client) SetVolume(ctx context.Context, volume int) error {
	if volume < 0 || volume > 50 {
		return errors.New("volume must be between 0 and 50")
	}
	return c.execOK(ctx, fmt.Sprintf("volume set %d", volume))
}

func (c *Client) SelectCamera(ctx context.Context, site string, source int) error {
	if err := validateSite(site); err != nil {
		return err
	}
	if source < 1 || source > 4 {
		return errors.New("camera source must be between 1 and 4")
	}
	return c.execOK(ctx, fmt.Sprintf("camera %s %d", site, source))
}

func (c *Client) CameraMove(ctx context.Context, site, direction string) error {
	if err := validateSite(site); err != nil {
		return err
	}
	if !oneOf(direction, "left", "right", "up", "down", "zoom+", "zoom-", "stop") {
		return errors.New("invalid camera direction")
	}
	if direction == "stop" {
		return c.ssh.ExecuteNoWait(ctx, fmt.Sprintf("camera %s stop", site))
	}
	return c.execOK(ctx, fmt.Sprintf("camera %s move %s", site, direction))
}

func (c *Client) CameraPreset(ctx context.Context, site, action string, preset int) error {
	if err := validateSite(site); err != nil {
		return err
	}
	if !oneOf(action, "go", "set") {
		return errors.New("preset action must be go or set")
	}
	max := 99
	if site == "far" {
		max = 15
	}
	if preset < 0 || preset > max {
		return fmt.Errorf("preset must be between 0 and %d", max)
	}
	return c.execOK(ctx, fmt.Sprintf("preset %s %s %d", site, action, preset))
}

func (c *Client) SendDTMF(ctx context.Context, digit string) error {
	if len(digit) != 1 || !strings.Contains("0123456789*#", digit) {
		return errors.New("DTMF digit must be one of 0-9, * or #")
	}
	lines, err := c.ssh.ExecuteOptional(ctx, "gendial "+digit, 80*time.Millisecond)
	if err != nil {
		return err
	}
	return responseError(lines)
}

func (c *Client) StartContent(ctx context.Context, source int) error {
	if source < 1 || source > 6 || source == 5 {
		return errors.New("content source must be 1, 2, 3, 4 or 6")
	}
	return c.execOK(ctx, fmt.Sprintf("vcbutton play %d", source))
}

func (c *Client) StopContent(ctx context.Context) error {
	return c.execOK(ctx, "vcbutton stop")
}

func (c *Client) execOK(ctx context.Context, command string) error {
	lines, err := c.Raw(ctx, command)
	if err != nil {
		return err
	}
	return responseError(lines)
}

func responseError(lines []string) error {
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, "error:") || strings.Contains(lower, " failed") || strings.HasSuffix(lower, " failed") {
			return errors.New(strings.TrimSpace(line))
		}
	}
	return nil
}

func validateSite(site string) error {
	if site != "near" && site != "far" {
		return errors.New("camera site must be near or far")
	}
	return nil
}

func oneOf(v string, values ...string) bool {
	for _, candidate := range values {
		if v == candidate {
			return true
		}
	}
	return false
}

func validateDialString(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("destination is required")
	}
	if len(v) > 256 {
		return errors.New("destination is too long")
	}
	if strings.ContainsAny(v, "\r\n\x00\"") {
		return errors.New("destination contains unsupported characters")
	}
	return nil
}

func safeToken(v string) string {
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "polycom-manager"
	}
	return b.String()
}

func extractQuotedOrTail(lines []string, prefix string) string {
	for _, line := range lines {
		if !strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
			continue
		}
		tail := strings.TrimSpace(line[len(prefix):])
		return strings.Trim(tail, ` "`)
	}
	return ""
}

type SystemInfo struct{ Model, SystemName, Firmware, Serial string }

type ContentStatus struct {
	State  string
	Source int
}

type CallInfo struct {
	CallID           string `json:"call_id"`
	FarSiteName      string `json:"far_site_name,omitempty"`
	FarSiteNumber    string `json:"far_site_number,omitempty"`
	Speed            string `json:"speed,omitempty"`
	ConnectionStatus string `json:"connection_status,omitempty"`
	MuteStatus       string `json:"mute_status,omitempty"`
	Direction        string `json:"direction,omitempty"`
	Type             string `json:"type,omitempty"`
}

func ParseCallInfo(lines []string) []CallInfo {
	var out []CallInfo
	for _, line := range lines {
		if !strings.HasPrefix(strings.ToLower(line), "callinfo:") {
			continue
		}
		parts := strings.Split(line[len("callinfo:"):], ":")
		if len(parts) < 7 {
			continue
		}
		n := len(parts)
		ci := CallInfo{
			CallID:           parts[0],
			Speed:            parts[n-5],
			ConnectionStatus: parts[n-4],
			MuteStatus:       parts[n-3],
			Direction:        parts[n-2],
			Type:             parts[n-1],
		}
		identity := parts[1 : n-5]
		if len(identity) > 0 {
			numberStart := len(identity) - 1
			if len(identity) >= 2 {
				prefix := strings.ToLower(identity[len(identity)-2])
				if prefix == "sip" || prefix == "sips" || prefix == "h323" {
					numberStart = len(identity) - 2
				}
			}
			ci.FarSiteNumber = strings.Join(identity[numberStart:], ":")
			if numberStart > 0 {
				ci.FarSiteName = strings.Join(identity[:numberStart], ":")
			}
		}
		out = append(out, ci)
	}
	return out
}
