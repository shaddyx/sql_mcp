package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sql-tool/internal/manager"
	"sql-tool/internal/tools"
)

var httpAddr = flag.String("http", "", "if set, use streamable HTTP at this address, instead of stdin/stdout")

const idleTimeoutEnv = "SQL_MCP_TOOL_IDLE_TIMEOUT"

func main() {
	flag.Parse()

	idleTimeout := idleTimeoutFromEnv()

	m := manager.NewManager(idleTimeout)
	h := tools.NewHandler(m)

	server := mcp.NewServer(&mcp.Implementation{Name: "sql-tool"}, &mcp.ServerOptions{
		Instructions: tools.ServerInstructions(),
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "connect",
		Description: tools.ConnectDescription(),
	}, h.Connect)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "execute",
		Description: tools.ExecuteDescription(),
	}, h.Execute)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "disconnect",
		Description: tools.DisconnectDescription(),
	}, h.Disconnect)

	if *httpAddr != "" {
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)
		log.Printf("MCP handler listening at %s", *httpAddr)
		log.Fatal(http.ListenAndServe(*httpAddr, handler))
	}

	defer m.CloseAll()
	t := &mcp.LoggingTransport{Transport: &mcp.StdioTransport{}, Writer: os.Stderr}
	if err := server.Run(context.Background(), t); err != nil {
		log.Printf("Server failed: %v", err)
	}
}

func idleTimeoutFromEnv() time.Duration {
	raw, ok := os.LookupEnv(idleTimeoutEnv)
	if !ok {
		return 5 * time.Minute
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("invalid %s value %q, defaulting to 300 seconds", idleTimeoutEnv, raw)
		return 5 * time.Minute
	}

	if seconds == -1 {
		return -1 * time.Second
	}
	if seconds < 0 {
		log.Printf("invalid %s value %d, defaulting to 300 seconds", idleTimeoutEnv, seconds)
		return 5 * time.Minute
	}

	return time.Duration(seconds) * time.Second
}
