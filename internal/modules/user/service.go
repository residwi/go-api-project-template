package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

type Service struct {
	repo   Repository
	cache  StatusCache
	logger *slog.Logger
}

func NewService(repo Repository, c StatusCache, log *slog.Logger) *Service {
	return &Service{repo: repo, cache: c, logger: log}
}

func (s *Service) GetByEmail(ctx context.Context, email string) (contract.Credentials, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return contract.Credentials{}, err
	}
	return contract.Credentials{
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

func (s *Service) Create(ctx context.Context, params contract.NewUser) (contract.User, error) {
	user := &User{
		Email:        params.Email,
		PasswordHash: params.PasswordHash,
		FirstName:    params.FirstName,
		LastName:     params.LastName,
		Role:         "user",
		Active:       true,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return contract.User{}, err
	}

	return contract.User{
		ID:           user.ID,
		Email:        user.Email,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		Role:         user.Role,
		Active:       user.Active,
		TokenVersion: user.TokenVersion,
	}, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (contract.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return contract.User{}, err
	}
	return contract.User{
		ID:           u.ID,
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Role:         u.Role,
		Active:       u.Active,
		TokenVersion: u.TokenVersion,
	}, nil
}

// userStatusCacheTTL bounds how long a revoked token keeps working: the auth
// middleware reads this cache on every authenticated request.
const userStatusCacheTTL = 30 * time.Second

// CheckStatus satisfies middleware.UserStatusChecker. Cached for 30s, fails open.
func (s *Service) CheckStatus(ctx context.Context, userID uuid.UUID) (contract.AccountStatus, error) {
	snap, found, err := s.cache.Get(ctx, userID)
	if err != nil {
		s.logger.WarnContext(
			ctx,
			"user status cache read failed, falling back to DB",
			slog.String("error", err.Error()),
		)
	} else if found {
		return contract.AccountStatus{Active: snap.Active, TokenVersion: snap.TokenVersion}, nil
	}

	active, tokenVersion, err := s.repo.GetStatusByID(ctx, userID)
	if err != nil {
		// A deleted/non-existent user is a definitive "deny", not an infra error:
		// report inactive so middleware returns 401 instead of 500.
		if errors.Is(err, apperror.ErrNotFound) {
			return contract.AccountStatus{Active: false}, nil
		}
		return contract.AccountStatus{}, err
	}

	if err := s.cache.Put(ctx, userID,
		StatusSnapshot{Active: active, TokenVersion: tokenVersion}, userStatusCacheTTL); err != nil {
		s.logger.WarnContext(ctx, "user status cache write failed", slog.String("error", err.Error()))
	}

	return contract.AccountStatus{Active: active, TokenVersion: tokenVersion}, nil
}

func (s *Service) GetProfile(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, p UpdateProfileParams) (*User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.FirstName != "" {
		u.FirstName = p.FirstName
	}
	if p.LastName != "" {
		u.LastName = p.LastName
	}
	if p.Phone != nil {
		u.Phone = *p.Phone
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}

func (s *Service) List(ctx context.Context, params ListParams) ([]User, int, error) {
	return s.repo.List(ctx, params)
}

func (s *Service) AdminGetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) AdminUpdate(ctx context.Context, id uuid.UUID, p AdminUpdateParams) (*User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.FirstName != "" {
		u.FirstName = p.FirstName
	}
	if p.LastName != "" {
		u.LastName = p.LastName
	}
	if p.Phone != nil {
		u.Phone = *p.Phone
	}
	if p.Active != nil {
		u.Active = *p.Active
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	s.invalidateStatusCache(ctx, id)

	return u, nil
}

// UpdateRoleParams and DeleteParams name their fields because both ids are
// uuid.UUID and the requesterID == targetID guard below does not catch a
// transposition: swapped, it acts on the admin.
type UpdateRoleParams struct {
	RequesterID uuid.UUID
	TargetID    uuid.UUID
	Role        string
}

type DeleteParams struct {
	RequesterID uuid.UUID
	TargetID    uuid.UUID
}

func (s *Service) UpdateRole(ctx context.Context, p UpdateRoleParams) error {
	if p.RequesterID == p.TargetID {
		return fmt.Errorf("%w: cannot change own role", apperror.ErrForbidden)
	}

	u, err := s.repo.GetByID(ctx, p.TargetID)
	if err != nil {
		return err
	}

	if u.Role == "admin" && p.Role == "user" {
		count, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot remove last admin", apperror.ErrBadRequest)
		}
	}

	u.Role = p.Role
	if err := s.repo.Update(ctx, u); err != nil {
		return err
	}

	// Bumping token_version revokes outstanding tokens, forcing a re-auth that
	// reflects the new role.
	if err := s.repo.IncrementTokenVersion(ctx, p.TargetID); err != nil {
		return fmt.Errorf("revoking tokens after role change: %w", err)
	}

	// Invalidate after the bump so the cache repopulates with the new token_version.
	s.invalidateStatusCache(ctx, p.TargetID)
	return nil
}

func (s *Service) Delete(ctx context.Context, p DeleteParams) error {
	if p.RequesterID == p.TargetID {
		return fmt.Errorf("%w: cannot delete own account", apperror.ErrForbidden)
	}

	u, err := s.repo.GetByID(ctx, p.TargetID)
	if err != nil {
		return err
	}

	if u.Role == "admin" {
		count, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot delete last admin", apperror.ErrBadRequest)
		}
	}

	if err := s.repo.Delete(ctx, p.TargetID); err != nil {
		return err
	}
	s.invalidateStatusCache(ctx, p.TargetID)
	return nil
}

// Without this a revoked or deactivated user keeps access until the TTL lapses.
// Best-effort: a failure is logged and the entry still expires on its own.
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
