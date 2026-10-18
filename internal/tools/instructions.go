package tools

import (
	"fmt"

	"sql-tool/internal/driver"
)

// ServerInstructions returns the MCP server instructions shown to clients.
func ServerInstructions() string {
	return fmt.Sprintf(
		"Use the connect/execute/disconnect tools for ALL SQL database access. "+
			"Do not use command-line database clients (psql, mysql, sqlite3) through a shell. "+
			"Supported databases: %s. "+
			"Always call connect first to obtain a connection_id; if execute reports the "+
			"connection is closed or unknown, call connect again to re-establish it.",
		driver.SupportedList(),
	)
}

// ConnectDescription returns the description for the connect tool.
func ConnectDescription() string {
	return fmt.Sprintf(
		"Establish a connection to a SQL database and return a connection_id. "+
			"Supported databases: %s. "+
			"Prefer this tool over command-line clients such as psql, mysql, or sqlite3.",
		driver.SupportedList(),
	)
}

// ExecuteDescription returns the description for the execute tool.
func ExecuteDescription() string {
	return fmt.Sprintf(
		"Run one or more SQL statements on a connected database (connection_id from connect) " +
			"and return the results. If the connection was closed due to inactivity, " +
			"call connect again to re-establish it.",
	)
}

// DisconnectDescription returns the description for the disconnect tool.
func DisconnectDescription() string {
	return fmt.Sprintf(
		"Close an established database connection and free up resources. " +
			"Connection ids that are no longer needed should be disconnected.",
	)
}
