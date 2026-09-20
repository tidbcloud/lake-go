package golake

import (
	"context"
	"database/sql/driver"
)

type lakeTx struct {
	dc *LakeConn
	// ctx is the BeginTx context, so COMMIT and ROLLBACK carry the same
	// request metadata (such as the user agent) as the BEGIN they end.
	ctx context.Context
}

// txContext derives the context COMMIT and ROLLBACK run on from the BeginTx
// context. database/sql calls Commit and Rollback without a context, and calls
// Rollback precisely when the BeginTx context is canceled, so its cancellation
// must not apply. Its query ID must not either: Databend/Lake treats a repeated
// query ID as a retry and returns the first query's result, so a COMMIT
// reusing the BEGIN's ID would report success without committing. Each
// statement gets a fresh ID from checkQueryID instead.
func txContext(ctx context.Context) context.Context {
	return contextWithoutQueryID{Context: context.WithoutCancel(ctx)}
}

func (tx *lakeTx) Commit() (err error) {
	if tx.dc == nil || tx.dc.rest == nil {
		return driver.ErrBadConn
	}
	// compatible with old server version
	if tx.dc.rest.sessionState.TxnState != "" {
		_, err = tx.dc.exec(tx.ctx, "COMMIT", nil, nil)
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
	_, err = tx.dc.exec(tx.ctx, "ROLLBACK", nil, nil)
	if err != nil {
		return
	}
	return
}
