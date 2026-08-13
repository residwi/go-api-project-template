package query

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/contract"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

const statusCacheTTL = 30 * time.Second

type Reader struct {
	repo   Repository
	cache  StatusCache
	logger *slog.Logger
}

func New(repo Repository, cache StatusCache, logger *slog.Logger) *Reader {
	return &Reader{repo: repo, cache: cache, logger: logger}
}

func (r *Reader) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *Reader) ListAdmin(ctx context.Context, params Params) ([]domain.User, int, error) {
	return r.repo.ListAdmin(ctx, params)
}

func (r *Reader) CheckStatus(ctx context.Context, userID uuid.UUID) (contract.AccountStatus, error) {
	snap, found, err := r.cache.Get(ctx, userID)
	if err != nil {
		r.logger.WarnContext(
			ctx,
			"user status cache read failed, falling back to DB",
			slog.String("error", err.Error()),
		)
	} else if found {
		return contract.AccountStatus{Active: snap.Active, TokenVersion: snap.TokenVersion}, nil
	}

	active, tokenVersion, err := r.repo.GetStatusByID(ctx, userID)
	if err != nil {
		if errors.Is(err, apperror.ErrNotFound) {
			return contract.AccountStatus{Active: false}, nil
		}
		return contract.AccountStatus{}, err
	}

	if err := r.cache.Put(ctx, userID,
		StatusSnapshot{Active: active, TokenVersion: tokenVersion}, statusCacheTTL); err != nil {
		r.logger.WarnContext(ctx, "user status cache write failed", slog.String("error", err.Error()))
	}

	return contract.AccountStatus{Active: active, TokenVersion: tokenVersion}, nil
}
