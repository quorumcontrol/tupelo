package main

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"

	"github.com/gorilla/mux"

	"github.com/quorumcontrol/tupelo/signer/cmd"
)

func main() {
	if os.Getenv("TUPELO_PPROF_ENABLED") == "true" {
		go func() {
			debugR := mux.NewRouter()
			debugR.HandleFunc("/debug/pprof/", pprof.Index)
			debugR.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			debugR.HandleFunc("/debug/pprof/profile", pprof.Profile)
			debugR.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			debugR.HandleFunc("/debug/pprof/trace", pprof.Trace)
			debugR.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
			debugR.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
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

	cmd.Execute()
}
