package readonlysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
)

type probeConnector struct{ calls int }
type probeDriver struct{}
type probeConn struct{}

func (p *probeConnector) Connect(context.Context) (driver.Conn, error) {
	p.calls++
	return &probeConn{}, nil
}
func (p *probeConnector) Driver() driver.Driver { return probeDriver{} }
func (probeDriver) Open(string) (driver.Conn, error) {
	return &probeConn{}, nil
}
func (*probeConn) Close() error {
	return nil
}
func (*probeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("probe only")
}
func (*probeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("probe only")
}
func (*probeConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return nil, errors.New("probe only")
}
func (*probeConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, errors.New("probe only")
}

func TestF4ConnectorCannotReopen(t *testing.T) {
	probe := &probeConnector{}
	wrapped := Wrap(probe)
	first, err := wrapped.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if next, err := wrapped.Connect(context.Background()); err == nil || next != nil || probe.calls != 1 {
		t.Fatal("connection-relative replay fence allowed a transparent reconnect")
	}
	if next, err := wrapped.Driver().Open("anything"); err == nil || next != nil {
		t.Fatal("underlying driver escaped the connector capability")
	}
}
