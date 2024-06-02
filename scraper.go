package main

import (
	"context"
	"database/sql"
	"github/matteo-campana/rss-aggregator/internal/database"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

func startScraping(
	db *database.Queries,
	concurrency int,
	timeBetweenRequest time.Duration,
) {
	log.Printf("Starting scraping with %d workers every %s duration", concurrency, timeBetweenRequest)
	ticker := time.NewTicker(timeBetweenRequest)
	for ; ; <-ticker.C {
		feeds, err := db.GetNextFeedsToFetch(
			context.Background(),
			int32(concurrency),
		)

		if err != nil {
			log.Println("error fetching feeds: ", err)
			continue
		}

		wg := &sync.WaitGroup{}
		for _, feed := range feeds {
			wg.Add(1)

			go scarpeFeed(db, wg, feed)
		}
		wg.Wait()

	}

}

func scarpeFeed(db *database.Queries, wg *sync.WaitGroup, feed database.Feed) {
	defer wg.Done()

	_, err := db.MarkFeedAsFetched(context.Background(), feed.ID)

	if err != nil {
		log.Printf("error marking feed as fetched: %s", err)
		return
	}

	rssFeed, err := urlToFeed(feed.Url)
	if err != nil {
		log.Printf("error fetching feed: %s", err)
		return
	}

	for _, item := range rssFeed.Channel.Items {
		title := sql.NullString{}
		if item.Title != "" {
			title = sql.NullString{String: item.Title, Valid: true}
		}

		description := sql.NullString{}
		if item.Description != "" {
			description = sql.NullString{String: item.Description, Valid: true}
		}

		published_at, err := time.Parse(time.RFC1123, item.PubDate)

		if err != nil {
			log.Printf("error parsing date %s: %s", item.PubDate, err)
			continue
		}

		_, err = db.CreatePost(context.Background(), database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			Title:       title,
			Description: description,
			PublishedAt: published_at,
			Url:         item.Link,
			FeedID:      feed.ID,
		})
		if err != nil {
			if strings.Contains(err.Error(), "chiave duplicato") {
				continue
			}

			log.Printf("error creating post: %s", err)
			continue
		}
	}

	log.Printf("Fetched %d items from %s, Feed %v collected", len(rssFeed.Channel.Items), feed.Url, feed.Name)
}
