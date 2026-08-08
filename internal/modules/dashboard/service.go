package dashboard

// Service is now an empty husk: every read it used to serve moved to a slice.
// The next commit removes it along with repository.go and postgres/.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}
