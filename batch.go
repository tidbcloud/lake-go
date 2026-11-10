package golake

import (
	"bufio"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/pkg/errors"
)

// \x60 represents a backtick
var httpInsertRe = regexp.MustCompile(`(?i)^\s*(?:INSERT|REPLACE) INTO\s+\x60?([\w.^\(]+)\x60?(?:\s*\([^\)]*\))?(?:\s+ON\s*\([^\)]*\))?(?:\s*\([^\)]*\))?\s+VALUES`)

type BatchStmt struct {
	query string
}

func PrepareBatch(query string) (stmt *BatchStmt, err error) {
	stmt = &BatchStmt{
		query: query,
	}
	return stmt, nil
}

func (stmt *BatchStmt) ExecBatch(ctx context.Context, conn *sql.Conn, rows [][]driver.Value) (result driver.Result, err error) {
	err = conn.Raw(func(rawConn interface{}) error {
		bendConn := rawConn.(*LakeConn)
		result, err = bendConn.ExecBatch(ctx, stmt.query, rows)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type Batch interface {
	AppendToFile(v []driver.Value) error
	BatchInsert() error
}

func (dc *LakeConn) prepareBatch(ctx context.Context, query string) (*httpBatch, error) {
	matches := httpInsertRe.FindStringSubmatch(query)
	if len(matches) < 2 {
		return nil, errors.New("PrepareBatch only support INSERT/REPLACE")
	}
	csvFile, err := os.CreateTemp("", "lake-batch-*.csv")
	if err != nil {
		return nil, err
	}

	return &httpBatch{
		query:     query,
		ctx:       ctx,
		conn:      dc,
		batchFile: csvFile,
	}, nil
}

type httpBatch struct {
	query     string
	ctx       context.Context
	conn      *LakeConn
	batchFile *os.File
}

func (b *httpBatch) cleanup() {
	b.batchFile.Close()
	if err := os.Remove(b.batchFile.Name()); err != nil {
		b.conn.log("delete batch insert file failed: ", err)
	}
}

func (b *httpBatch) BatchInsert() error {
	stage, err := b.UploadToStage(context.Background())
	if err != nil {
		return errors.Wrap(err, "upload to stage failed")
	}

	_, err = b.conn.rest.InsertWithStage(b.ctx, b.query, stage, nil, nil)
	if err != nil {
		return errors.Wrap(err, "insert with stage failed")
	}
	return nil
}

func (b *httpBatch) AppendToFile(row []driver.Value) error {
	lineData := make([]string, 0, len(row))
	for _, v := range row {
		var s string
		switch v := v.(type) {
		case string:
			s = v
		case time.Time:
			s = v.Format(timeFormat)
		case date:
			s = time.Time(v).Format(dateFormat)
		default:
			bytes, err := textEncode.Encode(v)
			if err != nil {
				return err
			}
			s = string(bytes)
		}
		lineData = append(lineData, s)
	}
	writer := csv.NewWriter(b.batchFile)
	if err := writer.Write(lineData); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

func (b *httpBatch) UploadToStage(ctx context.Context) (*StageLocation, error) {
	ctx = checkQueryID(ctx)
	fi, err := b.batchFile.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "get batch file size failed")
	}
	size := fi.Size()

	if _, err := b.batchFile.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Wrap(err, "rewind batch file failed")
	}
	input := bufio.NewReader(b.batchFile)
	stage := &StageLocation{
		Name: "~",
		Path: fmt.Sprintf("batch/%d-%s", time.Now().Unix(), filepath.Base(b.batchFile.Name())),
	}
	return stage, b.conn.rest.UploadToStage(ctx, stage, input, size)
}
