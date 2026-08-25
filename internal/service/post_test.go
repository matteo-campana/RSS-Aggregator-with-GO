package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/service"
)

func TestPostServiceClampsPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		limit      int32
		offset     int32
		wantLimit  int32
		wantOffset int32
	}{
		{name: "zero limit uses the default", limit: 0, wantLimit: defaultPageSize},
		{name: "negative limit uses the default", limit: -5, wantLimit: defaultPageSize},
		{name: "limit above the maximum is capped", limit: 5000, wantLimit: maxPageSize},
		{name: "limit at the maximum is kept", limit: maxPageSize, wantLimit: maxPageSize},
		{name: "limit within range is kept", limit: 42, wantLimit: 42},
		{name: "negative offset becomes zero", limit: 10, offset: -1, wantLimit: 10, wantOffset: 0},
		{name: "offset is passed through", limit: 10, offset: 30, wantLimit: 10, wantOffset: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakePostRepo{}
			svc := service.NewPostService(repo, defaultPageSize, maxPageSize)

			_, err := svc.ListForUser(context.Background(), uuid.New(), tt.limit, tt.offset)
			require.NoError(t, err)

			assert.Equal(t, tt.wantLimit, repo.gotLimit)
			assert.Equal(t, tt.wantOffset, repo.gotOffset)
		})
	}
}

// Clamping the limit alone left offset unbounded: Postgres still computes the
// whole result set and discards everything before the offset, doing maximum
// work for an empty response, repeatable at will.
func TestDeepOffsetIsRejected(t *testing.T) {
	t.Parallel()

	t.Run("posts", func(t *testing.T) {
		t.Parallel()

		repo := &fakePostRepo{}
		svc := service.NewPostService(repo, defaultPageSize, maxPageSize)

		_, err := svc.ListForUser(context.Background(), uuid.New(), 10, 2147483000)

		assert.ErrorIs(t, err, domain.ErrInvalidInput)
		assert.Zero(t, repo.gotLimit, "must not reach the repository")
	})

	t.Run("feeds", func(t *testing.T) {
		t.Parallel()

		repo := &fakeFeedRepo{}
		svc := service.NewFeedService(repo, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

		_, err := svc.List(context.Background(), 10, 2147483000)

		assert.ErrorIs(t, err, domain.ErrInvalidInput)
		assert.Zero(t, repo.gotLimit, "must not reach the repository")
	})

	t.Run("the boundary itself is accepted", func(t *testing.T) {
		t.Parallel()

		repo := &fakePostRepo{}
		svc := service.NewPostService(repo, defaultPageSize, maxPageSize)

		_, err := svc.ListForUser(context.Background(), uuid.New(), 10, 100_000)

		require.NoError(t, err)
		assert.Equal(t, int32(100_000), repo.gotOffset)
	})
}

func TestPostServicePropagatesError(t *testing.T) {
	t.Parallel()

	svc := service.NewPostService(&fakePostRepo{listErr: errBoom}, 10, 100)

	_, err := svc.ListForUser(context.Background(), uuid.New(), 0, 0)

	assert.ErrorIs(t, err, errBoom)
}
