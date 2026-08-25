package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// The response shapes below reproduce the existing public JSON contract
// field-for-field. Domain models stay free of transport concerns; these types
// carry the json tags.

type userResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key"`
}

type feedResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	UserID    uuid.UUID `json:"user_id"`
}

type feedFollowResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    uuid.UUID `json:"user_id"`
	FeedID    uuid.UUID `json:"feed_id"`
}

type postResponse struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	PublishedAt time.Time `json:"published_at"`
	URL         string    `json:"url"`
	FeedID      uuid.UUID `json:"feed_id"`
}

func newUserResponse(u domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Name:      u.Name,
		APIKey:    u.APIKey,
	}
}

func newFeedResponse(f domain.Feed) feedResponse {
	return feedResponse{
		ID:        f.ID,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
		Name:      f.Name,
		URL:       f.URL,
		UserID:    f.UserID,
	}
}

func newFeedResponses(feeds []domain.Feed) []feedResponse {
	out := make([]feedResponse, 0, len(feeds))
	for _, f := range feeds {
		out = append(out, newFeedResponse(f))
	}
	return out
}

func newFeedFollowResponse(ff domain.FeedFollow) feedFollowResponse {
	return feedFollowResponse{
		ID:        ff.ID,
		CreatedAt: ff.CreatedAt,
		UpdatedAt: ff.UpdatedAt,
		UserID:    ff.UserID,
		FeedID:    ff.FeedID,
	}
}

func newFeedFollowResponses(follows []domain.FeedFollow) []feedFollowResponse {
	out := make([]feedFollowResponse, 0, len(follows))
	for _, ff := range follows {
		out = append(out, newFeedFollowResponse(ff))
	}
	return out
}

func newPostResponse(p domain.Post) postResponse {
	return postResponse{
		ID:          p.ID,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		Title:       p.Title,
		Description: p.Description,
		PublishedAt: p.PublishedAt,
		URL:         p.URL,
		FeedID:      p.FeedID,
	}
}

func newPostResponses(posts []domain.Post) []postResponse {
	out := make([]postResponse, 0, len(posts))
	for _, p := range posts {
		out = append(out, newPostResponse(p))
	}
	return out
}
