package domain

import "slices"

type Transition struct {
	To                Status
	From              []Status
	SetsStockDeducted bool
	SetsStockReversed bool
}

//nolint:gochecknoglobals // immutable named transitions; struct/slice values cannot be const
var (
	PaymentProcessingTransition = Transition{
		To:   StatusPaymentProcessing,
		From: []Status{StatusAwaitingPayment, StatusPaymentProcessing},
	}

	// PaymentAttemptTransition refuses an order already processing, unlike
	// PaymentProcessingTransition, so of two concurrent retries only one wins.
	PaymentAttemptTransition = Transition{
		To:   StatusPaymentProcessing,
		From: []Status{StatusAwaitingPayment},
	}

	AwaitingPaymentTransition = Transition{To: StatusAwaitingPayment, From: []Status{StatusPaymentProcessing}}

	PaidTransition = Transition{
		To:                StatusPaid,
		From:              []Status{StatusPaymentProcessing, StatusAwaitingPayment},
		SetsStockDeducted: true,
	}

	FulfillmentFailedAfterChargeTransition = Transition{
		To:   StatusFulfillmentFailed,
		From: []Status{StatusCancelled, StatusExpired, StatusPaid},
	}

	FulfillmentFailedCompensatingTransition = Transition{
		To:   StatusFulfillmentFailed,
		From: []Status{StatusPaymentProcessing, StatusAwaitingPayment, StatusCancelled, StatusExpired, StatusPaid},
	}

	RefundTransition = Transition{
		To: StatusRefunded,
		From: []Status{
			StatusFulfillmentFailed,
			StatusPaid,
			StatusProcessing,
			StatusShipped,
			StatusDelivered,
		},
		SetsStockReversed: true,
	}

	ShippedTransition = Transition{To: StatusShipped, From: []Status{StatusPaid, StatusProcessing}}

	DeliveredTransition = Transition{To: StatusDelivered, From: []Status{StatusShipped}}

	CancelledTransition = Transition{
		To:                StatusCancelled,
		From:              []Status{StatusAwaitingPayment, StatusPaymentProcessing, StatusFulfillmentFailed},
		SetsStockReversed: true,
	}

	ExpiredTransition = Transition{To: StatusExpired, From: []Status{StatusAwaitingPayment}, SetsStockReversed: true}

	ProcessingTransition = Transition{To: StatusProcessing, From: []Status{StatusPaid}}
)

//nolint:gochecknoglobals // immutable registry of the named transitions above
var allTransitions = []Transition{
	PaymentProcessingTransition,
	PaymentAttemptTransition,
	AwaitingPaymentTransition,
	PaidTransition,
	FulfillmentFailedAfterChargeTransition,
	FulfillmentFailedCompensatingTransition,
	RefundTransition,
	ShippedTransition,
	DeliveredTransition,
	CancelledTransition,
	ExpiredTransition,
	ProcessingTransition,
}

func CanTransition(from, to Status) bool {
	for _, t := range allTransitions {
		if t.To == to && slices.Contains(t.From, from) {
			return true
		}
	}
	return false
}
