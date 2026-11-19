// Package readonlysql provides a driver-level query-only capability. It
// has no dependency on CSF ports, commit writers or persistence adapters.
// The underlying connector must already enforce SQLite mode=ro/query_only.
package readonlysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
)

var errDenied = errors.New("read-only SQL capability denied operation")

type connector struct{ inner driver.Connector }

// Wrap does not expose the underlying connection or driver to the caller.
func Wrap(inner driver.Connector) driver.Connector { return &connector{inner: inner} }

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	raw, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := raw.(driver.QueryerContext); !ok {
		raw.Close()
		return nil, errDenied
	}
	if _, ok := raw.(driver.ConnBeginTx); !ok {
		raw.Close()
		return nil, errDenied
	}
	return &conn{raw: raw}, nil
}
func (c *connector) Driver() driver.Driver { return deniedDriver{} }

type deniedDriver struct{}

func (deniedDriver) Open(string) (driver.Conn, error) { return nil, errDenied }

type conn struct{ raw driver.Conn }

func (c *conn) Close() error {
	return c.raw.Close()
}
func (c *conn) Prepare(string) (driver.Stmt, error) {
	return nil, errDenied
}
func (c *conn) Begin() (driver.Tx, error) {
	return nil, errDenied
}
func (c *conn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if !options.ReadOnly {
		return nil, errDenied
	}
	return c.raw.(driver.ConnBeginTx).BeginTx(ctx, options)
}
func (c *conn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, errDenied
}
func (c *conn) Ping(ctx context.Context) error {
	if p, ok := c.raw.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}
func (c *conn) CheckNamedValue(v *driver.NamedValue) error {
	if checker, ok := c.raw.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(v)
	}
	return driver.ErrSkip
}

// Queries are single statements with a closed PRAGMA inventory. This is
// defense-in-depth for fixed internal SQL, not a general SQL sandbox for
// arbitrary untrusted query text. No Exec, ATTACH, DDL, DML, writable
// PRAGMAs, prepared statement or additional driver may escape through it.
func allowed(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if strings.Contains(q, ";") || strings.Contains(q, "load_extension") || strings.Contains(q, "writefile") ||
		strings.Contains(q, "readfile") {
		return false
	}
	switch q {
	case "pragma data_version", "pragma query_only", "pragma journal_mode", "pragma synchronous", "pragma compile_options", "pragma user_version":
		return true
	}
	return strings.HasPrefix(q, "select ") || strings.HasPrefix(q, "select\n") || strings.HasPrefix(q, "select\t")
}

func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !allowed(query) {
		return nil, errDenied
	}
	return c.raw.(driver.QueryerContext).QueryContext(ctx, query, args)
}
