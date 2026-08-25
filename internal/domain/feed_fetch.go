package domain

import "time"

// FetchedFeed is a parser-agnostic view of a remote feed.
//
// The scraper works with this type rather than with gofeed's own structs so
// that swapping the parser implementation never reaches past the feedfetch
// package.
type FetchedFeed struct {
	Title       string
	Description string
	Link        string
	Items       []FetchedItem
}

// FetchedItem is a single entry of a FetchedFeed.
type FetchedItem struct {
	Title       string
	Description string
	Link        string
	// PublishedAt is nil when the feed omits a usable date; the scraper
	// decides the fallback rather than silently dropping the item.
	PublishedAt *time.Time
}
