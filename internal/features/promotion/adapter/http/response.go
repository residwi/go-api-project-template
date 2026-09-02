package http

type applyResponse struct {
	Code     string `json:"code"`
	Discount int64  `json:"discount"`
}

func toApplyResponse(code string, discount int64) applyResponse {
	return applyResponse{
		Code:     code,
		Discount: discount,
	}
}
