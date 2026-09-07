package tracing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

const flushTimeout = 5 * time.Second

func Setup(
	ctx context.Context,
	serviceName, env, exporter string,
	log *slog.Logger,
) (func(), error) {
	if exporter == "" || exporter == "none" {
		return func() {}, nil
	}

	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("building span exporter: %w", err)
	}

	res, err := newResource(ctx, serviceName, env)
	if err != nil {
		return nil, fmt.Errorf("building resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(dropRootClientSpans{sdktrace.NewBatchSpanProcessor(spanExporter)}),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.ErrorContext(context.Background(), "trace export failed", slog.String("error", err.Error()))
	}))

	return func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()

		if err := provider.Shutdown(flushCtx); err != nil {
			log.ErrorContext(flushCtx, "flushing traces failed", slog.String("error", err.Error()))
		}
	}, nil
}

func Record(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.RecordError(err)

	if kind := errs.Kind(err); kind != nil {
		span.SetAttributes(attribute.String("error.kind", kind.Error()))
		return
	}

	span.SetStatus(codes.Error, err.Error())
}

func newResource(ctx context.Context, serviceName, env string) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("deployment.environment.name", env),
		),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
	)
}

type dropRootClientSpans struct{ sdktrace.SpanProcessor }

// A root CLIENT span is machine chatter, not work anyone asked for: River polls
// river_job once a second, and each poll would otherwise be its own trace.
func (p dropRootClientSpans) OnEnd(span sdktrace.ReadOnlySpan) {
	if !span.Parent().IsValid() && span.SpanKind() == trace.SpanKindClient {
		return
	}

	p.SpanProcessor.OnEnd(span)
}
