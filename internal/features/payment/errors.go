package payment

import (
	"fmt"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

var ErrNotRefundable = fmt.Errorf("%w: payment not refundable", errs.ErrConflict)
