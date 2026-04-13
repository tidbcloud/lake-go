package golake

import (
	"database/sql/driver"

	"github.com/pkg/errors"
)

type lakeResult struct {
	affectedRows int64
	insertId     int64
}

func newLakeResult(affectedRows, insertId int64) *lakeResult {
	return &lakeResult{
		affectedRows: affectedRows,
		insertId:     insertId,
	}
}

func (res *lakeResult) LastInsertId() (int64, error) {
	return res.insertId, errors.New("LastInsertId is not supported")
}

func (res *lakeResult) RowsAffected() (int64, error) {
	return res.affectedRows, nil
}

var emptyResult driver.Result = noResult{}

type noResult struct{}

func (noResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (noResult) RowsAffected() (int64, error) {
	return 0, nil
}
