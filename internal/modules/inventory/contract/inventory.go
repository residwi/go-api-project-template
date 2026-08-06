// Package contract is inventory's published surface: the only inventory types
// another module may name. It imports no module and no platform package, so
// importing it can never pull inventory's implementation along.
package contract

// Availability carries no reservation count. Reserved is live order velocity per
// SKU, and the modules reading this expose their responses publicly.
type Availability struct {
	OnHand    int
	Available int
}

// StockState is an order's stock state before a reversal, telling inventory
// whether reversing means releasing a reservation or restocking deducted goods.
type StockState int

const (
	Reserved StockState = iota
	Deducted
)
