package golake

import (
	"context"
	"database/sql/driver"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const (
	accept                 = "Accept"
	contentType            = "Content-Type"
	jsonContentType        = "application/json; charset=utf-8"
	jsonMediaType          = "application/json"
	arrowStreamContentType = "application/vnd.apache.arrow.stream"
)

type LakeConn struct {
	ctx    context.Context
	cfg    *Config
	cancel context.CancelFunc
	closed int32
	logger *log.Logger
	rest   *APIClient
}

func (dc *LakeConn) columnTypeOptions(location *time.Location) *ColumnTypeOptions {
	opts := defaultColumnTypeOptions()
	if location != nil {
		opts.SetTimezone(location)
	} else if dc.cfg.Location != nil {
		opts.SetTimezone(dc.cfg.Location)
	}
	if dc.cfg != nil {
		if format, ok := dc.cfg.Params["binary_output_format"]; ok {
			opts.SetBinaryOutputFormat(format)
		}
		if mode, ok := dc.cfg.Params["http_json_result_mode"]; ok {
			opts.SetHTTPJSONResultMode(mode)
		}
	}
	return opts
}

func (dc *LakeConn) exec(ctx context.Context, query string, placeholders *[]int, args []driver.Value) (driver.Result, error) {
	ctx = checkQueryID(ctx)
	query, err := buildQuery(query, args, placeholders)
	if err != nil {
		return emptyResult, err
	}
	queryResponse, err := dc.rest.QuerySync(ctx, query)
	if err != nil {
		return emptyResult, err
	}

	affectedRows, err := parseAffectedRows(queryResponse)
	if err != nil {
		return emptyResult, err
	}

	return newLakeResult(affectedRows, 0), nil
}

func (dc *LakeConn) query(ctx context.Context, query string, placeholders *[]int, args []driver.Value) (rows driver.Rows, err error) {
	ctx = checkQueryID(ctx)
	query, err = buildQuery(query, args, placeholders)
	if err != nil {
		return nil, err
	}
	r0, err := dc.rest.StartQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() {
		if err != nil {
			_ = dc.rest.CloseQuery(ctx, r0)
		}
	}()

	if r0.Error != nil {
		return nil, fmt.Errorf("query error: %+v", r0.Error)
	}
	response, err := waitForData(ctx, dc, r0)
	if err != nil {
		return nil, err
	}
	return dc.newNextRows(ctx, response)
}

func (dc *LakeConn) Begin() (driver.Tx, error) {
	return dc.BeginTx(dc.ctx, driver.TxOptions{})
}

func (dc *LakeConn) BeginTx(
	ctx context.Context,
	_ driver.TxOptions) (
	driver.Tx, error) {
	if dc.rest == nil {
		return nil, driver.ErrBadConn
	}
	if _, err := dc.exec(ctx, "BEGIN", nil, nil); err != nil {
		return nil, err
	}
	return &lakeTx{dc}, nil
}

func (dc *LakeConn) cleanup() {
	_ = dc.rest.Logout(dc.ctx)
	dc.rest = nil
	dc.cfg = nil
}

func (dc *LakeConn) Ping(ctx context.Context) error {
	err := dc.rest.Verify(ctx)
	if err != nil {
		return errors.Wrap(err, "ping failed")
	}
	return nil
}

func (dc *LakeConn) Prepare(query string) (driver.Stmt, error) {
	return dc.PrepareContext(dc.ctx, query)
}

func (dc *LakeConn) prepare(_ context.Context, query string) (*lakeStmt, error) {
	logger.WithContext(dc.ctx).Infoln("Prepare")
	if dc.rest == nil {
		return nil, driver.ErrBadConn
	}
	placeholders := placeholders(query)
	stmt := &lakeStmt{
		dc:           dc,
		query:        query,
		placeholders: placeholders,
	}
	return stmt, nil
}

func (dc *LakeConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	ctx = checkQueryID(ctx)
	return dc.prepare(ctx, query)
}

func buildLakeConn(ctx context.Context, config *Config) (*LakeConn, error) {
	dc := &LakeConn{
		ctx:  ctx,
		cfg:  config,
		rest: NewAPIClientFromConfig(config),
	}
	if config.Debug {
		dc.logger = log.New(os.Stderr, "lake: ", log.LstdFlags)
	}
	if err := dc.rest.initializeConnectionInfo(ctx); err != nil {
		_ = dc.rest.Logout(context.Background())
		return nil, err
	}
	return dc, nil
}

func (dc *LakeConn) log(msg ...interface{}) {
	if dc.logger != nil {
		dc.logger.Println(msg...)
	}
}

// Close invalidates and potentially stops any current
// prepared statements and transactions, marking this
// connection as no longer in use.
func (dc *LakeConn) Close() error {
	if atomic.CompareAndSwapInt32(&dc.closed, 0, 1) {
		dc.log("close connection", dc.rest)
		cancel := dc.cancel
		dc.cancel = nil

		if cancel != nil {
			cancel()
		}
		dc.cleanup()
	}
	return nil
}

func (dc *LakeConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return dc.exec(ctx, query, nil, values)
}

func (dc *LakeConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return dc.query(ctx, query, nil, values)
}

func (dc *LakeConn) ExecBatch(ctx context.Context, query string, rows [][]driver.Value) (driver.Result, error) {
	batch, err := dc.prepareBatch(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, args := range rows {
		err = batch.AppendToFile(args)
		if err != nil {
			return nil, err
		}
	}
	err = batch.BatchInsert()
	if err != nil {
		return nil, err
	}
	return newLakeResult(int64(len(rows)), 0), nil
}

// checkQueryID checks if query_id exists in context, if not, generate a new one
func checkQueryID(ctx context.Context) context.Context {
	if _, ok := ctx.Value(ContextKeyQueryID).(string); !ok {
		queryId := uuid.NewString()
		queryId = strings.ReplaceAll(queryId, "-", "")
		ctx = context.WithValue(ctx, ContextKeyQueryID, queryId)
	}
	return ctx
}
