package postgres

import (
	"time"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres/sqlc"
)

// The generated structs are pinned by sqlc.yaml overrides to plain Go types
// (uuid.UUID, time.Time, *time.Time, *string), so these conversions stay
// mechanical. This file is the single place that would absorb a change in what
// sqlc emits.

// utc normalises a timestamp read back from a TIMESTAMP column. Values are
// always written in UTC; this guards the read path against a driver returning
// a different location.
func utc(t time.Time) time.Time { return t.UTC() }

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

func toDomainUser(u sqlc.User) domain.User {
	return domain.User{
		ID:        u.ID,
		CreatedAt: utc(u.CreatedAt),
		UpdatedAt: utc(u.UpdatedAt),
		Name:      u.Name,
		APIKey:    u.ApiKey,
	}
}

func toDomainFeed(f sqlc.Feed) domain.Feed {
	return domain.Feed{
		ID:            f.ID,
		CreatedAt:     utc(f.CreatedAt),
		UpdatedAt:     utc(f.UpdatedAt),
		Name:          f.Name,
		URL:           f.Url,
		UserID:        f.UserID,
		LastFetchedAt: utcPtr(f.LastFetchedAt),
	}
}

func toDomainFeeds(rows []sqlc.Feed) []domain.Feed {
	feeds := make([]domain.Feed, 0, len(rows))
	for _, r := range rows {
		feeds = append(feeds, toDomainFeed(r))
	}
	return feeds
}

func toDomainFeedFollow(ff sqlc.FeedFollow) domain.FeedFollow {
	return domain.FeedFollow{
		ID:        ff.ID,
		CreatedAt: utc(ff.CreatedAt),
		UpdatedAt: utc(ff.UpdatedAt),
		UserID:    ff.UserID,
		FeedID:    ff.FeedID,
	}
}

func toDomainFeedFollows(rows []sqlc.FeedFollow) []domain.FeedFollow {
	follows := make([]domain.FeedFollow, 0, len(rows))
	for _, r := range rows {
		follows = append(follows, toDomainFeedFollow(r))
	}
	return follows
}

func toDomainPost(p sqlc.Post) domain.Post {
	return domain.Post{
		ID:          p.ID,
		CreatedAt:   utc(p.CreatedAt),
		UpdatedAt:   utc(p.UpdatedAt),
		Title:       p.Title,
		Description: p.Description,
		PublishedAt: utc(p.PublishedAt),
		URL:         p.Url,
		FeedID:      p.FeedID,
	}
}

func toDomainPosts(rows []sqlc.Post) []domain.Post {
	posts := make([]domain.Post, 0, len(rows))
	for _, r := range rows {
		posts = append(posts, toDomainPost(r))
	}
	return posts
}
