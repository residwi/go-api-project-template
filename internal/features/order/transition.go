package order

import "slices"

// Transition is a guarded order-status change: an atomic compare-and-set that
// moves an order to To only if its current status is one of From.
//
// The named transitions below are the single source of truth for the order
// state machine. Every legal status change is one of these values: cross-feature
// callers apply one through Service.Apply, and the admin/cancel paths validate
// against them via CanTransition — so each allowed-from set lives in exactly one
// place and cannot drift between call sites (which is what let the refund
// transition drift and miss processing/shipped). Some From sets are
// intentionally broad to cover payment's race-recovery and idempotent edges
// (e.g. a gateway confirming before the local flip to payment_processing).
type Transition struct {
	To   Status
	From []Status
}

//nolint:gochecknoglobals // immutable named transitions; struct/slice values cannot be const
var (
	// PaymentProcessingTransition claims an order for charge processing
	// (idempotent if it is already processing).
	PaymentProcessingTransition = Transition{To: StatusPaymentProcessing, From: []Status{StatusAwaitingPayment, StatusPaymentProcessing}}

	// AwaitingPaymentTransition returns an order to awaiting payment after an
	// abandoned/failed charge attempt.
	AwaitingPaymentTransition = Transition{To: StatusAwaitingPayment, From: []Status{StatusPaymentProcessing}}

	// PaidTransition marks an order paid. awaiting_payment is allowed to cover
	// the race where the gateway confirms before the local flip to
	// payment_processing.
	PaidTransition = Transition{To: StatusPaid, From: []Status{StatusPaymentProcessing, StatusAwaitingPayment}}

	// FulfillmentFailedAfterChargeTransition marks an order fulfillment_failed
	// when a charge succeeds on an already-terminal order.
	FulfillmentFailedAfterChargeTransition = Transition{To: StatusFulfillmentFailed, From: []Status{StatusCancelled, StatusExpired, StatusPaid}}

	// FulfillmentFailedCompensatingTransition marks an order fulfillment_failed
	// from the compensating-refund path (a broader set of prior states).
	FulfillmentFailedCompensatingTransition = Transition{To: StatusFulfillmentFailed, From: []Status{StatusPaymentProcessing, StatusAwaitingPayment, StatusCancelled, StatusExpired, StatusPaid}}

	// RefundTransition marks an order refunded from any post-paid state.
	RefundTransition = Transition{To: StatusRefunded, From: []Status{StatusFulfillmentFailed, StatusPaid, StatusProcessing, StatusShipped, StatusDelivered}}

	// ShippedTransition marks an order shipped.
	ShippedTransition = Transition{To: StatusShipped, From: []Status{StatusPaid, StatusProcessing}}

	// DeliveredTransition marks an order delivered.
	DeliveredTransition = Transition{To: StatusDelivered, From: []Status{StatusShipped}}

	// CancelledTransition cancels an order that has not yet been paid for, or one
	// whose fulfillment failed (user- or admin-initiated).
	CancelledTransition = Transition{To: StatusCancelled, From: []Status{StatusAwaitingPayment, StatusPaymentProcessing, StatusFulfillmentFailed}}

	// ExpiredTransition expires an order whose payment window lapsed (applied by
	// the worker's expiry sweep).
	ExpiredTransition = Transition{To: StatusExpired, From: []Status{StatusAwaitingPayment}}

	// ProcessingTransition moves a paid order into fulfillment processing.
	ProcessingTransition = Transition{To: StatusProcessing, From: []Status{StatusPaid}}
)

// allTransitions is the registry CanTransition is derived from: the complete set
// of legal edges in the state machine. Every named Transition above must appear
// here.
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

// CanTransition reports whether moving an order from `from` to `to` is a legal
// edge of the state machine — i.e. some named Transition targets `to` and lists
// `from` in its allowed-from set.
func CanTransition(from, to Status) bool {
	for _, t := range allTransitions {
		if t.To == to && slices.Contains(t.From, from) {
			return true
		}
	}
	return false
}
