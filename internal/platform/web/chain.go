package web

import (
	"net/http"
	"slices"
)

type Middleware func(http.Handler) http.Handler

func Chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for _, v := range slices.Backward(mws) {
			next = v(next)
		}
		return next
	}
}
