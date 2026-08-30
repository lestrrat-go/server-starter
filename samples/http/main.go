package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	starter "github.com/lestrrat-go/server-starter/v2"
)

func main() {
	if !starter.IsUnderStartServer() {
		log.Fatal("http sample must be started by start_server")
	}

	listeners, err := starter.ListenAll()
	if err != nil {
		log.Fatalf("create inherited listeners: %s", err)
	}

	server := &http.Server{Handler: http.HandlerFunc(handle)}
	var wg sync.WaitGroup
	for _, listener := range listeners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("serve %s: %s", listener.Addr(), err)
			}
		}()
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	<-signals

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shut down HTTP server: %s", err)
	}
	wg.Wait()
}

func handle(w http.ResponseWriter, r *http.Request) {
	generation, ok := starter.Generation()
	if !ok {
		generation = 0
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "hello from start_server generation %d\\n", generation)
}
