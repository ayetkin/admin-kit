package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// Session is a signed-in operator, keyed by an opaque cookie token.
type Session struct {
	Token     string
	Email     string
	Name      string
	Picture   string
	CreatedAt int64 // unix milliseconds
	ExpiresAt int64 // unix milliseconds
}

// Expired reports whether the session is past its lifetime.
func (s Session) Expired() bool { return s.ExpiresAt <= time.Now().UnixMilli() }

// Store persists sessions. Implement it over whatever a panel already has (the
// project's SQLite database, say); NewMemoryStore is enough to get started, and
// enough for a single-replica panel that can afford to sign operators out on
// restart.
//
// Get returns (nil, nil) for a token that is unknown or expired.
type Store interface {
	Create(Session) error
	Get(token string) (*Session, error)
	Delete(token string) error
	// DeleteExpired purges every session past its expiry, returning how many
	// were removed. Auth calls it periodically.
	DeleteExpired() (int64, error)
}

// MemoryStore keeps sessions in memory. Signing in survives a page load but not
// a restart, and it is per-process, so a panel behind more than one replica
// needs a shared store instead.
type MemoryStore struct {
	mu       sync.Mutex
	sessions map[string]Session
}

// NewMemoryStore returns an empty in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: map[string]Session{}}
}

func (m *MemoryStore) Create(s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.Token] = s
	return nil
}

func (m *MemoryStore) Get(token string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return nil, nil
	}
	if s.Expired() {
		delete(m.sessions, token)
		return nil, nil
	}
	return &s, nil
}

func (m *MemoryStore) Delete(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
	return nil
}

func (m *MemoryStore) DeleteExpired() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for token, s := range m.sessions {
		if s.Expired() {
			delete(m.sessions, token)
			n++
		}
	}
	return n, nil
}

// randToken returns a 256-bit URL-safe random token, used for both session
// cookies and the OAuth state parameter.
func randToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}
