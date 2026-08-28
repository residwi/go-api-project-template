package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

type Service struct {
	repo   Repository
	cache  StatusCache
	logger *slog.Logger
}

func New(repo Repository, cache StatusCache, logger *slog.Logger) *Service {
	return &Service{repo: repo, cache: cache, logger: logger}
}

func (s *Service) GetByEmail(ctx context.Context, email string) (Credentials, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Role:         u.Role,
		Active:       u.Active,
		TokenVersion: u.TokenVersion,
	}, nil
}

func (s *Service) Create(ctx context.Context, params NewUser) (Profile, error) {
	u := &domain.User{
		Email:        params.Email,
		PasswordHash: params.PasswordHash,
		FirstName:    params.FirstName,
		LastName:     params.LastName,
		Role:         domain.RoleUser,
		Active:       true,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return Profile{}, err
	}

	return Profile{
		ID:           u.ID,
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Role:         u.Role,
		Active:       u.Active,
		TokenVersion: u.TokenVersion,
	}, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Profile, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	return Profile{
		ID:           u.ID,
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Role:         u.Role,
		Active:       u.Active,
		TokenVersion: u.TokenVersion,
	}, nil
}

func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.User, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) CheckStatus(ctx context.Context, userID uuid.UUID) (AccountStatus, error) {
	snap, found, err := s.cache.Get(ctx, userID)
	if err != nil {
		s.logger.WarnContext(
			ctx,
			"user status cache read failed, falling back to DB",
			slog.String("error", err.Error()),
		)
	} else if found {
		return AccountStatus(snap), nil
	}

	active, tokenVersion, err := s.repo.GetStatusByID(ctx, userID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return AccountStatus{Active: false}, nil
		}
		return AccountStatus{}, err
	}

	if err := s.cache.Put(ctx, userID,
		StatusSnapshot{Active: active, TokenVersion: tokenVersion}, statusCacheTTL); err != nil {
		s.logger.WarnContext(ctx, "user status cache write failed", slog.String("error", err.Error()))
	}

	return AccountStatus{Active: active, TokenVersion: tokenVersion}, nil
}

func (s *Service) UpdateProfile(
	ctx context.Context,
	id uuid.UUID,
	firstName, lastName string,
	phone *string,
) (*domain.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if firstName != "" {
		u.FirstName = firstName
	}
	if lastName != "" {
		u.LastName = lastName
	}
	if phone != nil {
		u.Phone = *phone
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}

func (s *Service) AdminUpdate(
	ctx context.Context,
	id uuid.UUID,
	firstName, lastName string,
	phone *string,
	active *bool,
) (*domain.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if firstName != "" {
		u.FirstName = firstName
	}
	if lastName != "" {
		u.LastName = lastName
	}
	if phone != nil {
		u.Phone = *phone
	}
	if active != nil {
		u.Active = *active
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	s.invalidateStatusCache(ctx, id)

	return u, nil
}

func (s *Service) UpdateRole(ctx context.Context, requesterID, targetID uuid.UUID, role string) error {
	if requesterID == targetID {
		return fmt.Errorf("%w: cannot change own role", errs.ErrForbidden)
	}

	u, err := s.repo.GetByID(ctx, targetID)
	if err != nil {
		return err
	}

	if u.Role == domain.RoleAdmin && role == domain.RoleUser {
		count, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot remove last admin", errs.ErrBadRequest)
		}
	}

	u.Role = role
	if err := s.repo.Update(ctx, u); err != nil {
		return err
	}

	if err := s.repo.IncrementTokenVersion(ctx, targetID); err != nil {
		return fmt.Errorf("revoking tokens after role change: %w", err)
	}

	s.invalidateStatusCache(ctx, targetID)
	return nil
}

func (s *Service) Delete(ctx context.Context, requesterID, targetID uuid.UUID) error {
	if requesterID == targetID {
		return fmt.Errorf("%w: cannot delete own account", errs.ErrForbidden)
	}

	u, err := s.repo.GetByID(ctx, targetID)
	if err != nil {
		return err
	}

	if u.Role == domain.RoleAdmin {
		count, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot delete last admin", errs.ErrBadRequest)
		}
	}

	if err := s.repo.Delete(ctx, targetID); err != nil {
		return err
	}

	s.invalidateStatusCache(ctx, targetID)
	return nil
}

func (s *Service) invalidateStatusCache(ctx context.Context, userID uuid.UUID) {
	if err := s.cache.Invalidate(ctx, userID); err != nil {
		s.logger.WarnContext(
			ctx,
			"failed to invalidate user status cache",
			slog.String("target_user_id", userID.String()),
			slog.String("error", err.Error()),
		)
	}
}

const statusCacheTTL = 30 * time.Second
