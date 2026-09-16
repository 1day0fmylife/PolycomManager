package polycom

import "testing"

func TestParseCallInfoWithNameAndNumber(t *testing.T) {
	got := ParseCallInfo([]string{"callinfo:43:Meeting Room:192.168.1.101:384:connected:notmuted:outgoing:videocall"})
	if len(got) != 1 {
		t.Fatalf("expected one call, got %d", len(got))
	}
	if got[0].CallID != "43" || got[0].FarSiteName != "Meeting Room" || got[0].FarSiteNumber != "192.168.1.101" || got[0].ConnectionStatus != "connected" {
		t.Fatalf("unexpected parse: %+v", got[0])
	}
}

func TestParseCallInfoWithoutName(t *testing.T) {
	got := ParseCallInfo([]string{"callinfo:36:192.168.1.102:256:connected:muted:outgoing:videocall"})
	if len(got) != 1 {
		t.Fatalf("expected one call, got %d", len(got))
	}
	if got[0].CallID != "36" || got[0].FarSiteNumber != "192.168.1.102" || got[0].Speed != "256" || got[0].MuteStatus != "muted" {
		t.Fatalf("unexpected parse: %+v", got[0])
	}
}

func TestParseCallInfoWithSIPURI(t *testing.T) {
	got := ParseCallInfo([]string{"callinfo:51:Alice:sip:alice@example.com:512:connected:notmuted:incoming:videocall"})
	if len(got) != 1 {
		t.Fatalf("expected one call, got %d", len(got))
	}
	if got[0].FarSiteName != "Alice" || got[0].FarSiteNumber != "sip:alice@example.com" || got[0].Speed != "512" {
		t.Fatalf("unexpected parse: %+v", got[0])
	}
}

func TestValidateDialStringRejectsControlCharacters(t *testing.T) {
	if err := validateDialString("sip:alice@example.com\nreboot now"); err == nil {
		t.Fatal("expected dial string validation error")
	}
}

func TestValidateSite(t *testing.T) {
	if err := validateSite("near"); err != nil {
		t.Fatalf("near should be valid: %v", err)
	}
	if err := validateSite("local"); err == nil {
		t.Fatal("expected invalid site error")
	}
}
