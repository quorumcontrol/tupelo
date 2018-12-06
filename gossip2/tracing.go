package gossip2

import (
	"bytes"
	"context"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-lib/metrics"
)

func newSpan(ctx context.Context, tracer opentracing.Tracer, operationName string, opts ...opentracing.StartSpanOption) (opentracing.Span, context.Context) {
	var span opentracing.Span
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		opts = append(opts, opentracing.ChildOf(parentSpan.Context()))
	}
	span = tracer.StartSpan(operationName, opts...)
	return span, opentracing.ContextWithSpan(ctx, span)
}

func spanToBinary(tracer opentracing.Tracer, span opentracing.Span) ([]byte, error) {
	var buf []byte
	carrier := bytes.NewBuffer(buf)
	err := tracer.Inject(span.Context(), opentracing.Binary, carrier)
	return buf, err
}

func binaryToSpanContext(tracer opentracing.Tracer, bits []byte) (opentracing.SpanContext, error) {
	carrier := bytes.NewBuffer(bits)
	return tracer.Extract(opentracing.Binary, carrier)
}

func InitializeForTesting() {
	// Sample configuration for testing. Use constant sampling to sample every trace
	// and enable LogSpan to log every span via configured Logger.
	cfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans: true,
		},
	}

	// Example logger and metrics factory. Use github.com/uber/jaeger-client-go/log
	// and github.com/uber/jaeger-lib/metrics respectively to bind to real logging and metrics
	// frameworks.
	jLogger := jaegerlog.StdLogger
	jMetricsFactory := metrics.NullFactory

	// Initialize tracer with a logger and a metrics factory
	_, err := cfg.InitGlobalTracer(
		"Gossiper",
		jaegercfg.Logger(jLogger),
		jaegercfg.Metrics(jMetricsFactory),
	)
	if err != nil {
		log.Errorf("Could not initialize jaeger tracer: %s", err.Error())
		return
	}
}
