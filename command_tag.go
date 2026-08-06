package postgres

import (
	"database/sql/driver"
	"errors"
	"strconv"
	"strings"
)

type commandResult struct{ tag string }

func (r commandResult) LastInsertId() (int64, error) {
	return 0, errors.New("postgres: LastInsertId is unsupported; use RETURNING")
}

func (r commandResult) RowsAffected() (int64, error) {
	parts := strings.Fields(r.tag)
	if len(parts) == 0 {
		return 0, nil
	}
	value, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		return 0, nil
	}
	return value, nil
}

var _ driver.Result = commandResult{}
