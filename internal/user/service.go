package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/auth"
	"github.com/residwi/go-api-project-template/internal/middleware"
)

type Service struct {
	repo Repository
	rdb  *redis.Client
}

func NewService(repo Repository, rdb *redis.Client) *Service {
	return &Service{repo: repo, rdb: rdb}
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

func userStatusCacheKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:status:%s", userID.String())
}

// invalidateStatusCache drops the cached active/token_version for a user so a
// status change (deactivation, deletion, role change, token revocation) takes
// effect on the next request instead of only after the 30s TTL — otherwise a
// revoked or deactivated user keeps access for up to 30s. Best-effort: a failure
// is logged and the entry still expires on its own.
func (s *Service) invalidateStatusCache(ctx context.Context, userID uuid.UUID) {
	if s.rdb == nil {
		return
	}
	if err := s.rdb.Del(ctx, userStatusCacheKey(userID)).Err(); err != nil {
		slog.WarnContext(ctx, "failed to invalidate user status cache", "user_id", userID, "error", err)
	}
}

// CheckStatus satisfies middleware.UserStatusChecker. Uses Redis cache (30s TTL), fails-open.
func (s *Service) CheckStatus(ctx context.Context, userID uuid.UUID) (middleware.UserStatusResult, error) {
	if s.rdb != nil {
		key := userStatusCacheKey(userID)
		cached, err := s.rdb.HGetAll(ctx, key).Result()
		if err != nil {
			slog.WarnContext(ctx, "user status cache read failed, falling back to DB", "error", err)
		} else if len(cached) > 0 {
			active := cached["active"] == "1"
			tokenVersion, _ := strconv.Atoi(cached["token_version"])
			return middleware.UserStatusResult{Active: active, TokenVersion: tokenVersion}, nil
		}
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

	result := middleware.UserStatusResult{Active: active, TokenVersion: tokenVersion}

	if s.rdb != nil {
		key := userStatusCacheKey(userID)
		activeStr := "0"
		if active {
			activeStr = "1"
		}
		pipe := s.rdb.Pipeline()
		pipe.HSet(ctx, key, "active", activeStr, "token_version", strconv.Itoa(tokenVersion))
		pipe.Expire(ctx, key, 30*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			slog.WarnContext(ctx, "user status cache write failed", "error", err)
		}
	}

	return result, nil
}

func (s *Service) GetProfile(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, req UpdateProfileRequest) (*User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.FirstName != "" {
		u.FirstName = req.FirstName
	}
	if req.LastName != "" {
		u.LastName = req.LastName
	}
	if req.Phone != nil {
		u.Phone = *req.Phone
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

func (s *Service) AdminUpdate(ctx context.Context, id uuid.UUID, req AdminUpdateUserRequest) (*User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.FirstName != "" {
		u.FirstName = req.FirstName
	}
	if req.LastName != "" {
		u.LastName = req.LastName
	}
	if req.Phone != nil {
		u.Phone = *req.Phone
	}
	if req.Active != nil {
		u.Active = *req.Active
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
