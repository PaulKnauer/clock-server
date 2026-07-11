package security

import (
	"sync"
	"testing"
)

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

// TestCredentialStoreReload verifies that after Replace(), the old credential
// is rejected and the new credential is accepted.
func TestCredentialStoreReload(t *testing.T) {
	credA := Credential{ID: "a", Token: "token-a", Devices: []string{"*"}}
	credB := Credential{ID: "b", Token: "token-b", Devices: []string{"*"}}

	store := NewCredentialStore([]Credential{credA})

	// credA is present, credB is not.
	loaded := store.Load()
	if len(loaded) != 1 || loaded[0].Token != "token-a" {
		t.Fatalf("expected credential A before reload, got %v", loaded)
	}

	// Replace with credB only.
	store.Replace([]Credential{credB})

	loaded = store.Load()
	if len(loaded) != 1 || loaded[0].Token != "token-b" {
		t.Fatalf("expected credential B after reload, got %v", loaded)
	}
	// Old token must not appear in the new set.
	for _, c := range loaded {
		if c.Token == "token-a" {
			t.Fatal("old credential A still present after reload")
		}
	}
}

// TestCredentialStoreFailSafeOnBadReload verifies that when a Replace() is NOT
// called (because parsing failed), the original credentials remain valid.
// This simulates the fail-safe behaviour in the SIGHUP handler.
func TestCredentialStoreFailSafeOnBadReload(t *testing.T) {
	credA := Credential{ID: "a", Token: "token-a", Devices: []string{"*"}}
	store := NewCredentialStore([]Credential{credA})

	// Attempt to parse malformed credentials — this would fail before Replace().
	_, err := ParseCredentials("bad-format-no-pipe")
	if err == nil {
		t.Fatal("expected parse error for malformed credentials")
	}
	// Because parse failed, we do NOT call store.Replace(). The original
	// credentials must still be present.
	loaded := store.Load()
	if len(loaded) != 1 || loaded[0].Token != "token-a" {
		t.Fatalf("expected original credential A to remain, got %v", loaded)
	}
}

// TestCredentialStoreConcurrentReplace exercises concurrent reads during a
// Replace() to verify there is no race condition. Run with -race.
func TestCredentialStoreConcurrentReplace(t *testing.T) {
	initial := Credential{ID: "init", Token: "token-init", Devices: []string{"*"}}
	store := NewCredentialStore([]Credential{initial})

	var wg sync.WaitGroup
	const readers = 50
	const replacements = 20

	// Spawn readers that continuously load from the store.
	stop := make(chan struct{})
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					creds := store.Load()
					if len(creds) == 0 {
						t.Errorf("Load() returned empty slice during concurrent Replace()")
					}
				}
			}
		}()
	}

	// Perform several Replace() calls while readers are running.
	for i := 0; i < replacements; i++ {
		newCred := Credential{
			ID:      "rotated",
			Token:   "token-rotated",
			Devices: []string{"*"},
		}
		store.Replace([]Credential{newCred})
	}

	close(stop)
	wg.Wait()
}
