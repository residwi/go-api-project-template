package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type Service struct {
	repo  Repository
	cache StatusCache
}

func NewService(repo Repository, c StatusCache) *Service {
	return &Service{repo: repo, cache: c}
}

// GetByEmail satisfies auth.UserProvider
func (s *Service) GetByEmail(ctx context.Context, email string) (auth.UserCredentials, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return auth.UserCredentials{}, err
	}
	return auth.UserCredentials{
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

// Create satisfies auth.UserProvider
func (s *Service) Create(ctx context.Context, params auth.CreateUserParams) (auth.UserResult, error) {
	user := &User{
		Email:        params.Email,
		PasswordHash: params.PasswordHash,
		FirstName:    params.FirstName,
		LastName:     params.LastName,
		Role:         "user",
		Active:       true,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return auth.UserResult{}, err
	}

	return auth.UserResult{
		ID:           user.ID,
		Email:        user.Email,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		Role:         user.Role,
		Active:       user.Active,
		TokenVersion: user.TokenVersion,
	}, nil
}

// GetByID satisfies auth.UserProvider
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (auth.UserResult, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return auth.UserResult{}, err
	}
	return auth.UserResult{
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

// invalidateStatusCache drops the cached active/token_version for a user so a
// status change (deactivation, deletion, role change, token revocation) takes
// effect on the next request instead of only after the 30s TTL — otherwise a
// revoked or deactivated user keeps access for up to 30s. Best-effort: a failure
// is logged and the entry still expires on its own.
func (s *Service) invalidateStatusCache(ctx context.Context, userID uuid.UUID) {
	if err := s.cache.Invalidate(ctx, userID); err != nil {
		slog.WarnContext(ctx, "failed to invalidate user status cache", "user_id", userID, "error", err)
	}
}

func (s *Service) CheckStatus(ctx context.Context, userID uuid.UUID) (middleware.UserStatusResult, error) {
	snap, found, err := s.cache.Get(ctx, userID)
	if err != nil {
		slog.WarnContext(ctx, "user status cache read failed, falling back to DB", "error", err)
	} else if found {
		return middleware.UserStatusResult{Active: snap.Active, TokenVersion: snap.TokenVersion}, nil
	}

	active, tokenVersion, err := s.repo.GetStatusByID(ctx, userID)
	if err != nil {
		// A deleted/non-existent user is a definitive "deny", not an infra error:
		// report inactive so middleware returns 401 instead of 500.
		if errors.Is(err, apperror.ErrNotFound) {
			return middleware.UserStatusResult{Active: false}, nil
		}
		return middleware.UserStatusResult{}, err
	}

	if err := s.cache.Put(ctx, userID,
		StatusSnapshot{Active: active, TokenVersion: tokenVersion}, userStatusCacheTTL); err != nil {
		slog.WarnContext(ctx, "user status cache write failed", "error", err)
	}

	return middleware.UserStatusResult{Active: active, TokenVersion: tokenVersion}, nil
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

// UpdateRoleParams and DeleteParams name the actor and the subject. Both are
// uuid.UUID, and the requesterID == targetID guard below does not catch a
// transposition -- swapped, it would act on the admin instead of the target.
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

	// Revoke the target user's outstanding access tokens: the auth middleware
	// rejects a token whose token_version differs from the DB, so bumping it
	// forces a re-auth that reflects the new role.
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
