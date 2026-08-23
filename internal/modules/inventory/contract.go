package inventory

// Availability is the read shape product asks for -- on-hand quantity and
// what remains free to sell, without exposing the reservation mechanics
// behind it.
type Availability struct {
	OnHand    int
	Available int
}

// StockState tells Restore which column a caller's prior write touched, so
// it can undo the right one: a reservation still sitting in reserved_stock,
// or a deduction already moved into available_stock's ledger.
type StockState int

const (
	Reserved StockState = iota
	Deducted
)
