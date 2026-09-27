package user

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo   Repository
	tracer trace.Tracer
}

func New(repo Repository) *Service {
	return &Service{
		repo:   repo,
		tracer: otel.Tracer("github.com/residwi/go-api-project-template/internal/features/user"),
	}
}

func (s *Service) GetByEmail(ctx context.Context, email string) (_ Credentials, err error) {
	ctx, span := s.tracer.Start(ctx, "user.GetByEmail")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return Credentials{}, err
	}

	return Credentials{Profile: toProfile(u), PasswordHash: u.PasswordHash}, nil
}

func (s *Service) Create(ctx context.Context, params NewUser) (_ Profile, err error) {
	ctx, span := s.tracer.Start(ctx, "user.Create")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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

	return toProfile(u), nil
}

func (s *Service) GetProfile(ctx context.Context, id uuid.UUID) (_ Profile, err error) {
	ctx, span := s.tracer.Start(ctx, "user.GetProfile")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Profile{}, err
	}

	return toProfile(u), nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (_ *domain.User, err error) {
	ctx, span := s.tracer.Start(ctx, "user.GetByID")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) (_ []domain.User, _ int, err error) {
	ctx, span := s.tracer.Start(ctx, "user.ListAdmin")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) UpdateProfile(
	ctx context.Context,
	id uuid.UUID,
	firstName, lastName string,
	phone *string,
) (_ *domain.User, err error) {
	ctx, span := s.tracer.Start(ctx, "user.UpdateProfile")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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

	if err := s.repo.UpdateProfile(ctx, u); err != nil {
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
) (_ *domain.User, err error) {
	ctx, span := s.tracer.Start(ctx, "user.AdminUpdate")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	wasActiveAdmin := u.Role == domain.RoleAdmin && u.Active

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

	applied, err := s.repo.UpdateGuarded(ctx, u)
	if err != nil {
		return nil, err
	}
	if !applied {
		if wasActiveAdmin {
			return nil, fmt.Errorf("%w: cannot deactivate or demote last admin", errs.ErrBadRequest)
		}
		return nil, errs.ErrNotFound
	}

	return u, nil
}

func (s *Service) UpdateRole(ctx context.Context, requesterID, targetID uuid.UUID, role string) (err error) {
	ctx, span := s.tracer.Start(ctx, "user.UpdateRole")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	if requesterID == targetID {
		return fmt.Errorf("%w: cannot change own role", errs.ErrForbidden)
	}

	u, err := s.repo.GetByID(ctx, targetID)
	if err != nil {
		return err
	}

	wasActiveAdmin := u.Role == domain.RoleAdmin && u.Active

	u.Role = role
	applied, err := s.repo.UpdateGuarded(ctx, u)
	if err != nil {
		return err
	}
	if !applied {
		if wasActiveAdmin {
			return fmt.Errorf("%w: cannot remove last admin", errs.ErrBadRequest)
		}
		return errs.ErrNotFound
	}

	if err := s.repo.IncrementTokenVersion(ctx, targetID); err != nil {
		return fmt.Errorf("revoking tokens after role change: %w", err)
	}

	return nil
}

func (s *Service) Delete(ctx context.Context, requesterID, targetID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "user.Delete")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	if requesterID == targetID {
		return fmt.Errorf("%w: cannot delete own account", errs.ErrForbidden)
	}

	u, err := s.repo.GetByID(ctx, targetID)
	if err != nil {
		return err
	}

	applied, err := s.repo.Delete(ctx, targetID)
	if err != nil {
		return err
	}
	if !applied {
		if u.Role == domain.RoleAdmin && u.Active {
			return fmt.Errorf("%w: cannot delete last admin", errs.ErrBadRequest)
		}
		return errs.ErrNotFound
	}

	return nil
}

func toProfile(u *domain.User) Profile {
	return Profile{
		ID:           u.ID,
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Role:         u.Role,
		Active:       u.Active,
		TokenVersion: u.TokenVersion,
	}
}
