// Command server runs the handwriting synthesizer.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"typeshit/internal/api"
	"typeshit/internal/render"
	"typeshit/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	data := flag.String("data", "data", "data directory")
	flag.Parse()

	st, err := store.Open(*data)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	srv := api.New(st, &render.Engine{Store: st})

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on http://%s (data: %s)", *addr, *data)
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
