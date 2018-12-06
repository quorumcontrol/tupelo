package gossip2

import (
	"bytes"
	"context"
	"io"

	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-lib/metrics"
)

var GlobalCloser io.Closer

func newSpan(ctx context.Context, tracer opentracing.Tracer, operationName string, opts ...opentracing.StartSpanOption) (opentracing.Span, context.Context) {
	var span opentracing.Span
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		opts = append(opts, opentracing.ChildOf(parentSpan.Context()))
	}
	span = tracer.StartSpan(operationName, opts...)
	return span, opentracing.ContextWithSpan(ctx, span)
}

func spanToTextMap(tracer opentracing.Tracer, span opentracing.Span) (opentracing.TextMapCarrier, error) {
	carrier := make(opentracing.TextMapCarrier)
	err := tracer.Inject(span.Context(), opentracing.TextMap, carrier)
	return carrier, err
}

func textMapToSpanContext(tracer opentracing.Tracer, carrier opentracing.TextMapCarrier) (opentracing.SpanContext, error) {
	return tracer.Extract(opentracing.TextMap, carrier)
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

var trlog = logging.Logger("tracing")

type traceLogger struct{}

// Error logs a message at error priority
func (tl *traceLogger) Error(msg string) {
	trlog.Errorf(msg)
}

// Infof logs a message at info priority
func (tl *traceLogger) Infof(msg string, args ...interface{}) {
	trlog.Infof(msg, args...)
}

func InitializeForTesting() {
	// Sample configuration for testing. Use constant sampling to sample every trace
	// and enable LogSpan to log every span via configured Logger.
	cfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeRateLimiting,
			Param: 10,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans: false,
		},
	}

	// Example logger and metrics factory. Use github.com/uber/jaeger-client-go/log
	// and github.com/uber/jaeger-lib/metrics respectively to bind to real logging and metrics
	// frameworks.
	jMetricsFactory := metrics.NullFactory

	// Initialize tracer with a logger and a metrics factory
	closer, err := cfg.InitGlobalTracer(
		"Gossiper",
		jaegercfg.Logger(new(traceLogger)),
		jaegercfg.Metrics(jMetricsFactory),
	)
	if err != nil {
		log.Errorf("Could not initialize jaeger tracer: %s", err.Error())
		return
	}
	GlobalCloser = closer
}
