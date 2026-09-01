package domain

type Transition struct {
	To    Status
	From  []Status
	stock stockEffect
}

func (t Transition) DeductsStock() bool { return t.stock == stockDeducted }

func (t Transition) ReversesStock() bool { return t.stock == stockReversed }

//nolint:gochecknoglobals // immutable state table; struct and slice values cannot be const
var (
	ToAwaitingPayment = Transition{
		To:   StatusAwaitingPayment,
		From: []Status{StatusPaymentProcessing},
	}

	ToPaymentProcessing = Transition{
		To:   StatusPaymentProcessing,
		From: []Status{StatusAwaitingPayment, StatusPaymentProcessing},
	}

	ToPaymentAttempt = Transition{
		To:   StatusPaymentProcessing,
		From: []Status{StatusAwaitingPayment},
	}

	ToPaid = Transition{
		To:    StatusPaid,
		From:  []Status{StatusAwaitingPayment, StatusPaymentProcessing},
		stock: stockDeducted,
	}

	ToProcessing = Transition{
		To:   StatusProcessing,
		From: []Status{StatusPaid},
	}

	ToShipped = Transition{
		To:   StatusShipped,
		From: []Status{StatusPaid, StatusProcessing},
	}

	ToDelivered = Transition{
		To:   StatusDelivered,
		From: []Status{StatusShipped},
	}

	ToFulfillmentFailedAfterCharge = Transition{
		To:   StatusFulfillmentFailed,
		From: []Status{StatusPaid, StatusCancelled, StatusExpired},
	}

	ToFulfillmentFailedCompensating = Transition{
		To: StatusFulfillmentFailed,
		From: []Status{
			StatusAwaitingPayment,
			StatusPaymentProcessing,
			StatusPaid,
			StatusCancelled,
			StatusExpired,
		},
	}

	ToCancelled = Transition{
		To:    StatusCancelled,
		From:  []Status{StatusAwaitingPayment, StatusPaymentProcessing, StatusFulfillmentFailed},
		stock: stockReversed,
	}

	ToExpired = Transition{
		To:    StatusExpired,
		From:  []Status{StatusAwaitingPayment},
		stock: stockReversed,
	}

	ToRefunded = Transition{
		To: StatusRefunded,
		From: []Status{
			StatusPaid,
			StatusProcessing,
			StatusShipped,
			StatusDelivered,
			StatusFulfillmentFailed,
		},
		stock: stockReversed,
	}
)

type stockEffect uint8

const (
	stockUnchanged stockEffect = iota
	stockDeducted
	stockReversed
)
