package web

import (
	"net/http"
	"slices"
	"strings"
)

type Router struct {
	mux    *http.ServeMux
	prefix string
	mws    []Middleware
	routes *[]string
}

func NewRouter(mux *http.ServeMux) *Router {
	return &Router{mux: mux, routes: new([]string)}
}

func (r *Router) Group(prefix string, mws ...Middleware) *Router {
	return &Router{
		mux:    r.mux,
		prefix: r.prefix + prefix,
		mws:    join(r.mws, mws),
		routes: r.routes,
	}
}

func (r *Router) Handle(pattern string, handler http.Handler) {
	method, path, _ := strings.Cut(pattern, " ")
	fullPath := r.prefix + path
	if len(r.mws) > 0 {
		handler = Chain(r.mws...)(handler)
	}
	r.mux.Handle(method+" "+fullPath, handler)
	*r.routes = append(*r.routes, method+"\t"+fullPath)
}

func (r *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.Handle(pattern, handler)
}

func (r *Router) Routes() []string {
	return slices.Clone(*r.routes)
}

func join(a, b []Middleware) []Middleware {
	mws := make([]Middleware, 0, len(a)+len(b))
	mws = append(mws, a...)
	return append(mws, b...)
}
