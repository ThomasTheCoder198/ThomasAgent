package auth

import (
	"context"
	stderrors "errors"

	"github.com/jackc/pgx/v5"
)

type rollbackFailureDB struct {
	transactionDB
	failure error
}

func (db rollbackFailureDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := db.transactionDB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return rollbackFailureTx{Tx: tx, failure: db.failure}, nil
}

type rollbackFailureTx struct {
	pgx.Tx
	failure error
}

func (tx rollbackFailureTx) Rollback(ctx context.Context) error {
	return stderrors.Join(tx.Tx.Rollback(ctx), tx.failure)
}
