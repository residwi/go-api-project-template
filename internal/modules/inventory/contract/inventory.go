package contract

type Availability struct {
	OnHand    int
	Available int
}

type StockState int

const (
	Reserved StockState = iota
	Deducted
)
