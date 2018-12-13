package gossip2

import (
	"bytes"
	"context"

	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	opentracing "github.com/opentracing/opentracing-go"
	"go.elastic.co/apm/module/apmot"
)

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

func InitializeForTesting(serviceName string) {
	opentracing.SetGlobalTracer(apmot.New())
}
