package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// UserGetter is what Handler needs from query.Reader: query.Reader satisfies
// it directly, so nothing sits between them, and the mockery-generated mock
// is the other implementation, used in handler_test.go.
type UserGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

type Handler struct {
	reader UserGetter
}

func New(reader UserGetter) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("GET /users/me", h.me)
}

// The public self-service shape. role came with the auth token, active is implied
// by authenticating at all, and the timestamps are not the profile's business.
// adminUserResponse carries all three, which is why the two are not merged.
type userResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Phone     string    `json:"phone,omitempty"`
}

// PasswordHash, TokenVersion and DeletedAt are structurally absent, not omitted:
// there is no tag to strip.
func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Phone:     u.Phone,
	}
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	u, err := h.reader.GetByID(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toUserResponse(u))
}
