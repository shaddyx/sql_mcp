package manager

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func connectSQLite(t *testing.T, m *Manager) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manager.db")
	id, err := m.Connect(context.Background(), "sqlite://"+path)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	return id
}

func TestConnectReturnsUniqueID(t *testing.T) {
	m := NewManager(0)

	path := filepath.Join(t.TempDir(), "a.db")
	id1, err := m.Connect(context.Background(), "sqlite://"+path)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	path2 := filepath.Join(t.TempDir(), "b.db")
	id2, err := m.Connect(context.Background(), "sqlite://"+path2)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if id1 == id2 {
		t.Fatalf("ids should be unique: %q == %q", id1, id2)
	}
	defer m.CloseAll()
}

func TestConnectManyYieldsUniqueIDs(t *testing.T) {
	m := NewManager(0)
	defer m.CloseAll()

	path := filepath.Join(t.TempDir(), "many.db")
	const n = 200
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id, err := m.Connect(context.Background(), "sqlite://"+path)
		if err != nil {
			t.Fatalf("Connect() #%d error = %v", i, err)
		}
		ids = append(ids, id)
	}
	seen := make(map[string]bool, n)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate connection id generated: %q", id)
		}
		seen[id] = true
	}
}

func TestConnectInvalidURL(t *testing.T) {
	m := NewManager(0)
	defer m.CloseAll()

	if _, err := m.Connect(context.Background(), "oracle://x@y/z"); err == nil {
		t.Fatal("Connect() expected error for unsupported scheme")
	}
}

func TestDisconnectKnownAndUnknown(t *testing.T) {
	m := NewManager(0)
	defer m.CloseAll()

	id := connectSQLite(t, m)

	if err := m.Disconnect(id); err != nil {
		t.Fatalf("Disconnect(%q) error = %v", id, err)
	}

	if err := m.Disconnect(id); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("Disconnect(id) after close = %v, want ErrConnectionNotFound", err)
	}

	if err := m.Disconnect("nope"); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("Disconnect(unknown) = %v, want ErrConnectionNotFound", err)
	}
}

func TestExecuteAfterDisconnect(t *testing.T) {
	m := NewManager(0)
	defer m.CloseAll()

	id := connectSQLite(t, m)
	if err := m.Disconnect(id); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}

	ac, err := m.ActiveConn(id)
	if err == nil {
		ac.Done()
		t.Fatal("ActiveConn() on closed connection = nil error")
	}
	if !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("ActiveConn() error = %v, want ErrConnectionNotFound", err)
	}
}

func TestIdleAutoClose(t *testing.T) {
	m := NewManager(200 * time.Millisecond)
	defer m.CloseAll()

	id := connectSQLite(t, m)

	ac, err := m.ActiveConn(id)
	if err != nil {
		t.Fatalf("ActiveConn() error = %v", err)
	}
	ac.Done()

	time.Sleep(500 * time.Millisecond)

	ac, err = m.ActiveConn(id)
	if err == nil {
		ac.Done()
		t.Fatal("ActiveConn() after idle timeout = nil error")
	}
	if !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("ActiveConn() after idle timeout = %v, want ErrConnectionClosed", err)
	}
}

func TestIdleTimerResetOnActivity(t *testing.T) {
	m := NewManager(300 * time.Millisecond)
	defer m.CloseAll()

	id := connectSQLite(t, m)

	ac, err := m.ActiveConn(id)
	if err != nil {
		t.Fatalf("ActiveConn() error = %v", err)
	}
	ac.Done()

	// Keep the connection active well past the original idle timeout.
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		ac, err := m.ActiveConn(id)
		if err != nil {
			t.Fatalf("ActiveConn() iteration %d error = %v", i, err)
		}
		ac.Done()
	}

	// The connection must still be alive.
	if _, err := m.ActiveConn(id); err != nil {
		t.Fatalf("ActiveConn() after sustained activity = %v, want nil", err)
	}
}

func TestIdleDisabled(t *testing.T) {
	m := NewManager(-1)
	defer m.CloseAll()

	id := connectSQLite(t, m)

	ac, err := m.ActiveConn(id)
	if err != nil {
		t.Fatalf("ActiveConn() error = %v", err)
	}
	ac.Done()

	time.Sleep(200 * time.Millisecond)

	if _, err := m.ActiveConn(id); err != nil {
		t.Fatalf("ActiveConn() with idle disabled = %v, want nil", err)
	}
}

func TestConcurrentActivity(t *testing.T) {
	m := NewManager(100 * time.Millisecond)
	defer m.CloseAll()

	id := connectSQLite(t, m)

	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 20; j++ {
				ac, err := m.ActiveConn(id)
				if err != nil {
					return
				}
				var one int
				if err := ac.DB().QueryRow("SELECT 1").Scan(&one); err != nil {
					ac.Done()
					return
				}
				ac.Done()
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		<-done
	}
}
