package web

import (
	"context"
	"errors"
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

func (r *Router) HandleFunc(pattern string, handler HandlerFunc) {
	r.handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tw := &trackingWriter{ResponseWriter: w}
		err := handler(tw, req)
		if err == nil {
			return
		}

		ctx := req.Context()
		switch {
		case errors.Is(err, context.Canceled) && ctx.Err() != nil:
			r.log.DebugContext(ctx, "request cancelled", slog.String("error", err.Error()))
		case tw.wrote:
			r.log.ErrorContext(ctx, "error after response was written", slog.String("error", err.Error()))
		case errs.Kind(err) == nil:
			r.log.ErrorContext(ctx, "unexpected error", slog.String("error", err.Error()))
		}
		if !tw.wrote {
			response.HandleErr(w, err)
		}
	}))
}

func (r *Router) handle(pattern string, handler http.Handler) {
	method, path, _ := strings.Cut(pattern, " ")
	if len(r.mws) > 0 {
		handler = Chain(r.mws...)(handler)
	}
	r.mux.Handle(method+" "+r.prefix+path, handler)
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
