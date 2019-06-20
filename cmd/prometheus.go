package cmd

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
)

// startPromServer starts an HTTP server providing Prometheus metrics
func startPromServer() error {
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(":2112", nil); err != nil {
			middleware.Log.Errorw("Prometheus metrics server failed", "error", err)
		}
	}()
	return nil
}
