package core

import (
	"net/http"
	"strconv"
)

type OffsetPage struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type OffsetPageResult[T any] struct {
	Items      []T              `json:"items"`
	Pagination OffsetPagination `json:"pagination"`
}

type OffsetPagination struct {
	CurrentPage int  `json:"current_page"`
	PageSize    int  `json:"page_size"`
	TotalItems  int  `json:"total_items"`
	TotalPages  int  `json:"total_pages"`
	HasPrevious bool `json:"has_previous"`
	HasNext     bool `json:"has_next"`
}

func ParseOffsetPage(r *http.Request) OffsetPage {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return OffsetPage{Page: page, PageSize: pageSize}
}

func (p OffsetPage) Offset() int {
	return (p.Page - 1) * p.PageSize
}

func (p OffsetPage) Limit() int {
	return p.PageSize
}

func NewOffsetPageResult[T any](items []T, page OffsetPage, total int) OffsetPageResult[T] {
	totalPages := total / page.PageSize
	if total%page.PageSize > 0 {
		totalPages++
	}
	if items == nil {
		items = []T{}
	}
	return OffsetPageResult[T]{
		Items: items,
		Pagination: OffsetPagination{
			CurrentPage: page.Page,
			PageSize:    page.PageSize,
			TotalItems:  total,
			TotalPages:  totalPages,
			HasPrevious: page.Page > 1,
			HasNext:     page.Page < totalPages,
		},
	}
}
