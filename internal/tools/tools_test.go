package tools

import (
	"context"
	"errors"
	"os"
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

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "single statement without semicolon",
			query: "SELECT 1",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "single statement with trailing semicolon",
			query: "SELECT 1;",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "two statements",
			query: "CREATE TABLE t (id INT); SELECT * FROM t;",
			want:  []string{"CREATE TABLE t (id INT)", "SELECT * FROM t"},
		},
		{
			name:  "semicolon inside string literal",
			query: "INSERT INTO t (v) VALUES ('a;b'); SELECT 1",
			want:  []string{"INSERT INTO t (v) VALUES ('a;b')", "SELECT 1"},
		},
		{
			name:  "escaped quote inside string literal",
			query: "INSERT INTO t (v) VALUES ('it''s;fine'); SELECT 1;",
			want:  []string{"INSERT INTO t (v) VALUES ('it''s;fine')", "SELECT 1"},
		},
		{
			name:  "semicolon inside double-quoted identifier",
			query: `SELECT "a;b" FROM t; SELECT 2;`,
			want:  []string{`SELECT "a;b" FROM t`, "SELECT 2"},
		},
		{
			name:  "semicolon inside backtick identifier",
			query: "SELECT `a;b` FROM t; SELECT 2;",
			want:  []string{"SELECT `a;b` FROM t", "SELECT 2"},
		},
		{
			name:  "semicolon inside line comment",
			query: "SELECT 1 -- comment;still comment\n; SELECT 2",
			want:  []string{"SELECT 1 -- comment;still comment", "SELECT 2"},
		},
		{
			name:  "semicolon inside block comment",
			query: "SELECT 1 /* a;b */; SELECT 2;",
			want:  []string{"SELECT 1 /* a;b */", "SELECT 2"},
		},
		{
			name:  "empty and whitespace statements dropped",
			query: "; ;SELECT 1;; ;",
			want:  []string{"SELECT 1"},
		},
		{
			name:  "no split on semicolon before literal ends",
			query: "SELECT 'x' ; ",
			want:  []string{"SELECT 'x'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitStatements(tt.query)
			if len(got) != len(tt.want) {
				t.Fatalf("splitStatements(%q) = %#v, want %#v", tt.query, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitStatements(%q)[%d] = %q, want %q", tt.query, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPlaceholderCount(t *testing.T) {
	tests := []struct {
		name string
		stmt string
		want int
	}{
		{name: "no placeholders", stmt: "CREATE TABLE t (id INT)", want: 0},
		{name: "one positional", stmt: "INSERT INTO t (v) VALUES (?)", want: 1},
		{name: "two positional", stmt: "INSERT INTO t (a,b) VALUES (?,?)", want: 2},
		{name: "postgres numbered", stmt: "INSERT INTO t (a,b) VALUES ($1,$2)", want: 2},
		{name: "question mark in literal", stmt: "INSERT INTO t (v) VALUES ('what?')", want: 0},
		{name: "question mark in comment", stmt: "SELECT 1 -- ?\n", want: 0},
		{name: "postgres dollar with no question", stmt: "SELECT $1", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := placeholderCount(tt.stmt); got != tt.want {
				t.Fatalf("placeholderCount(%q) = %d, want %d", tt.stmt, got, tt.want)
			}
		})
	}
}

func TestExecuteMultiStatement(t *testing.T) {
	h := newTestHandler(t)
	id := connectTool(t, h, filepath.Join(t.TempDir(), "multi.db"))

	_, r, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO t (name) VALUES ('alice');",
	})
	if err != nil {
		t.Fatalf("Execute(multi) error = %v", err)
	}
	if len(r.Results) != 2 {
		t.Fatalf("Execute(multi) results = %d, want 2", len(r.Results))
	}
	if r.Results[0].RowsAffected != 0 {
		t.Fatalf("CREATE rows_affected = %d, want 0", r.Results[0].RowsAffected)
	}
	if r.Results[1].RowsAffected != 1 {
		t.Fatalf("INSERT rows_affected = %d, want 1", r.Results[1].RowsAffected)
	}

	_, r, err = h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "SELECT name FROM t WHERE name = 'alice'",
	})
	if err != nil {
		t.Fatalf("Execute(SELECT) error = %v", err)
	}
	if len(r.Results) != 1 || len(r.Results[0].Rows) != 1 {
		t.Fatalf("SELECT results = %+v, want 1 row", r.Results)
	}
}

func TestExecuteNoStatements(t *testing.T) {
	h := newTestHandler(t)
	id := connectTool(t, h, filepath.Join(t.TempDir(), "empty.db"))

	_, _, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        " ; ; ",
	})
	if err == nil {
		t.Fatal("Execute(empty) expected error")
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
		{name: "output path without format", args: ExecuteArgs{ConnectionID: "x", Query: "SELECT 1", OutputPath: "/tmp/out.csv"}},
		{name: "output format without path", args: ExecuteArgs{ConnectionID: "x", Query: "SELECT 1", OutputFormat: "csv"}},
		{name: "unsupported output format", args: ExecuteArgs{ConnectionID: "x", Query: "SELECT 1", OutputFormat: "xml", OutputPath: "/tmp/out.xml"}},
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

func TestExecuteOutputCSVAndJSON(t *testing.T) {
	tests := []struct {
		name   string
		format string
		file   string
		want   string
	}{
		{
			name:   "csv",
			format: "csv",
			file:   "users.csv",
			want:   "id,name\n1,alice\n",
		},
		{
			name:   "json",
			format: "json",
			file:   "users.json",
			want:   `[{"id":1,"name":"alice"}]`,
		},
		{
			name:   "format is case insensitive",
			format: "CSV",
			file:   "users.csv",
			want:   "id,name\n1,alice\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t)
			id := connectTool(t, h, filepath.Join(t.TempDir(), "out.db"))

			if _, _, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
				ConnectionID: id,
				Query:        "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users (name) VALUES ('alice');",
			}); err != nil {
				t.Fatalf("Execute(setup) error = %v", err)
			}

			outPath := filepath.Join(t.TempDir(), "export", tt.file)
			_, r, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
				ConnectionID: id,
				Query:        "SELECT id, name FROM users",
				OutputFormat: tt.format,
				OutputPath:   outPath,
			})
			if err != nil {
				t.Fatalf("Execute(output) error = %v", err)
			}
			if len(r.SavedFiles) != 1 || r.SavedFiles[0] != outPath {
				t.Fatalf("SavedFiles = %#v, want [%s]", r.SavedFiles, outPath)
			}
			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", outPath, err)
			}
			if got := string(data); got != tt.want {
				t.Fatalf("file content = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecuteOutputMultipleStatements(t *testing.T) {
	h := newTestHandler(t)
	id := connectTool(t, h, filepath.Join(t.TempDir(), "multi-out.db"))

	if _, _, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "CREATE TABLE a (v INTEGER); INSERT INTO a VALUES (1);",
	}); err != nil {
		t.Fatalf("Execute(setup) error = %v", err)
	}

	dir := t.TempDir()
	_, r, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "SELECT v FROM a; SELECT 42 AS answer;",
		OutputFormat: "csv",
		OutputPath:   filepath.Join(dir, "res.csv"),
	})
	if err != nil {
		t.Fatalf("Execute(output) error = %v", err)
	}
	want := []string{filepath.Join(dir, "res_1.csv"), filepath.Join(dir, "res_2.csv")}
	if len(r.SavedFiles) != 2 || r.SavedFiles[0] != want[0] || r.SavedFiles[1] != want[1] {
		t.Fatalf("SavedFiles = %#v, want %#v", r.SavedFiles, want)
	}
}

func TestExecuteOutputCountOnlyResult(t *testing.T) {
	h := newTestHandler(t)
	id := connectTool(t, h, filepath.Join(t.TempDir(), "count-out.db"))

	_, _, err := h.Execute(context.Background(), &mcp.CallToolRequest{}, ExecuteArgs{
		ConnectionID: id,
		Query:        "CREATE TABLE t (v INTEGER)",
		OutputFormat: "json",
		OutputPath:   filepath.Join(t.TempDir(), "t.json"),
	})
	if err == nil {
		t.Fatal("Execute(output) expected error when no result sets carry rows")
	}
}
