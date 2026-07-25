package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fmi/internal/core/cache"
	"fmi/internal/core/config"
	"fmi/internal/core/places"
	"fmi/internal/core/server"
	"fmi/internal/havainnot"
	"fmi/internal/salama"
	"fmi/internal/tutka"
)

func main() {
	log.Println("Starting Tutka backend...")

	// 1. Load config
	cfg := config.LoadConfig()

	// 2. Initialize the shared live cache
	var liveCache cache.Cache
	if cfg.NoRedis {
		log.Println("Redis is disabled (--no-redis). Using in-memory cache.")
		liveCache = cache.NewMemoryCache()
	} else {
		log.Printf("Connecting to Redis at %s...\n", cfg.RedisURL)
		var err error
		liveCache, err = cache.NewRedisCache(cfg.RedisURL)
		if err != nil {
			log.Printf("WARNING: Redis connection failed: %v. Falling back to in-memory cache.\n", err)
			liveCache = cache.NewMemoryCache()
		} else {
			log.Println("Connected to Redis successfully.")
		}
	}
	defer liveCache.Close()

	// 3. Context for background tasks lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Wire and start the weather modes. A mode that fails to start is logged
	// and left degraded — one broken FMI feed must not take down the others.
	tutkaService := tutka.NewService(cfg, liveCache)
	if err := tutkaService.Start(ctx); err != nil {
		log.Printf("ERROR starting tutka mode: %v\n", err)
	}
	defer tutkaService.Stop()

	salamaService := salama.NewService(liveCache)
	if err := salamaService.Start(ctx); err != nil {
		log.Printf("ERROR starting salama mode: %v\n", err)
	}
	defer salamaService.Stop()

	havainnotService := havainnot.NewService(liveCache)
	if err := havainnotService.Start(ctx); err != nil {
		log.Printf("ERROR starting havainnot mode: %v\n", err)
	}
	defer havainnotService.Stop()

	// 5. Place search. Not a mode: no poller, no snapshot, nothing to report to
	// health — it answers a search or it does not. It is also the only endpoint
	// a visitor can cause an upstream request from, which is why it caches and
	// paces (see the package doc).
	placesService := places.NewService(liveCache)

	// 6. Assemble the shared router: global endpoints + each mode's routes.
	handlers := server.NewHandlers(liveCache, tutkaService, salamaService, havainnotService)
	router := server.NewRouter(handlers, placesService)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	// 7. Handle OS shutdown signals for graceful termination
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Listening on http://localhost:%s\n", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server listen failed: %v\n", err)
		}
	}()

	sig := <-shutdownChan
	log.Printf("Received signal: %s. Initiating graceful shutdown...\n", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v\n", err)
	}

	log.Println("Tutka backend stopped.")
}
