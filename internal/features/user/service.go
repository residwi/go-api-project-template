package user

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/auth"
	"github.com/residwi/go-api-project-template/internal/middleware"
)

type Service struct {
	repo Repository
	pool *pgxpool.Pool
	rdb  *redis.Client
}

func NewService(repo Repository, pool *pgxpool.Pool, rdb *redis.Client) *Service {
	return &Service{repo: repo, pool: pool, rdb: rdb}
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

// CheckStatus satisfies middleware.UserStatusChecker. Uses Redis cache (30s TTL), fails-open.
func (s *Service) CheckStatus(ctx context.Context, userID uuid.UUID) (middleware.UserStatusResult, error) {
	if s.rdb != nil {
		key := fmt.Sprintf("user:status:%s", userID.String())
		cached, err := s.rdb.HGetAll(ctx, key).Result()
		if err != nil {
			slog.WarnContext(ctx, "user status cache read failed, falling back to DB", "error", err)
		} else if len(cached) > 0 {
			active := cached["active"] == "1"
			var tokenVersion int
			_, _ = fmt.Sscanf(cached["token_version"], "%d", &tokenVersion)
			return middleware.UserStatusResult{Active: active, TokenVersion: tokenVersion}, nil
		}
	}

	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return middleware.UserStatusResult{}, err
	}

	result := middleware.UserStatusResult{Active: u.Active, TokenVersion: u.TokenVersion}

	if s.rdb != nil {
		key := fmt.Sprintf("user:status:%s", userID.String())
		activeStr := "0"
		if u.Active {
			activeStr = "1"
		}
		pipe := s.rdb.Pipeline()
		pipe.HSet(ctx, key, "active", activeStr, "token_version", strconv.Itoa(u.TokenVersion))
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

	return u, nil
}

func (s *Service) UpdateRole(ctx context.Context, requesterID, targetID uuid.UUID, role string) error {
	if requesterID == targetID {
		return fmt.Errorf("%w: cannot change own role", core.ErrForbidden)
	}

	u, err := s.repo.GetByID(ctx, targetID)
	if err != nil {
		return err
	}

	if u.Role == "admin" && role == "user" {
		count, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot remove last admin", core.ErrBadRequest)
		}
	}

	u.Role = role
	return s.repo.Update(ctx, u)
}

func (s *Service) Delete(ctx context.Context, requesterID, targetID uuid.UUID) error {
	if requesterID == targetID {
		return fmt.Errorf("%w: cannot delete own account", core.ErrForbidden)
	}

	u, err := s.repo.GetByID(ctx, targetID)
	if err != nil {
		return err
	}

	if u.Role == "admin" {
		count, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot delete last admin", core.ErrBadRequest)
		}
	}

	return s.repo.Delete(ctx, targetID)
}
