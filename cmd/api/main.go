// Command api runs the RSS aggregator HTTP service and its background scraper.
//
// This file is the composition root: it is the only place that names concrete
// implementations. Everything it wires together depends on interfaces.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/matteo-campana/rss-aggregator/internal/apikey"
	"github.com/matteo-campana/rss-aggregator/internal/config"
	"github.com/matteo-campana/rss-aggregator/internal/feedfetch"
	"github.com/matteo-campana/rss-aggregator/internal/scraper"
	"github.com/matteo-campana/rss-aggregator/internal/service"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres/sqlc"
	httptransport "github.com/matteo-campana/rss-aggregator/internal/transport/http"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// A missing .env is not an error: in production the environment is set by
	// the platform.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	// Cancelled on SIGINT/SIGTERM; every component watches it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL, postgres.PoolOptions{
		MaxConns:        cfg.DBMaxConns,
		MinConns:        cfg.DBMinConns,
		MaxConnLifetime: cfg.DBMaxConnLifetime,
	})
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("connected to postgres")

	// Adapters.
	queries := sqlc.New(pool)
	userRepo := postgres.NewUserRepository(queries)
	feedRepo := postgres.NewFeedRepository(queries)
	followRepo := postgres.NewFeedFollowRepository(queries)
	postRepo := postgres.NewPostRepository(queries)

	clock := service.SystemClock{}
	ids := service.UUIDGenerator{}
	keys := apikey.Generator{}

	// Application services.
	userSvc := service.NewUserService(userRepo, keys, clock, ids)
	feedSvc := service.NewFeedService(feedRepo, clock, ids, cfg.DefaultPageSize, cfg.MaxPageSize)
	followSvc := service.NewFeedFollowService(followRepo, clock, ids, cfg.DefaultPageSize, cfg.MaxPageSize)
	postSvc := service.NewPostService(postRepo, cfg.DefaultPageSize, cfg.MaxPageSize)

	// The fetcher owns its HTTP client so every outbound request goes through
	// the dial guard that keeps user-supplied feed URLs off the internal network.
	fetcher := feedfetch.New(feedfetch.Options{
		UserAgent:             cfg.ScraperUserAgent,
		Timeout:               cfg.ScraperHTTPTimeout,
		AllowPrivateAddresses: cfg.ScraperAllowPrivateAddresses,
	})
	if cfg.ScraperAllowPrivateAddresses {
		logger.Warn("scraper may reach private addresses; do not enable this in production")
	}
	scr := scraper.New(feedRepo, postRepo, fetcher, clock, ids, logger, scraper.Options{
		Concurrency:    cfg.ScraperConcurrency,
		Interval:       cfg.ScraperInterval,
		RequestTimeout: cfg.ScraperRequestTimeout,
	})

	router := httptransport.NewRouter(httptransport.Deps{
		Users:          userSvc,
		Feeds:          feedSvc,
		FeedFollows:    followSvc,
		Posts:          postSvc,
		Logger:         logger,
		AllowedOrigins: cfg.AllowedOrigins,
	})

	srv := &http.Server{
		Addr:    net.JoinHostPort("", cfg.Port),
		Handler: router,
		// Without these a slow client can hold a connection open indefinitely.
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	failures := make(chan error, 2)

	go func() {
		logger.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failures <- fmt.Errorf("http server: %w", err)
			return
		}
		failures <- nil
	}()

	scraperDone := make(chan struct{})
	go func() {
		defer close(scraperDone)

		if err := scr.Run(ctx); err != nil {
			failures <- fmt.Errorf("scraper: %w", err)
			return
		}
		// Run returning without an error before shutdown was requested means
		// the scraper stopped on its own. Reporting it is what stops the
		// process serving HTTP with a dead scraper and nothing observing it.
		if ctx.Err() == nil {
			failures <- errors.New("scraper stopped unexpectedly")
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case runErr = <-failures:
		// Stop the remaining components too.
		stop()
	}

	return shutdown(srv, scraperDone, cfg.ShutdownTimeout, logger, runErr)
}

// shutdown drains the HTTP server and waits for the scraper to finish.
func shutdown(
	srv *http.Server,
	scraperDone <-chan struct{},
	timeout time.Duration,
	logger *slog.Logger,
	runErr error,
) error {
	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), timeout)
	defer cancelHTTP()

	if err := srv.Shutdown(httpCtx); err != nil {
		logger.Error("http server shutdown", "error", err)
		if runErr == nil {
			runErr = fmt.Errorf("http server shutdown: %w", err)
		}
	}

	// A fresh deadline for the second wait. Draining connections can legitimately
	// consume the whole first one, and reusing it would declare the scraper
	// stuck without ever waiting for it.
	scraperCtx, cancelScraper := context.WithTimeout(context.Background(), timeout)
	defer cancelScraper()

	// Check for completion first: when both channels are ready select picks at
	// random, which produced a spurious warning about half the time.
	select {
	case <-scraperDone:
	default:
		select {
		case <-scraperDone:
		case <-scraperCtx.Done():
			logger.Warn("scraper did not stop before the shutdown deadline")
		}
	}

	logger.Info("shutdown complete")
	return runErr
}
