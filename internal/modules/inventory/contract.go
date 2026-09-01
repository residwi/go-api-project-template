package inventory

type Availability struct {
	OnHand    int
	Available int
}

type StockState int

const (
	StockReserved StockState = iota
	StockDeducted
)

func StockStateOf(deducted bool) StockState {
	if deducted {
		return StockDeducted
	}
	return StockReserved
}
