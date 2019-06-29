package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
)

var preflightGauge prometheus.Gauge
var staleTxnsCounter prometheus.Counter
var inactiveConflictSetsGauge prometheus.Gauge
var numConflictSetsGauge prometheus.Gauge
var committedTransactionsCounter prometheus.Counter
var chainTreesGauge prometheus.Gauge

func init() {
	preflightGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "quorumcontrol",
		Subsystem: "tupelo",
		Name:      "txns_preflight",
		Help:      "Number of transactions in preflight.",
	})
	if err := prometheus.Register(preflightGauge); err != nil {
		middleware.Log.Errorw("failed to register preflight gauge")
	}

	staleTxnsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "quorumcontrol",
		Subsystem: "tupelo",
		Name:      "txns_stale",
		Help:      "Number of stale transactions received.",
	})
	if err := prometheus.Register(staleTxnsCounter); err != nil {
		middleware.Log.Errorw("failed to register stale transactions counter")
	}

	inactiveConflictSetsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "quorumcontrol",
		Subsystem: "tupelo",
		Name:      "inactive_conflictsets",
		Help:      "Number of inactive conflict sets.",
	})
	if err := prometheus.Register(inactiveConflictSetsGauge); err != nil {
		middleware.Log.Errorw("failed to register inactive conflict sets gauge")
	}

	numConflictSetsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "quorumcontrol",
		Subsystem: "tupelo",
		Name:      "num_conflictsets",
		Help:      "Current number of conflict sets.",
	})
	if err := prometheus.Register(numConflictSetsGauge); err != nil {
		middleware.Log.Errorw("failed to register number of conflict sets gauge")
	}

	committedTransactionsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "quorumcontrol",
		Subsystem: "tupelo",
		Name:      "total_committed_transactions",
		Help:      "Total number of committed transactions.",
	})
	if err := prometheus.Register(committedTransactionsCounter); err != nil {
		middleware.Log.Errorw("failed to register committed transactions counter")
	}

	chainTreesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "quorumcontrol",
		Subsystem: "tupelo",
		Name:      "total_chain_trees",
		Help:      "Total number of chain trees.",
	})
	if err := prometheus.Register(chainTreesGauge); err != nil {
		middleware.Log.Errorw("failed to register total number of chain trees gauge")
	}
}

// IncPreflightTxns increases the gauge of preflight transactions.
func IncPreflightTxns() {
	preflightGauge.Inc()
}

// DecPreflightTxns decreases the gauge of preflight transactions.
func DecPreflightTxns() {
	preflightGauge.Dec()
}

// IncStaleTxns increases the count of stale transactions received.
func IncStaleTxns() {
	staleTxnsCounter.Inc()
}

// IncInactiveConflictSets increases the gauge of inactive conflict sets.
func IncInactiveConflictSets() {
	inactiveConflictSetsGauge.Inc()
}

// DecInactiveConflictSets decreases the gauge of inactive conflict sets.
func DecInactiveConflictSets() {
	inactiveConflictSetsGauge.Dec()
}

func SetNumConflictSets(num int64) {
	numConflictSetsGauge.Set(float64(num))
}

// IncTotalCommittedTransactions increases the counter of committed transactions.
func IncTotalCommittedTransactions() {
	committedTransactionsCounter.Inc()
}

// IncNumChainTrees increases the gauge of total number of chain trees.
func IncNumChainTrees() {
	chainTreesGauge.Inc()
}

// DecNumChainTrees decreases the gauge of total number of chain trees.
func DecNumChainTrees() {
	chainTreesGauge.Dec()
}
