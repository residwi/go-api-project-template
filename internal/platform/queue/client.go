package queue

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func NewInsertClient(db database.DB) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(db.Primary), &river.Config{})
}
