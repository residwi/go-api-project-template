package notification

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/modules/notification/jobs"
	jobspg "github.com/residwi/go-api-project-template/internal/modules/notification/jobs/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Deps struct {
	Repo   Repository
	DB     database.DB
	Logger *slog.Logger
}

type Service struct {
	repo Repository
	Jobs *jobs.Worker
}

func New(d Deps) *Service {
	return &Service{
		repo: d.Repo,
		Jobs: jobs.New(jobspg.New(d.DB), d.Logger),
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
