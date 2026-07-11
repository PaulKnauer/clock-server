package security

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Credential represents an API credential identity with scoped device access.
type Credential struct {
	ID      string
	Token   string
	Devices []string
}

// Allows reports whether the credential can operate on the target device.
func (c Credential) Allows(deviceID string) bool {
	deviceID = strings.TrimSpace(deviceID)
	for _, scope := range c.Devices {
		scope = strings.TrimSpace(scope)
		if scope == "*" {
			return true
		}
		if strings.HasSuffix(scope, "*") {
			prefix := strings.TrimSuffix(scope, "*")
			if strings.HasPrefix(deviceID, prefix) {
				return true
			}
			continue
		}
		if deviceID == scope {
			return true
		}
	}
	return false
}

// CredentialStore holds a credential slice that can be atomically replaced.
// All reads are lock-free; Replace performs a single atomic pointer swap
// so there is never a window where zero credentials are valid.
type CredentialStore struct {
	ptr atomic.Pointer[[]Credential]
}

// NewCredentialStore creates a CredentialStore pre-loaded with the given credentials.
func NewCredentialStore(creds []Credential) *CredentialStore {
	cs := &CredentialStore{}
	cs.ptr.Store(&creds)
	return cs
}

// Load returns the current credential slice. Safe for concurrent use.
func (cs *CredentialStore) Load() []Credential {
	p := cs.ptr.Load()
	if p == nil {
		return nil
	}
	return *p
}

// Replace atomically swaps the entire credential set. Safe for concurrent use.
func (cs *CredentialStore) Replace(creds []Credential) {
	cs.ptr.Store(&creds)
}

// ParseCredentials parses semicolon-separated credentials in the format:
// id|token|scope1,scope2;id2|token2|*
func ParseCredentials(raw string) ([]Credential, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	entries := strings.Split(raw, ";")
	out := make([]Credential, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid credential entry %q", entry)
		}
		id := strings.TrimSpace(parts[0])
		token := strings.TrimSpace(parts[1])
		if id == "" || token == "" {
			return nil, fmt.Errorf("credential id and token are required in %q", entry)
		}
		scopeParts := strings.Split(parts[2], ",")
		devices := make([]string, 0, len(scopeParts))
		for _, scope := range scopeParts {
			scope = strings.TrimSpace(scope)
			if scope != "" {
				devices = append(devices, scope)
			}
		}
		if len(devices) == 0 {
			return nil, fmt.Errorf("at least one scope is required in %q", entry)
		}
		out = append(out, Credential{ID: id, Token: token, Devices: devices})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
