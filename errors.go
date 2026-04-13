package golake

import (
	"github.com/pkg/errors"
)

var (
	ErrPlaceholderCount = errors.New("lake: wrong placeholder count")
	ErrNoLastInsertID   = errors.New("no LastInsertId available")
	ErrNoRowsAffected   = errors.New("no RowsAffected available")
)
