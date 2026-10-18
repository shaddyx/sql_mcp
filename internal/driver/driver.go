package driver

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Engine describes a supported database engine and the connection-url forms it accepts.
type Engine struct {
	Name    string
	Schemes []string // human-readable accepted forms, e.g. "postgres://", "postgresql://"
}

// engine pairs an Engine's metadata with the logic that recognizes its url
// forms and opens a *sql.DB for them.
type engine struct {
	Engine
	match func(u *url.URL, raw string) bool
	open  func(u *url.URL, raw string) (*sql.DB, error)
}

// registry is the single source of truth for the databases this server
// supports: metadata and opening logic live in one entry per engine, so they
// cannot drift apart. Matching order matters: exact scheme checks first, then
// the "file:" prefix, then plain filesystem paths.
var registry = []engine{
	{
		Engine: Engine{Name: "PostgreSQL", Schemes: []string{"postgres://", "postgresql://"}},
		match: func(u *url.URL, _ string) bool {
			return u.Scheme == "postgres" || u.Scheme == "postgresql"
		},
		open: openPostgres,
	},
	{
		Engine: Engine{Name: "MySQL", Schemes: []string{"mysql://"}},
		match:  func(u *url.URL, _ string) bool { return u.Scheme == "mysql" },
		open:   openMySQL,
	},
	{
		Engine: Engine{Name: "SQLite", Schemes: []string{"sqlite://", "file:", "plain file path"}},
		match: func(u *url.URL, raw string) bool {
			return u.Scheme == "sqlite" ||
				strings.HasPrefix(raw, "file:") ||
				(!u.IsAbs() && isPlainPath(raw))
		},
		open: openSQLite,
	},
}

// Supported returns a copy of the engine registry.
func Supported() []Engine {
	out := make([]Engine, len(registry))
	for i, e := range registry {
		out[i] = Engine{
			Name:    e.Name,
			Schemes: append([]string(nil), e.Schemes...),
		}
	}
	return out
}

// SupportedList renders the registry as a single human-readable line, e.g.
// "PostgreSQL (postgres:// / postgresql://), MySQL (mysql://), ...".
func SupportedList() string {
	engines := Supported()
	parts := make([]string, 0, len(engines))
	for _, e := range engines {
		parts = append(parts, fmt.Sprintf("%s (%s)", e.Name, strings.Join(e.Schemes, " / ")))
	}
	return strings.Join(parts, ", ")
}

// Open parses a connection url and opens a *sql.DB for the matching engine.
// Supported schemes: postgres/postgresql, mysql, sqlite (a plain file path is
// also accepted and treated as a SQLite database).
func Open(rawURL string) (*sql.DB, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid connection url: %w", err)
	}

	for _, e := range registry {
		if e.match(u, rawURL) {
			return e.open(u, rawURL)
		}
	}
	return nil, fmt.Errorf("unsupported connection url: %q", rawURL)
}

// openPostgres opens a postgres/postgresql connection from the parsed url.
func openPostgres(u *url.URL, _ string) (*sql.DB, error) {
	dsn, err := postgresDSN(u)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}
	return db, nil
}

// openMySQL opens a mysql connection from the parsed url.
func openMySQL(u *url.URL, _ string) (*sql.DB, error) {
	dsn, err := mysqlDSN(u)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening mysql connection: %w", err)
	}
	return db, nil
}

// openSQLite opens a SQLite connection, accepting the sqlite:// scheme, the
// "file:" prefix, and plain filesystem paths.
func openSQLite(u *url.URL, rawURL string) (*sql.DB, error) {
	var path string
	switch {
	case u.Scheme == "sqlite":
		path = u.Opaque
	case strings.HasPrefix(rawURL, "file:"):
		path = strings.TrimPrefix(rawURL, "file:")
	default:
		path = rawURL
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite connection: %w", err)
	}
	return db, nil
}

// isPlainPath reports whether the raw string has no scheme component, i.e. it
// can only be a filesystem path to a SQLite database.
func isPlainPath(raw string) bool {
	return !strings.Contains(raw, "://") && !strings.Contains(raw, ":")
}

// postgresDSN converts a postgres url into a lib/pq DSN. In the common
// local/dev case the server has no SSL, so we default to "sslmode=disable"
// unless the caller explicitly set an sslmode query parameter.
func postgresDSN(u *url.URL) (string, error) {
	q := u.Query()
	if !q.Has("sslmode") {
		q.Set("sslmode", "disable")
	}
	u2 := *u
	u2.RawQuery = q.Encode()
	return u2.String(), nil
}

// mysqlDSN converts a mysql:// url into the go-sql-driver DSN format.
// Example: mysql://user:pass@host:3306/dbname?parseTime=true
// -> user:pass@tcp(host:3306)/dbname?parseTime=true
func mysqlDSN(u *url.URL) (string, error) {
	user := u.User
	host := u.Host
	if host == "" {
		return "", fmt.Errorf("mysql connection url missing host")
	}

	var b strings.Builder
	if user != nil {
		b.WriteString(user.String())
		b.WriteString("@")
	}
	b.WriteString("tcp(")
	b.WriteString(host)
	b.WriteString(")")

	path := strings.TrimPrefix(u.Path, "/")
	if path != "" {
		b.WriteString("/")
		b.WriteString(path)
	}

	q := u.Query()
	q.Set("parseTime", "true")
	if _, ok := q["charset"]; !ok {
		q.Set("charset", "utf8mb4")
	}
	if enc := q.Encode(); enc != "" {
		b.WriteString("?")
		b.WriteString(enc)
	}

	return b.String(), nil
}
