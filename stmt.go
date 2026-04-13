package golake

import (
	"context"
	"database/sql/driver"
	"errors"
)

var (
	errStmtClosed = errors.New("stmt is already closed")
)

type lakeStmt struct {
	dc           *LakeConn
	query        string
	placeholders []int
	closed       bool
}

func (stmt *lakeStmt) Close() error {
	stmt.closed = true
	return nil
}

func (stmt *lakeStmt) NumInput() int {
	return len(stmt.placeholders)
}

func (stmt *lakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	if stmt.closed {
		return nil, errStmtClosed
	}
	return stmt.dc.exec(context.Background(), stmt.query, &stmt.placeholders, args)
}

func (stmt *lakeStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if stmt.closed {
		return nil, errStmtClosed
	}
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return stmt.dc.exec(ctx, stmt.query, &stmt.placeholders, values)
}

func (stmt *lakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	if stmt.closed {
		return nil, errStmtClosed
	}
	return stmt.dc.query(context.Background(), stmt.query, &stmt.placeholders, args)
}

func (stmt *lakeStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if stmt.closed {
		return nil, errStmtClosed
	}
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return stmt.dc.query(ctx, stmt.query, &stmt.placeholders, values)
}
