package credentials

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/contract"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

type Store struct {
	repo Repository
}

func New(repo Repository) *Store {
	return &Store{repo: repo}
}

func (s *Store) GetByEmail(ctx context.Context, email string) (contract.Credentials, error) {
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

func (s *Store) Create(ctx context.Context, params contract.NewUser) (contract.User, error) {
	user := &domain.User{
		Email:        params.Email,
		PasswordHash: params.PasswordHash,
		FirstName:    params.FirstName,
		LastName:     params.LastName,
		Role:         domain.RoleUser,
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

func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (contract.User, error) {
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
