package dashboard

type SummaryResponse struct {
	Sales           SalesSummary      `json:"sales"`
	StatusBreakdown []StatusBreakdown `json:"status_breakdown"`
}
