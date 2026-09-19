# SQL Tool

An MCP (Model Context Protocol) tool for interacting with SQL databases: establishing connections, executing queries, and managing resources through an agent-friendly interface.

## Features

- **connect** — establish a connection to a SQL database, returning a connection id
- **execute** — run SQL queries on a connected database and return the results
- **disconnect** — close an established connection and free up resources
- **Automatic idle connection cleanup** — idle connections are closed after a configurable period to prevent resource leaks

## Supported databases

- PostgreSQL (`postgres://` / `postgresql://`)
- MySQL (`mysql://`)
- SQLite (`sqlite://file/path.db` or a plain `file/path.db`)

## Requirements

- Go 1.26+
- For integration tests: Docker + Docker Compose

## Installation

First install [gorun](https://github.com/shaddyx/gorun), a tool that clones, builds, runs, and caches a Go application straight from a git URL:

```bash
curl -fsSL https://raw.githubusercontent.com/shaddyx/gorun/master/install.sh | bash
```

Then run the SQL tool from its repository (the `--main` flag points at the binary, which lives in `cmd/sql-tool`):

```bash
gorun --main cmd/sql-tool https://github.com/shaddyx/sql_mcp
```

The first run clones, builds, and caches the server binary in `~/.cache/gorun/`; subsequent runs execute the cached binary instantly. Pin a version with an `@ref` suffix, e.g. `https://github.com/shaddyx/sql_mcp@v1.0.2`, and force a re-fetch/rebuild with `--upgrade`.

## Usage

Run the server over stdio (default):

```bash
go run ./cmd/sql-tool
```

Run over streamable HTTP:

```bash
go run ./cmd/sql-tool -http ":8080"
```

### Configuration

| Environment variable            | Description                                              | Default |
| ------------------------------ | -------------------------------------------------------- | ------- |
| `SQL_MCP_TOOL_IDLE_TIMEOUT`    | Idle connection timeout in seconds. `-1` disables auto-close. | `300` (5 minutes) |
| `ALLOWED_DIRS`                 | Comma-separated glob patterns where `execute` may write output files (see `output_path`). `**` spans path segments, `*` and `?` stay within one segment, and `{cwd}` expands to the working directory. A pattern without glob characters names a directory and allows everything beneath it. An explicitly empty value forbids all exports. | `{cwd}/**` |

Example `ALLOWED_DIRS=/tmp/**,{cwd}/**` allows creating output files anywhere under `/tmp` or under the current working directory.

The inactivity timer is reset each time a query starts or finishes. If a connection is closed due to inactivity, the `execute` method returns an error instructing the agent to call `connect` again to re-establish the connection.

## MCP methods

### connect

Establish a connection to a database and return a connection id.

- `connection_url` (string, required): the database location and authentication details.

```go
connect("postgresql://user:password@localhost:5432/mydatabase")
```

### execute

Run a SQL query on a connected database and return the results (columns, rows, rows affected).

- `connection_id` (string, required): the id of the established connection.
- `query` (string, required): the SQL query to execute. Multiple statements can be batched with semicolons; each runs one at a time and contributes a result set.
- `params` (array, optional): parameters to bind to the query.
- `timeout` (integer, optional): maximum time in seconds to wait for the query. Default `30`.
- `output_format` (string, optional): store the result sets on disk as `csv` or `json`. Requires `output_path`.
- `output_path` (string, optional): file path to write the result sets to; must be within `ALLOWED_DIRS`. When the query produces several result sets, each is written to a numbered file (`out.csv` becomes `out_1.csv`, `out_2.csv`, ...). JSON files contain an array of objects keyed by column name.

```go
execute("connection_id_123", "SELECT * FROM users", null, 10)
execute("connection_id_123", "SELECT * FROM users", null, 10, "csv", "/tmp/users.csv")
```

### disconnect

Close an existing connection, freeing up resources.

- `connection_id` (string, required): the id of the connection to close.

```go
disconnect("connection_id_123")
```

## Testing

Run the unit tests:

```bash
go test ./...
```

Run the full suite against PostgreSQL, MySQL and SQLite in Docker:

```bash
./scripts/test-docker.sh
```

The script brings up `postgres:16` and `mysql:8` containers (published on host ports `5433`/`3307`), waits for them to become healthy, then runs the integration tests (build tag `integration`), which drive the real server over stdio. Containers are torn down afterwards; pass `--keep` to leave them running.

Useful flags:
- `--keep` — leave the containers running after the test
- `--no-up` — assume the databases are already up and healthy
