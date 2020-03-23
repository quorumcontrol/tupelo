package apm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/metrics/result"
	apmlib "go.elastic.co/apm"
)

type byTypeAggregator struct {
	Type        string
	TotalBlocks uint64
	DeltaBlocks uint64
	Count       uint64
}

type Recorder struct {
	byType          map[string]*byTypeAggregator
	txn             *apmlib.Transaction
	round           uint64
	meausuredRounds uint64
	tracer          *apmlib.Tracer
}

func New() *Recorder {
	os.Setenv("ELASTIC_APM_API_BUFFER_SIZE", "100mb")
	os.Setenv("ELASTIC_APM_SPAN_FRAMES_MIN_DURATION", "3600m")
	os.Setenv("ELASTIC_APM_STACK_TRACE_LIMIT", "0")
	os.Setenv("ELASTIC_APM_TRANSACTION_SAMPLE_RATE", "1.0")
	os.Setenv("ELASTIC_APM_METRICS_INTERVAL", "0s")

	tracer, err := apmlib.NewTracer("tupelo-metrics", "v1")
	if err != nil {
		panic(err)
	}

	return &Recorder{
		tracer: tracer,
		byType: make(map[string]*byTypeAggregator),
	}
}

func (r *Recorder) Start(ctx context.Context, startRound *types.RoundWrapper, endRound *types.RoundWrapper) error {
	r.round = endRound.Height()
	r.meausuredRounds = r.round - startRound.Height()

	r.txn = r.tracer.StartTransaction(fmt.Sprintf("round%d", r.round), "roundmetrics")
	r.txn.Context.SetLabel("round", fmt.Sprintf("%d", r.round))
	r.txn.Context.SetLabel("meausureRounds", fmt.Sprintf("%d", r.meausuredRounds))

	return nil
}

func (r *Recorder) Record(ctx context.Context, result *result.Result) error {
	defer r.tracer.Flush(make(chan struct{}))

	if _, ok := r.byType[result.Type]; !ok {
		r.byType[result.Type] = &byTypeAggregator{Type: result.Type}
	}

	r.byType[result.Type].DeltaBlocks += result.DeltaBlocks
	r.byType[result.Type].TotalBlocks += result.TotalBlocks
	r.byType[result.Type].Count++

	txn := r.tracer.StartTransaction(strings.ReplaceAll(result.Did, ":", "_"), "treemetrics")
	defer txn.End()

	txn.Context.SetLabel("round", fmt.Sprintf("%d", result.Round))
	txn.Context.SetLabel("measuredblocks", fmt.Sprintf("%d", result.DeltaBlocks))
	txn.Context.SetLabel("totalblocks", fmt.Sprintf("%d", result.TotalBlocks))
	txn.Context.SetLabel("type", result.Type)
	txn.Context.SetLabel("tags", result.Tags)

	return nil
}

func (r *Recorder) Finish(ctx context.Context) error {
	var totalBlocks uint64
	var measuredBlocks uint64
	var count uint64

	for t, result := range r.byType {
		totalBlocks += result.TotalBlocks
		measuredBlocks += result.DeltaBlocks
		count += result.Count

		txn := r.txn.StartSpanOptions(fmt.Sprintf("round%d/%s", r.round, t), "treemetrics", apmlib.SpanOptions{Parent: r.txn.TraceContext()})
		txn.Context.SetLabel("round", fmt.Sprintf("%d", r.round))
		txn.Context.SetLabel("totalblocks", fmt.Sprintf("%d", result.TotalBlocks))
		txn.Context.SetLabel("measuredblocks", fmt.Sprintf("%d", result.DeltaBlocks))
		txn.Context.SetLabel("count", fmt.Sprintf("%d", result.Count))
		txn.Context.SetLabel("type", result.Type)
		txn.End()
	}

	r.txn.Context.SetLabel("round", fmt.Sprintf("%d", r.round))
	r.txn.Context.SetLabel("totalblocks", fmt.Sprintf("%d", totalBlocks))
	r.txn.Context.SetLabel("measuredblocks", fmt.Sprintf("%d", measuredBlocks))
	r.txn.Context.SetLabel("count", fmt.Sprintf("%d", count))
	r.txn.End()

	r.tracer.Flush(make(chan struct{}))

	return nil
}
