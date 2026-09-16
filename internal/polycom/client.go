package polycom

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"polycom-manager/internal/sshclient"
)

type Client struct{ ssh *sshclient.Session }

func New(s *sshclient.Session) *Client { return &Client{ssh: s} }

func (c *Client) Raw(ctx context.Context, command string) ([]string, error) {
	return c.ssh.Execute(ctx, command)
}

func (c *Client) Init(ctx context.Context, sessionName string) error {
	commands := []string{
		fmt.Sprintf("session name %s", safeToken(sessionName)),
		"notify callstatus",
		"notify mutestatus",
		"notify sysstatus",
	}
	for _, cmd := range commands {
		if _, err := c.Raw(ctx, cmd); err != nil {
			return fmt.Errorf("%s: %w", cmd, err)
		}
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

func (c *Client) Dial(ctx context.Context, destination string, speed int) error {
	if speed <= 0 {
		speed = 512
	}
	if err := validateDialString(destination); err != nil {
		return err
	}
	_, err := c.Raw(ctx, fmt.Sprintf(`dial manual %d "%s" sip`, speed, destination))
	return err
}

func (c *Client) Hangup(ctx context.Context) error { _, err := c.Raw(ctx, "hangup all"); return err }
func (c *Client) SetMute(ctx context.Context, muted bool) error {
	v := "off"
	if muted {
		v = "on"
	}
	_, err := c.Raw(ctx, "mute near "+v)
	return err
}
func (c *Client) SetVolume(ctx context.Context, volume int) error {
	if volume < 0 || volume > 50 {
		return errors.New("volume must be between 0 and 50")
	}
	_, err := c.Raw(ctx, fmt.Sprintf("volume set %d", volume))
	return err
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
type CallInfo struct{ CallID, FarSiteName, FarSiteNumber, Speed, ConnectionStatus, MuteStatus, Direction, Type string }

func ParseCallInfo(lines []string) []CallInfo {
	var out []CallInfo
	for _, line := range lines {
		if !strings.HasPrefix(strings.ToLower(line), "callinfo:") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 9 {
			continue
		}
		out = append(out, CallInfo{CallID: parts[1], FarSiteName: parts[2], FarSiteNumber: parts[3], Speed: parts[4], ConnectionStatus: parts[5], MuteStatus: parts[6], Direction: parts[7], Type: strings.Join(parts[8:], ":")})
	}
	return out
}
