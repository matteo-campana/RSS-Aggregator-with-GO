package http_test

import (
	nethttp "net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

func TestHealthz(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	resp := h.do(t, nethttp.MethodGet, "/v1/healthz", "", "")

	assert.Equal(t, nethttp.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestCreateUser(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.users.user = domain.User{
		ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		CreatedAt: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
		Name:      "Ada",
		APIKey:    "secret",
	}

	resp := h.do(t, nethttp.MethodPost, "/v1/users", `{"name":"Ada"}`, "")

	require.Equal(t, nethttp.StatusCreated, resp.StatusCode)
	assert.Equal(t, "Ada", h.users.gotName)

	body := decode[map[string]any](t, resp)
	// The public JSON contract is unchanged.
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", body["id"])
	assert.Equal(t, "Ada", body["name"])
	assert.Equal(t, "secret", body["api_key"])
	assert.Contains(t, body, "created_at")
	assert.Contains(t, body, "updated_at")
}

func TestCreateUserRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	resp := h.do(t, nethttp.MethodPost, "/v1/users", `{"name":`, "")

	assert.Equal(t, nethttp.StatusBadRequest, resp.StatusCode)
}

func TestCreateUserMapsValidationError(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.users.createErr = domain.NewValidationError("name", "must not be empty")

	resp := h.do(t, nethttp.MethodPost, "/v1/users", `{"name":""}`, "")

	require.Equal(t, nethttp.StatusBadRequest, resp.StatusCode)
	body := decode[map[string]any](t, resp)
	assert.Contains(t, body["error"], "name")
}

// An unexpected internal error must not leak its text to the client.
func TestInternalErrorIsNotLeaked(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.users.createErr = errBoom

	resp := h.do(t, nethttp.MethodPost, "/v1/users", `{"name":"Ada"}`, "")

	require.Equal(t, nethttp.StatusInternalServerError, resp.StatusCode)
	raw := bodyString(t, resp)
	assert.NotContains(t, raw, "boom")
	assert.Contains(t, raw, "internal server error")
}

func TestListFeedsIsPublicAndReturnsArray(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	resp := h.do(t, nethttp.MethodGet, "/v1/feeds", "", "")

	require.Equal(t, nethttp.StatusOK, resp.StatusCode)
	// An empty result must marshal as [] rather than null.
	assert.Equal(t, "[]", bodyString(t, resp))
}

func TestListFeedsPassesPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantLimit  int32
		wantOffset int32
	}{
		{name: "no parameters", query: "", wantStatus: nethttp.StatusOK},
		{name: "limit and offset", query: "?limit=25&offset=50", wantStatus: nethttp.StatusOK, wantLimit: 25, wantOffset: 50},
		{name: "non numeric limit", query: "?limit=all", wantStatus: nethttp.StatusBadRequest},
		{name: "negative limit", query: "?limit=-1", wantStatus: nethttp.StatusBadRequest},
		{name: "negative offset", query: "?offset=-1", wantStatus: nethttp.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			// This route is public: no API key.
			resp := h.do(t, nethttp.MethodGet, "/v1/feeds"+tt.query, "", "")

			require.Equal(t, tt.wantStatus, resp.StatusCode)
			if tt.wantStatus != nethttp.StatusOK {
				return
			}
			assert.Equal(t, tt.wantLimit, h.feeds.gotLimit)
			assert.Equal(t, tt.wantOffset, h.feeds.gotOffset)
		})
	}
}

func TestCreateFeed(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	h := newHarness(t)
	h.users.user = domain.User{ID: owner}
	h.feeds.feed = domain.Feed{ID: uuid.New(), Name: "Go Blog", URL: "https://go.dev/blog/feed.atom", UserID: owner}

	resp := h.do(t, nethttp.MethodPost, "/v1/feeds",
		`{"name":"Go Blog","url":"https://go.dev/blog/feed.atom"}`, "key")

	require.Equal(t, nethttp.StatusCreated, resp.StatusCode)
	assert.Equal(t, owner, h.feeds.gotUserID, "the feed must be owned by the authenticated user")
	assert.Equal(t, "Go Blog", h.feeds.gotName)
	assert.Equal(t, "https://go.dev/blog/feed.atom", h.feeds.gotURL)
}

func TestCreateFeedMapsConflict(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.feeds.createErr = domain.ErrConflict

	resp := h.do(t, nethttp.MethodPost, "/v1/feeds", `{"name":"n","url":"https://e.com/f"}`, "key")

	assert.Equal(t, nethttp.StatusConflict, resp.StatusCode)
}

// The old handler answered 201 Created for this read-only endpoint.
func TestListFeedFollowsReturns200(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	resp := h.do(t, nethttp.MethodGet, "/v1/feed_follows", "", "key")

	assert.Equal(t, nethttp.StatusOK, resp.StatusCode)
}

func TestCreateFeedFollowMapsUnknownFeedTo404(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.follows.createErr = domain.ErrNotFound

	resp := h.do(t, nethttp.MethodPost, "/v1/feed_follows",
		`{"feed_id":"22222222-2222-2222-2222-222222222222"}`, "key")

	assert.Equal(t, nethttp.StatusNotFound, resp.StatusCode)
}

func TestDeleteFeedFollow(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	id := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	h := newHarness(t)
	h.users.user = domain.User{ID: owner}

	resp := h.do(t, nethttp.MethodDelete, "/v1/feed_follows/"+id.String(), "", "key")

	require.Equal(t, nethttp.StatusNoContent, resp.StatusCode)
	assert.Equal(t, id, h.follows.gotID)
	assert.Equal(t, owner, h.follows.gotUserID, "deletion must be scoped to the caller")
}

func TestDeleteFeedFollowRejectsBadUUID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	resp := h.do(t, nethttp.MethodDelete, "/v1/feed_follows/not-a-uuid", "", "key")

	assert.Equal(t, nethttp.StatusBadRequest, resp.StatusCode)
}

func TestDeleteFeedFollowMapsMissingTo404(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.follows.deleteErr = domain.ErrNotFound

	resp := h.do(t, nethttp.MethodDelete,
		"/v1/feed_follows/33333333-3333-3333-3333-333333333333", "", "key")

	assert.Equal(t, nethttp.StatusNotFound, resp.StatusCode)
}

func TestListPostsPassesPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantLimit  int32
		wantOffset int32
	}{
		{name: "no parameters", query: "", wantStatus: nethttp.StatusOK},
		{name: "limit and offset", query: "?limit=25&offset=50", wantStatus: nethttp.StatusOK, wantLimit: 25, wantOffset: 50},
		{name: "non numeric limit", query: "?limit=lots", wantStatus: nethttp.StatusBadRequest},
		{name: "negative limit", query: "?limit=-1", wantStatus: nethttp.StatusBadRequest},
		{name: "negative offset", query: "?offset=-1", wantStatus: nethttp.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			resp := h.do(t, nethttp.MethodGet, "/v1/posts"+tt.query, "", "key")

			require.Equal(t, tt.wantStatus, resp.StatusCode)
			if tt.wantStatus != nethttp.StatusOK {
				return
			}
			assert.Equal(t, tt.wantLimit, h.posts.gotLimit)
			assert.Equal(t, tt.wantOffset, h.posts.gotOffset)
		})
	}
}

func TestPostsResponseKeepsNullableFields(t *testing.T) {
	t.Parallel()

	title := "A title"
	h := newHarness(t)
	h.posts.posts = []domain.Post{{
		ID:          uuid.New(),
		Title:       &title,
		Description: nil,
		PublishedAt: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
		URL:         "https://example.com/a",
	}}

	resp := h.do(t, nethttp.MethodGet, "/v1/posts", "", "key")

	require.Equal(t, nethttp.StatusOK, resp.StatusCode)
	body := decode[[]map[string]any](t, resp)
	require.Len(t, body, 1)
	assert.Equal(t, "A title", body[0]["title"])
	assert.Nil(t, body[0]["description"], "a NULL description must serialise as null")
}

// The removed GET /v1/err debug endpoint must be gone.
func TestRemovedErrEndpoint(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	resp := h.do(t, nethttp.MethodGet, "/v1/err", "", "")

	assert.Equal(t, nethttp.StatusNotFound, resp.StatusCode)
}
