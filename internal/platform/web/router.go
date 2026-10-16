package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

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

func (r *Router) HandleFunc(pattern string, handler HandlerFunc) {
	r.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tw := &trackingWriter{ResponseWriter: w}
		err := handler(tw, req)
		if err == nil {
			return
		}

		ctx := req.Context()
		if tw.wrote {
			r.log.ErrorContext(ctx, "error after response was written", slog.String("error", err.Error()))
			return
		}
		if errs.Kind(err) == nil {
			r.log.ErrorContext(ctx, "unexpected error", slog.String("error", err.Error()))
		}
		response.HandleErr(w, err)
	}))
}

type trackingWriter struct {
	http.ResponseWriter

	wrote bool
}

func (w *trackingWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *trackingWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

func join(a, b []Middleware) []Middleware {
	mws := make([]Middleware, 0, len(a)+len(b))
	mws = append(mws, a...)
	return append(mws, b...)
}
