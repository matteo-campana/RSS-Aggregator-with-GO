package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
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
	List(ctx context.Context, limit, offset int32) ([]domain.Feed, error)
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
	// limitBody comes before accessLog so MaxBytesReader still sees the real
	// http.ResponseWriter: it type-asserts on an unexported interface that
	// chi's wrapped writer does not satisfy, and without it an oversized
	// request is drained instead of closing the connection.
	r.Use(limitBody(maxRequestBody))
	r.Use(s.accessLog)
	// Inside accessLog, so a recovered panic is still logged with its status.
	// chi's own Recoverer dumps a plain-text stack to stdout, which corrupts a
	// line-delimited JSON log stream.
	r.Use(s.recoverPanics)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// chi's defaults answer with plain text and, for 405, an empty body, which
	// breaks any client that parses non-2xx bodies as {"error": ...}.
	r.NotFound(s.handleNotFound)
	r.MethodNotAllowed(s.handleMethodNotAllowed)

	v1 := chi.NewRouter()
	v1.NotFound(s.handleNotFound)
	v1.MethodNotAllowed(s.handleMethodNotAllowed)

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

func (s *server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	s.respond(w, http.StatusNotFound, errorResponse{Error: "resource not found"})
}

func (s *server) handleMethodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	s.respond(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
}

// limitBody caps the request body. It is a middleware rather than something
// decodeJSON does, so it runs against the unwrapped ResponseWriter.
func limitBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// recoverPanics turns a handler panic into a 500 and a structured log line.
func (s *server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			// ErrAbortHandler is the documented way to abort without logging.
			if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(recovered)
			}

			s.log.Error("panic in handler",
				"method", r.Method,
				"path", r.URL.Path,
				"panic", fmt.Sprint(recovered),
				"stack", string(debug.Stack()),
				"request_id", middleware.GetReqID(r.Context()),
			)
			s.respond(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		}()

		next.ServeHTTP(w, r)
	})
}

// accessLog records one structured line per request.
func (s *server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Deferred so a request that unwinds is still recorded; logging after
		// ServeHTTP returns meant a panic produced no access-log line at all.
		defer func() {
			s.log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start),
				"request_id", middleware.GetReqID(r.Context()),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}
