package order

// Transition is a guarded order-status change: an atomic compare-and-set that
// moves an order to To only if its current status is one of From. The From set
// is the operation's guard and may intentionally differ from validTransitions —
// payment relies on race-recovery and idempotent edges the ideal state machine
// does not list — so each operation's allowed-from set is named once here,
// rather than scattered as ad-hoc string lists at call sites (which is what let
// the refund transition drift and miss processing/shipped).
type Transition struct {
	To   Status
	From []Status
}

// Cross-feature status transitions, applied via Service.Apply.
//
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
)
