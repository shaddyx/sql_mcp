package tools

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sql-tool/internal/manager"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(manager.NewManager(-1))
}

func connectTool(t *testing.T, h *Handler, dbPath string) string {
	t.Helper()
	_, out, err := h.Connect(context.Background(), &mcp.CallToolRequest{}, ConnectArgs{
		ConnectionURL: "sqlite://" + dbPath,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if out.ConnectionID == "" {
		t.Fatal("Connect() returned empty connection id")
	}
	return out.ConnectionID
}

func TestConnect(t *testing.T) {
	h := newTestHandler(t)

	id := connectTool(t, h, filepath.Join(t.TempDir(), "c.db"))
	if id == "" {
		t.Fatal("Connect() returned empty id")
	}
}

func TestConnectEmptyURL(t *testing.T) {
	h := newTestHandler(t)

	_, _, err := h.Connect(context.Background(), &mcp.CallToolRequest{}, ConnectArgs{})
	if err == nil {
		t.Fatal("Connect() expected error for empty connection_url")
	}
}

func TestExecuteCRUD(t *testing.T) {
	h := newTestHandler(t)
	id := connectTool(t, h, filepath.Join(t.TempDir(), "crud.db"))

	_, r, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)",
	})
	if err != nil {
		t.Fatalf("Execute(CREATE) error = %v", err)
	}
	if len(r.Results) != 1 {
		t.Fatalf("Execute(CREATE) results = %d, want 1", len(r.Results))
	}

	if _, r, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "INSERT INTO users (name) VALUES (?)",
		Params:       []any{"alice"},
	}); err != nil {
		t.Fatalf("Execute(INSERT) error = %v", err)
	} else if r.Results[0].RowsAffected != 1 {
		t.Fatalf("INSERT rows_affected = %d, want 1", r.Results[0].RowsAffected)
	}

	_, r, err = h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "SELECT id, name FROM users ORDER BY id",
	})
	if err != nil {
		t.Fatalf("Execute(SELECT) error = %v", err)
	}
	res := r.Results[0]
	if len(res.Columns) != 2 {
		t.Fatalf("SELECT columns = %v, want 2 columns", res.Columns)
	}
	if res.Columns[0] != "id" || res.Columns[1] != "name" {
		t.Fatalf("SELECT columns = %v, want [id name]", res.Columns)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("SELECT rows = %d, want 1", len(res.Rows))
	}
}

func TestExecuteMissingConnection(t *testing.T) {
	h := newTestHandler(t)

	_, _, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: "nope",
		Query:        "SELECT 1",
	})
	if !errors.Is(err, manager.ErrConnectionNotFound) {
		t.Fatalf("Execute(unknown conn) = %v, want ErrConnectionNotFound", err)
	}
}

func TestExecuteIdleClosedConnection(t *testing.T) {
	m := manager.NewManager(150 * time.Millisecond)
	defer m.CloseAll()
	h := NewHandler(m)

	id, err := m.Connect(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "idle.db"))
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	time.Sleep(400 * time.Millisecond)

	_, _, err = h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "SELECT 1",
	})
	if !errors.Is(err, manager.ErrConnectionClosed) {
		t.Fatalf("Execute(after idle) = %v, want ErrConnectionClosed", err)
	}
}

func TestExecuteValidation(t *testing.T) {
	h := newTestHandler(t)

	tests := []struct {
		name string
		args ExecuteArgs
	}{
		{name: "missing connection id", args: ExecuteArgs{Query: "SELECT 1"}},
		{name: "missing query", args: ExecuteArgs{ConnectionID: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, tt.args); err == nil {
				t.Fatal("Execute() expected validation error")
			}
		})
	}
}

func TestDisconnect(t *testing.T) {
	h := newTestHandler(t)
	id := connectTool(t, h, filepath.Join(t.TempDir(), "d.db"))

	if _, _, err := h.Disconnect(context.Background(), &mcp.CallToolRequest{}, DisconnectArgs{
		ConnectionID: id,
	}); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}

	if _, _, err := h.Disconnect(context.Background(), &mcp.CallToolRequest{}, DisconnectArgs{
		ConnectionID: id,
	}); !errors.Is(err, manager.ErrConnectionNotFound) {
		t.Fatalf("Disconnect(twice) = %v, want ErrConnectionNotFound", err)
	}
}

func TestDisconnectMissing(t *testing.T) {
	h := newTestHandler(t)

	if _, _, err := h.Disconnect(context.Background(), &mcp.CallToolRequest{}, DisconnectArgs{
		ConnectionID: "nope",
	}); !errors.Is(err, manager.ErrConnectionNotFound) {
		t.Fatalf("Disconnect(unknown) = %v, want ErrConnectionNotFound", err)
	}
}

func TestExecuteTimeoutOverride(t *testing.T) {
	h := newTestHandler(t)
	id := connectTool(t, h, filepath.Join(t.TempDir(), "t.db"))

	timeout := 1
	_, r, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "SELECT 1",
		Timeout:      &timeout,
	})
	if err != nil {
		t.Fatalf("Execute(Timeout=1) error = %v", err)
	}
	if len(r.Results) != 1 {
		t.Fatalf("Execute(Timeout=1) results = %d, want 1", len(r.Results))
	}
}
