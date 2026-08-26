package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"goflix/internal/catalog"
	"goflix/internal/config"
	"goflix/internal/mediaresolver"
	"goflix/internal/server"
)

func main() {
	cfg, configErr := config.Load("config.conf")
	if configErr != nil {
		log.Fatal("Failed to load config.conf: ", configErr)
	}

	resolver, resolverErr := mediaresolver.New(cfg.Resolver)
	if resolverErr != nil {
		log.Fatal("Failed to initialize media source resolver: ", resolverErr)
	}

	client := catalog.NewClient(cfg.TMDBAccessToken, cfg.TMDBAPIKey)
	store := catalog.NewStore(client)

	// Server-side subtitle ladder: VidKing/VidLove ship no embedded
	// renditions, so resolve external subtitles during Resolve() and embed
	// them into the master manifest for native-HLS engines (smart TVs).
	resolver.SubRenditionProvider = func(ctx context.Context, req mediaresolver.MediaRequest) []mediaresolver.SubRendition {
		return server.FetchSubRenditions(ctx, client, req)
	}

	// Graceful shutdown context: cancelled by SIGTERM or SIGINT. Created here
	// so it can also bound the catalog refresh loop goroutine lifetime.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if !client.HasCredentials() {
		log.Println("WARNING: No TMDB credentials in config.conf")
		log.Println("Set TMDB_ACCESS_TOKEN (Bearer token) or TMDB_API_KEY")
	} else {
		if cfg.TMDBAccessToken != "" {
			log.Println("Using TMDB Bearer access token")
		} else {
			log.Println("Using TMDB API key")
		}

		// Initial fetch of all caches, then auto-refresh every 30 minutes.
		// ctx is cancelled by SIGTERM/SIGINT, which stops the ticker goroutine.
		store.StartRefreshLoop(ctx)
	}

	handler := server.New(server.Deps{
		Resolver:  resolver,
		Store:     store,
		Client:    client,
		StartedAt: time.Now(),
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second, // covers streaming proxy responses
		IdleTimeout:       90 * time.Second,
	}

	// Graceful shutdown: wait for signal then cleanly drain active
	// connections and release browser sessions before exiting.

	go func() {
		log.Println("Server listening on :8080 — open http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Error starting server: ", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutdown signal received — draining connections...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	resolver.Close()
	log.Println("Server stopped cleanly.")
}
