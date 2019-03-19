package tracing

import (
	"context"
	"fmt"

	"github.com/opentracing/opentracing-go"
)

type contextSpanKey struct{}

var parentSpanKey = contextSpanKey{}

// ContextHolder is a struct that you can include in your actor structs
// in order to make them easily traceable
type ContextHolder struct {
	context context.Context
}

// Contextable defines an interface for getting and setting a context
// ContentHolder implements the interface.
type Contextable interface {
	SetContext(ctx context.Context)
	GetContext() context.Context
}

// Traceable defines the interface necessary to make an
// actor struct traceable. ContextHolder implements this interface.
type Traceable interface {
	StartTrace() opentracing.Span
	StopTrace()
	NewSpan(name string) opentracing.Span
	LogKV(key string, value interface{})
	SerializedContext() (map[string]string, error)
	RehydrateSerialized(serialized map[string]string, childName string) (opentracing.Span, error)
}

// StartTrace starts the parent trace of a transactionwrapper
func (ch *ContextHolder) StartTrace(name string) opentracing.Span {
	parent, ctx := opentracing.StartSpanFromContext(context.Background(), name)
	ctx = context.WithValue(ctx, parentSpanKey, parent)
	ch.context = ctx
	return parent
}

// StopTrace stops the parent trace of a transactionwrapper
func (ch *ContextHolder) StopTrace() {
	val := ch.context.Value(parentSpanKey)
	val.(opentracing.Span).Finish()
}

// NewSpan returns a new span as a child of whatever span is
// already in the context.
func (ch *ContextHolder) NewSpan(name string) opentracing.Span {
	sp, ctx := opentracing.StartSpanFromContext(ch.context, name)
	ch.context = ctx
	return sp
}

// LogKV logs a key/value pair to the current span
func (ch *ContextHolder) LogKV(key string, value interface{}) {
	sp := opentracing.SpanFromContext(ch.context)
	sp.LogKV(key, value)
}

// SetContext overrides the current context of the ContextHolder
func (ch *ContextHolder) SetContext(ctx context.Context) {
	parent := opentracing.SpanFromContext(ctx)
	if parent != nil {
		ctx = context.WithValue(ctx, parentSpanKey, parent)
	}
	ch.context = ctx
}

// GetContext returns the current context
func (ch *ContextHolder) GetContext() context.Context {
	return ch.context
}

// SerializedContext returns a text map of the current span context
func (ch *ContextHolder) SerializedContext() (map[string]string, error) {
	serializedContext := make(map[string]string)
	sp := opentracing.SpanFromContext(ch.context)
	err := opentracing.GlobalTracer().Inject(sp.Context(), opentracing.TextMap, opentracing.TextMapCarrier(serializedContext))
	if err != nil {
		return nil, fmt.Errorf("error injecting: %v", err)
	}
	return serializedContext, nil
}

// RehydrateSerialized takes the output of SerializedContext and starts a new span with the childName and sets up
// the context to the correct value.
// WARNING : this will overwrite any context that has previously been set (this is usually the first thing to be called)
func (ch *ContextHolder) RehydrateSerialized(serialized map[string]string, childName string) (opentracing.Span, error) {
	ctx := context.Background()
	sp, err := SpanContextFromSerialized(serialized, childName)
	if err != nil {
		return nil, fmt.Errorf("error deserializing: %v", err)
	}
	ctx = opentracing.ContextWithSpan(ctx, sp)

	ch.SetContext(ctx)
	return sp, nil
}

// SpanContextFromSerialized takes the output of SerializedContext and starts a new span with the childName
func SpanContextFromSerialized(serialized map[string]string, childName string) (opentracing.Span, error) {
	spanContext, err := opentracing.GlobalTracer().Extract(opentracing.TextMap, opentracing.TextMapCarrier(serialized))
	var sp opentracing.Span
	if err != nil {
		return nil, fmt.Errorf("error rehydrating: %v", err)
	}

	sp = opentracing.StartSpan(childName, opentracing.ChildOf(spanContext))
	return sp, nil
}
