package middleware

import (
	"net/http"
	"strings"
)

type Middleware func(http.Handler) http.Handler

func Chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

type RouteGroup struct {
	mux        *http.ServeMux
	prefix     string
	middleware Middleware
}

func NewRouteGroup(mux *http.ServeMux, prefix string, mws ...Middleware) *RouteGroup {
	var mw Middleware
	if len(mws) > 0 {
		mw = Chain(mws...)
	}
	return &RouteGroup{mux: mux, prefix: prefix, middleware: mw}
}

func (g *RouteGroup) Handle(pattern string, handler http.Handler) {
	method, path, _ := strings.Cut(pattern, " ")
	fullPattern := method + " " + g.prefix + path
	if g.middleware != nil {
		handler = g.middleware(handler)
	}
	g.mux.Handle(fullPattern, handler)
}

func (g *RouteGroup) HandleFunc(pattern string, handler http.HandlerFunc) {
	g.Handle(pattern, handler)
}
