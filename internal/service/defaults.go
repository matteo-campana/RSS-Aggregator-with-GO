package service

import (
	"time"

	"github.com/google/uuid"
)

// SystemClock is the production Clock.
//
// It always reports UTC. The previous code mixed time.Now().UTC() with a bare
// time.Now() in the feed-follow path; centralising the clock removes that class
// of inconsistency entirely.
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// UUIDGenerator is the production IDGenerator.
type UUIDGenerator struct{}

// NewID returns a random UUID.
func (UUIDGenerator) NewID() uuid.UUID { return uuid.New() }
