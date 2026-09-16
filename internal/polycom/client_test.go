package polycom

import "testing"

func TestParseCallInfo(t *testing.T) {
	got := ParseCallInfo([]string{"callinfo:43:Meeting Room:192.168.1.101:384:connected:notmuted:outgoing:videocall"})
	if len(got) != 1 {
		t.Fatalf("expected one call, got %d", len(got))
	}
	if got[0].CallID != "43" || got[0].FarSiteName != "Meeting Room" || got[0].ConnectionStatus != "connected" {
		t.Fatalf("unexpected parse: %+v", got[0])
	}
}
