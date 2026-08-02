package inventory

import "github.com/google/uuid"

type Stock struct {
	ProductID uuid.UUID
	Quantity  int
	Reserved  int
	Available int
}

// StockState is the prior state of an order's stock, telling Restore whether to
// release a reservation or restock already-deducted goods.
type StockState int

const (
	Reserved StockState = iota
	Deducted
)
