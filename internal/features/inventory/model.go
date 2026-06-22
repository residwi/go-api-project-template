package inventory

import "github.com/google/uuid"

type Stock struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
	Reserved  int       `json:"reserved"`
	Available int       `json:"available"`
}

// StockState is the prior state of an order's stock, telling Restore whether to
// release a reservation or restock already-deducted goods.
type StockState int

const (
	Reserved StockState = iota
	Deducted
)
