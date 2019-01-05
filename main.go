package main

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"

	"github.com/gorilla/mux"
	"github.com/quorumcontrol/tupelo/cmd"
	"github.com/quorumcontrol/tupelo/gossip2"
)

func main() {
	if os.Getenv("TUPELO_PPROF_ENABLED") == "true" {
		go func() {
			debugR := mux.NewRouter()
			debugR.HandleFunc("/debug/pprof/", pprof.Index)
			debugR.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			debugR.HandleFunc("/debug/pprof/profile", pprof.Profile)
			debugR.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			debugR.Handle("/debug/pprof/heap", pprof.Handler("heap"))
			debugR.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
			debugR.Handle("/debug/pprof/block", pprof.Handler("block"))
			debugR.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
			err := http.ListenAndServe(":8080", debugR)
			if err != nil {
				fmt.Println(err.Error())
			}
		}()
	}
	if os.Getenv("TUPELO_TRACING_ENABLED") == "true" {
		gossip2.InitializeForTesting("tupelo")
	}

	cmd.Execute()
}
