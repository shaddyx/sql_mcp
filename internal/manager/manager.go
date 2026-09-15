package manager

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"sql-tool/internal/driver"
)

// Sentinel errors surfaced to the MCP client.
var (
	ErrConnectionNotFound = errors.New("connection not found: call the connect method to establish a new connection")
	ErrConnectionClosed   = errors.New("connection has been closed due to inactivity: call the connect method to re-establish the connection")
)

// Manager owns the registry of live connections and enforces the idle
// auto-close policy.
type Manager struct {
	mu          sync.Mutex
	conns       map[string]*conn
	idleTimeout time.Duration // <= 0 disables auto-close
}

type conn struct {
	id          string
	db          *sql.DB
	idleTimeout time.Duration
	mu          sync.Mutex
	busy        int
	lastAct     time.Time
	timer       *time.Timer
	closed      bool
}

// NewManager returns a Manager with the given idle timeout. A timeout <= 0
// disables the idle auto-close feature.
func NewManager(idleTimeout time.Duration) *Manager {
	return &Manager{
		conns:       map[string]*conn{},
		idleTimeout: idleTimeout,
	}
}

// Connect opens a database connection and registers it, returning its id.
func (m *Manager) Connect(ctx context.Context, rawURL string) (string, error) {
	db, err := driver.Open(rawURL)
	if err != nil {
		return "", err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return "", fmt.Errorf("connecting to database: %w", err)
	}

	id := newID()
	c := &conn{
		id:          id,
		db:          db,
		idleTimeout: m.idleTimeout,
		lastAct:     time.Now(),
	}

	m.mu.Lock()
	m.conns[id] = c
	m.mu.Unlock()

	if m.idleTimeout > 0 {
		t := time.AfterFunc(m.idleTimeout, func() { m.checkIdle(c) })
		c.mu.Lock()
		c.timer = t
		c.mu.Unlock()
	}

	return id, nil
}

// Disconnect closes and deregisters an existing connection.
func (m *Manager) Disconnect(id string) error {
	m.mu.Lock()
	c, ok := m.conns[id]
	if ok {
		delete(m.conns, id)
	}
	m.mu.Unlock()

	if !ok {
		return ErrConnectionNotFound
	}
	c.close()
	return nil
}

// CloseAll closes and deregisters every live connection.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	conns := make([]*conn, 0, len(m.conns))
	for _, c := range m.conns {
		conns = append(conns, c)
	}
	m.conns = map[string]*conn{}
	m.mu.Unlock()

	for _, c := range conns {
		c.close()
	}
}

// ActiveConn returns a single conn handle for a connection. It must be
// released with Done after use.
func (m *Manager) ActiveConn(id string) (*ActiveConn, error) {
	m.mu.Lock()
	c, ok := m.conns[id]
	m.mu.Unlock()
	if !ok {
		return nil, ErrConnectionNotFound
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrConnectionClosed
	}
	c.lastAct = time.Now()
	c.busy++
	c.resetTimer()

	return &ActiveConn{c: c}, nil
}

// checkIdle runs when a connection's idle timer fires. If the connection is
// still idle (no in-flight queries and no recent activity), it is marked
// closed so subsequent calls report ErrConnectionClosed.
func (m *Manager) checkIdle(c *conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.busy > 0 || time.Since(c.lastAct) < c.idleTimeout {
		return
	}
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
	}
	c.db.Close()
}

func (c *conn) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
	c.db.Close()
}

func (c *conn) resetTimer() {
	if c.timer != nil && c.idleTimeout > 0 {
		c.timer.Reset(c.idleTimeout)
	}
}

// ActiveConn is a checked-out connection. DB returns the underlying handle for
// running queries; Done must be called once when finished to release it.
type ActiveConn struct {
	c    *conn
	once sync.Once
}

// DB returns the underlying database handle for executing queries.
func (a *ActiveConn) DB() *sql.DB { return a.c.db }

// Done releases the connection, resetting the idle timer.
func (a *ActiveConn) Done() {
	a.once.Do(func() {
		a.c.mu.Lock()
		a.c.busy--
		a.c.lastAct = time.Now()
		a.c.resetTimer()
		a.c.mu.Unlock()
	})
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("generating connection id: %v", err))
	}
	return hex.EncodeToString(b)
}
