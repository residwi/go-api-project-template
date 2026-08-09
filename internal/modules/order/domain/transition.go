package domain

import "slices"

// Transition is a guarded order-status change: a compare-and-set that moves an
// order to To only if its current status is one of From.
//
// The named transitions below are the whole state machine. Every allowed-from
// set lives here exactly once, so no call site can drift from another. Some
// From sets are deliberately broad, covering payment's race-recovery edges
// (e.g. a gateway confirming before the local flip to payment_processing).
type Transition struct {
	To   Status
	From []Status
	// SetsStockDeducted rides along in the same compare-and-set, so the flag can
	// never disagree with the status. True only for PaidTransition.
	SetsStockDeducted bool
	// SetsStockReversed makes a later reversal a no-op instead of double-releasing
	// -- refunding an order already cancelled-and-released would otherwise take
	// another order's reservation.
	SetsStockReversed bool
}

//nolint:gochecknoglobals // immutable named transitions; struct/slice values cannot be const
var (
	// PaymentProcessingTransition claims an order for charge processing
	// (idempotent if it is already processing).
	PaymentProcessingTransition = Transition{
		To:   StatusPaymentProcessing,
		From: []Status{StatusAwaitingPayment, StatusPaymentProcessing},
	}

	AwaitingPaymentTransition = Transition{To: StatusAwaitingPayment, From: []Status{StatusPaymentProcessing}}

	// PaidTransition allows awaiting_payment for the race where the gateway confirms
	// before the local flip to payment_processing.
	PaidTransition = Transition{
		To:                StatusPaid,
		From:              []Status{StatusPaymentProcessing, StatusAwaitingPayment},
		SetsStockDeducted: true,
	}

	// FulfillmentFailedAfterChargeTransition marks an order fulfillment_failed
	// when a charge succeeds on an already-terminal order.
	FulfillmentFailedAfterChargeTransition = Transition{
		To:   StatusFulfillmentFailed,
		From: []Status{StatusCancelled, StatusExpired, StatusPaid},
	}

	// FulfillmentFailedCompensatingTransition marks an order fulfillment_failed
	// from the compensating-refund path (a broader set of prior states).
	FulfillmentFailedCompensatingTransition = Transition{
		To:   StatusFulfillmentFailed,
		From: []Status{StatusPaymentProcessing, StatusAwaitingPayment, StatusCancelled, StatusExpired, StatusPaid},
	}

	// RefundTransition marks an order refunded from any post-paid state. The
	// refund reverses the order's inventory hold, so this marks it reversed.
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

	// CancelledTransition reverses the inventory hold in the same transaction, so it
	// marks the hold reversed.
	CancelledTransition = Transition{
		To:                StatusCancelled,
		From:              []Status{StatusAwaitingPayment, StatusPaymentProcessing, StatusFulfillmentFailed},
		SetsStockReversed: true,
	}

	// ExpiredTransition releases the reservation, so it marks the hold reversed.
	ExpiredTransition = Transition{To: StatusExpired, From: []Status{StatusAwaitingPayment}, SetsStockReversed: true}

	ProcessingTransition = Transition{To: StatusProcessing, From: []Status{StatusPaid}}
)

// Every named Transition above must appear here: CanTransition is derived from
// this registry.
//
//nolint:gochecknoglobals // immutable registry of the named transitions above
var allTransitions = []Transition{
	PaymentProcessingTransition,
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
