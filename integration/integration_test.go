//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// buildServer compiles the sql-tool server and returns its path.
func buildServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(os.TempDir(), "sql-tool-server-test")
	cmd := exec.Command("go", "build", "-o", bin, "sql-tool/cmd/sql-tool")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate repo root")
	return ""
}

func startServer(t *testing.T, bin string, idleTimeout string) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "SQL_MCP_TOOL_IDLE_TIMEOUT="+idleTimeout)

	client := mcp.NewClient(&mcp.Implementation{Name: "integration-test"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

func callToolErr(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

// structuredContent decodes the structured result of a tool call.
func structuredContent[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	if res.IsError {
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text = tc.Text
			}
		}
		t.Fatalf("tool call errored: %s", text)
	}
	var out T
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshaling StructuredContent: %v", err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decoding StructuredContent: %v", err)
	}
	return out
}

func lastErrorMessage(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func TestConnectExecuteDisconnectPostgres(t *testing.T) {
	if os.Getenv("SQL_TOOL_TEST_POSTGRES_URL") == "" {
		t.Skip("SQL_TOOL_TEST_POSTGRES_URL not set")
	}
	bin := buildServer(t)
	cs := startServer(t, bin, "600")

	res := callTool(t, cs, "connect", map[string]any{
		"connection_url": os.Getenv("SQL_TOOL_TEST_POSTGRES_URL"),
	})
	out := structuredContent[struct {
		ConnectionID string `json:"connection_id"`
	}](t, res)
	if out.ConnectionID == "" {
		t.Fatal("empty connection id")
	}

	if err := tableFlow(t, cs, out.ConnectionID, "$1", "id SERIAL PRIMARY KEY"); err != nil {
		t.Fatalf("table flow: %v", err)
	}

	callTool(t, cs, "disconnect", map[string]any{"connection_id": out.ConnectionID})
}

func TestConnectExecuteDisconnectMySQL(t *testing.T) {
	if os.Getenv("SQL_TOOL_TEST_MYSQL_URL") == "" {
		t.Skip("SQL_TOOL_TEST_MYSQL_URL not set")
	}
	bin := buildServer(t)
	cs := startServer(t, bin, "600")

	res := callTool(t, cs, "connect", map[string]any{
		"connection_url": os.Getenv("SQL_TOOL_TEST_MYSQL_URL"),
	})
	out := structuredContent[struct {
		ConnectionID string `json:"connection_id"`
	}](t, res)
	if out.ConnectionID == "" {
		t.Fatal("empty connection id")
	}

	if err := tableFlow(t, cs, out.ConnectionID, "?", "id INTEGER PRIMARY KEY AUTO_INCREMENT"); err != nil {
		t.Fatalf("table flow: %v", err)
	}

	callTool(t, cs, "disconnect", map[string]any{"connection_id": out.ConnectionID})
}

func TestConnectExecuteDisconnectSQLite(t *testing.T) {
	bin := buildServer(t)
	cs := startServer(t, bin, "600")

	url := "sqlite://" + filepath.Join(t.TempDir(), "integration.db")
	res := callTool(t, cs, "connect", map[string]any{"connection_url": url})
	out := structuredContent[struct {
		ConnectionID string `json:"connection_id"`
	}](t, res)

	if err := tableFlow(t, cs, out.ConnectionID, "?", "id INTEGER PRIMARY KEY AUTOINCREMENT"); err != nil {
		t.Fatalf("table flow: %v", err)
	}

	callTool(t, cs, "disconnect", map[string]any{"connection_id": out.ConnectionID})
}

// tableFlow exercises CREATE/INSERT/SELECT with params and verifies the result.
// placeholder is the bind-marker style and idCol the primary-key column
// definition for the database dialect.
func tableFlow(t *testing.T, cs *mcp.ClientSession, connID string, placeholder string, idCol string) error {
	t.Helper()

	callTool(t, cs, "execute", map[string]any{
		"connection_id": connID,
		"query":         "DROP TABLE IF EXISTS demo",
	})
	callTool(t, cs, "execute", map[string]any{
		"connection_id": connID,
		"query":         "CREATE TABLE demo (" + idCol + ", name TEXT)",
	})
	res := callTool(t, cs, "execute", map[string]any{
		"connection_id": connID,
		"query":         "INSERT INTO demo (name) VALUES (" + placeholder + ")",
		"params":        []any{"alice"},
	})
	insertRes := structuredContent[struct {
		Results []struct {
			RowsAffected int64 `json:"rows_affected"`
		} `json:"results"`
	}](t, res)
	if len(insertRes.Results) != 1 || insertRes.Results[0].RowsAffected != 1 {
		t.Fatalf("insert rows_affected = %+v, want 1", insertRes.Results)
	}

	res = callTool(t, cs, "execute", map[string]any{
		"connection_id": connID,
		"query":         "SELECT id, name FROM demo ORDER BY id",
	})
	selectRes := structuredContent[struct {
		Results []struct {
			Columns []string `json:"columns"`
			Rows    [][]any  `json:"rows"`
		} `json:"results"`
	}](t, res)
	if len(selectRes.Results) != 1 {
		t.Fatalf("select results = %d, want 1", len(selectRes.Results))
	}
	r := selectRes.Results[0]
	if len(r.Columns) != 2 || len(r.Rows) != 1 {
		t.Fatalf("select shape = cols %v rows %d, want 2 cols 1 row", r.Columns, len(r.Rows))
	}

	return nil
}

func TestIdleAutoClose(t *testing.T) {
	bin := buildServer(t)
	// 1 second idle timeout.
	cs := startServer(t, bin, "1")

	url := "sqlite://" + filepath.Join(t.TempDir(), "idle.db")
	res := callTool(t, cs, "connect", map[string]any{"connection_url": url})
	out := structuredContent[struct {
		ConnectionID string `json:"connection_id"`
	}](t, res)

	time.Sleep(2500 * time.Millisecond)

	res = callToolErr(t, cs, "execute", map[string]any{
		"connection_id": out.ConnectionID,
		"query":         "SELECT 1",
	})
	if !res.IsError {
		t.Fatal("execute after idle expected IsError")
	}
	msg := lastErrorMessage(res)
	if msg == "" {
		t.Fatal("no error text returned")
	}
}

func TestExecuteUnknownConnection(t *testing.T) {
	bin := buildServer(t)
	cs := startServer(t, bin, "600")

	res := callToolErr(t, cs, "execute", map[string]any{
		"connection_id": "does-not-exist",
		"query":         "SELECT 1",
	})
	if !res.IsError {
		t.Fatal("expected error for unknown connection")
	}
}
