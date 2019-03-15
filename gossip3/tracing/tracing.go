package tracing

import (
	"github.com/opentracing/opentracing-go"
	"go.elastic.co/apm/module/apmot"
)

func StartElastic() {
	opentracing.SetGlobalTracer(apmot.New())
}
