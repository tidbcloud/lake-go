package golake

import (
	"database/sql/driver"
)

type lakeTx struct {
	dc *LakeConn
}

func (tx *lakeTx) Commit() (err error) {
	if tx.dc == nil || tx.dc.rest == nil {
		return driver.ErrBadConn
	}
	// compatible with old server version
	if tx.dc.rest.sessionState.TxnState != "" {
		_, err = tx.dc.exec(tx.dc.ctx, "COMMIT", nil, nil)
		if err != nil {
			return
		}
	}
	return
}

func (tx *lakeTx) Rollback() (err error) {
	if tx.dc == nil || tx.dc.rest == nil {
		return driver.ErrBadConn
	}
	_, err = tx.dc.exec(tx.dc.ctx, "ROLLBACK", nil, nil)
	if err != nil {
		return
	}
	return
}
