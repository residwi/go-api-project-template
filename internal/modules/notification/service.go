package notification

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/modules/notification/jobs"
	jobspg "github.com/residwi/go-api-project-template/internal/modules/notification/jobs/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Deps struct {
	Repo   Repository
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

type Service struct {
	repo Repository
	// Jobs is exported because it is not a slice: notification/jobs is the
	// queue-draining worker at the feature root, consumed directly by
	// order/checkout (name-matched against order.NotificationEnqueuer) and by
	// cmd/worker/main.go, which hands it to jobs.Runner as both queue and
	// processor.
	Jobs *jobs.Worker
}

func New(d Deps) *Service {
	return &Service{
		repo: d.Repo,
		Jobs: jobs.New(jobspg.New(d.Pool), d.Logger),
	}
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Notification, error) {
	return s.repo.ListByUser(ctx, userID, cursor)
}

func (s *Service) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.CountUnread(ctx, userID)
}

func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.MarkRead(ctx, userID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, userID)
}
