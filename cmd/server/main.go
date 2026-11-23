// Command server runs the handwriting synthesizer.
package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"typeshit/internal/api"
	"typeshit/internal/assets"
	"typeshit/internal/render"
	"typeshit/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	data := flag.String("data", "data", "data directory")
	fontsDir := flag.String("fonts", "", "extra font directory (*.ttf/*.otf + optional catalog.json); default <data>/fonts")
	flag.Parse()

	if *fontsDir == "" {
		*fontsDir = filepath.Join(*data, "fonts")
	}
	st, err := store.Open(*data)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	if n, err := assets.LoadDir(*fontsDir); err != nil {
		log.Printf("fonts: %v", err)
	} else if n > 0 {
		log.Printf("fonts: %d extra font(s) from %s", n, *fontsDir)
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
