package web

import (
	"log/slog"
	"net/http"
	"strings"
)

type Router struct {
	mux    *http.ServeMux
	log    *slog.Logger
	prefix string
	mws    []Middleware
}

func NewRouter(mux *http.ServeMux, log *slog.Logger) *Router {
	return &Router{mux: mux, log: log}
}

func (r *Router) Group(prefix string, mws ...Middleware) *Router {
	return &Router{
		mux:    r.mux,
		log:    r.log,
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
