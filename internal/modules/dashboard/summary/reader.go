package summary

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) GetSummary(
	ctx context.Context,
	from, to time.Time,
) (domain.SalesSummary, []domain.StatusBreakdown, error) {
	var (
		sales     domain.SalesSummary
		breakdown []domain.StatusBreakdown
	)

	// Pass gctx, not ctx: errgroup cancels it the moment either query fails, so the
	// sibling stops instead of running on against a doomed context.
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		sales, err = r.repo.GetSalesSummary(gctx, from, to)
		return err
	})
	g.Go(func() error {
		var err error
		breakdown, err = r.repo.ListOrderStatusBreakdown(gctx, from, to)
		return err
	})
	if err := g.Wait(); err != nil {
		return domain.SalesSummary{}, nil, err
	}
	return sales, breakdown, nil
}
