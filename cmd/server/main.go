package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if err := run(); err != nil {
		log.Fatalf("[fatal] %v", err)
	}
}

func run() error {
	root := flag.String("root", ".", "data root: directory containing target.txt and data/definitions/world.json (the process changes into it, like the TypeScript server directory)")
	portFlag := flag.Int("port", 0, "WebSocket/HTTP port (0 = WEBSOCKET_PORT env, then 43594)")
	noCache := flag.Bool("no-cache", false, "skip the OpenRS2 cache download; read the revision from target.txt only (boot and login work, definition-backed features do not)")
	tickMs := flag.Int("tick", 0, "tick rate in milliseconds (0 = 600)")
	saveBackend := flag.String("save-backend", "json", "character-save backend: json, memory, or sqlite")
	flag.Parse()

	if err := os.Chdir(*root); err != nil {
		return fmt.Errorf("changing to data root %s: %w", *root, err)
	}

	srv, err := buildServer(Options{
		NoCache:     *noCache,
		TickRateMs:  *tickMs,
		SaveBackend: *saveBackend,
		Port:        *portFlag,
	})
	if err != nil {
		return err
	}

	port := srv.Port(*portFlag)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: srv.Handler(),
	}
	srv.Start()
	log.Printf("[server] %s started: revision %d, tick %d ms, listening on :%d", srv.WorldName(), srv.Revision(), srv.tickRateMs, port)
	log.Printf("[server] NOTE: movement + collision, NPC drivers, combat, shops, trading, and the interface runtime are wired (walk clicks route and step with real map collision; NPCs wander, aggress, and fight with hitsplat sync; shops open, buy, and sell; players trade; interfaces open, dispatch clicks to content, and close on movement); the only plugin content is the date_minutes varp sender. See PORT_STATUS.md.")

	serveErr := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	// Graceful shutdown mirrors Server.ts signal handling (persist +
	// exit): Server.Stop saves online players (reason "shutdown",
	// snapshotted on the tick goroutine), then flushes accounts.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	select {
	case err := <-serveErr:
		return fmt.Errorf("serving: %w", err)
	case got := <-sig:
		log.Printf("[shutdown] caught %s, shutting down", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("[shutdown] HTTP shutdown: %v", err)
	}
	srv.Stop()
	log.Printf("[shutdown] done")
	return nil
}
