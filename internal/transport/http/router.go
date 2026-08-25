package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// The interfaces this package consumes. They are declared here, next to the
// code that calls them, so the transport layer never imports the service
// implementations it happens to be wired to.

// UserService registers users and resolves API keys.
type UserService interface {
	Create(ctx context.Context, name string) (domain.User, error)
	Authenticate(ctx context.Context, apiKey string) (domain.User, error)
}

// FeedService registers and lists feeds.
type FeedService interface {
	Create(ctx context.Context, userID uuid.UUID, name, url string) (domain.Feed, error)
	List(ctx context.Context) ([]domain.Feed, error)
}

// FeedFollowService manages the follow relation.
type FeedFollowService interface {
	Create(ctx context.Context, userID, feedID uuid.UUID) (domain.FeedFollow, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.FeedFollow, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

// PostService reads a user's posts.
type PostService interface {
	ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Post, error)
}

// Deps are everything the router needs to serve requests.
type Deps struct {
	Users          UserService
	Feeds          FeedService
	FeedFollows    FeedFollowService
	Posts          PostService
	Logger         *slog.Logger
	AllowedOrigins []string
}

type server struct {
	users   UserService
	feeds   FeedService
	follows FeedFollowService
	posts   PostService
	log     *slog.Logger
}

// NewRouter builds the HTTP handler for the whole API.
func NewRouter(d Deps) http.Handler {
	logger := d.Logger
	if logger == nil {
		logger = discardLogger()
	}

	s := &server{
		users:   d.Users,
		feeds:   d.Feeds,
		follows: d.FeedFollows,
		posts:   d.Posts,
		log:     logger,
	}

	origins := d.AllowedOrigins
	if len(origins) == 0 {
		origins = []string{"https://*", "http://*"}
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// middleware.RealIP is deliberately NOT used: it rewrites r.RemoteAddr from
	// client-controlled headers, which is spoofable unless a trusted proxy
	// sanitises them (GHSA-3fxj-6jh8-hvhx).
	//
	// Recoverer turns a handler panic into a 500 instead of taking the whole
	// process down with it.
	r.Use(middleware.Recoverer)
	r.Use(s.accessLog)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	v1 := chi.NewRouter()

	// Public.
	v1.Get("/healthz", s.handleHealth)
	v1.Post("/users", s.handleCreateUser)
	v1.Get("/feeds", s.handleListFeeds)

	// Authenticated.
	v1.Get("/users", s.requireUser(s.handleGetCurrentUser))
	v1.Post("/feeds", s.requireUser(s.handleCreateFeed))
	v1.Get("/feed_follows", s.requireUser(s.handleListFeedFollows))
	v1.Post("/feed_follows", s.requireUser(s.handleCreateFeedFollow))
	v1.Delete("/feed_follows/{feed_follow_id}", s.requireUser(s.handleDeleteFeedFollow))
	v1.Get("/posts", s.requireUser(s.handleListPosts))

	r.Mount("/v1", v1)

	return r
}

// accessLog records one structured line per request.
func (s *server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration", time.Since(start),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}
