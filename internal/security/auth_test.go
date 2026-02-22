package security

import "testing"

func TestCredentialAllows(t *testing.T) {
	cred := Credential{
		ID:      "ops",
		Devices: []string{"clock-1", "clock-*", " * "},
	}

	if !cred.Allows("clock-1") {
		t.Fatal("expected exact scope to match")
	}
	if !cred.Allows("clock-42") {
		t.Fatal("expected prefix scope to match")
	}
	if !cred.Allows("anything") {
		t.Fatal("expected wildcard scope to match")
	}

	locked := Credential{ID: "limited", Devices: []string{"clock-allowed"}}
	if locked.Allows("clock-denied") {
		t.Fatal("expected denied device to fail scope check")
	}
}

func TestParseCredentials(t *testing.T) {
	creds, err := ParseCredentials(" ops | s3cr3t | clock-1,clock-* ; viewer|token2|* ")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(creds) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(creds))
	}
	if creds[0].ID != "ops" || creds[0].Token != "s3cr3t" {
		t.Fatalf("unexpected first credential: %#v", creds[0])
	}
	if len(creds[0].Devices) != 2 || creds[0].Devices[0] != "clock-1" || creds[0].Devices[1] != "clock-*" {
		t.Fatalf("unexpected scopes: %#v", creds[0].Devices)
	}
	if creds[1].ID != "viewer" || len(creds[1].Devices) != 1 || creds[1].Devices[0] != "*" {
		t.Fatalf("unexpected second credential: %#v", creds[1])
	}
}

func TestParseCredentialsRejectsInvalidEntries(t *testing.T) {
	cases := []string{
		"ops|secret",
		"ops||clock-1",
		"ops|secret|, ,",
	}
	for _, raw := range cases {
		if _, err := ParseCredentials(raw); err == nil {
			t.Fatalf("expected parse error for %q", raw)
		}
	}
}
