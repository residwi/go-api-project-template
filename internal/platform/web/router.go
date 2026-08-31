package web

import (
	"net/http"
	"strings"
)

type Router struct {
	mux    *http.ServeMux
	prefix string
	mws    []Middleware
}

func NewRouter(mux *http.ServeMux) *Router {
	return &Router{mux: mux}
}

func (r *Router) Group(prefix string, mws ...Middleware) *Router {
	return &Router{
		mux:    r.mux,
		prefix: r.prefix + prefix,
		mws:    join(r.mws, mws),
	}
}

func (r *Router) Handle(pattern string, handler http.Handler) {
	method, path, _ := strings.Cut(pattern, " ")
	if len(r.mws) > 0 {
		handler = Chain(r.mws...)(handler)
	}
	r.mux.Handle(method+" "+r.prefix+path, handler)
}

func (r *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.Handle(pattern, handler)
}

func join(a, b []Middleware) []Middleware {
	mws := make([]Middleware, 0, len(a)+len(b))
	mws = append(mws, a...)
	return append(mws, b...)
}
