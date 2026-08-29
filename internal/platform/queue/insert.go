package queue

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func Insert(
	ctx context.Context,
	client *river.Client[pgx.Tx],
	db database.DB,
	args river.JobArgs,
	opts *river.InsertOpts,
) error {
	if tx, ok := database.PrimaryDB(ctx, db).(pgx.Tx); ok {
		_, err := client.InsertTx(ctx, tx, args, opts)
		return err
	}

	_, err := client.Insert(ctx, args, opts)
	return err
}
