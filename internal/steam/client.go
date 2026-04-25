package steam

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.steampowered.com"

// Client is the concurrency-safe HTTP client for the Steam Web API endpoints
// pzsm uses: ISteamRemoteStorage for collection expansion (legacy, no key) and
// IPublishedFileService for per-item details (modern, key required).
type Client struct {
	http   *http.Client
	base   string
	apiKey string
}

// Option configures the Client. Useful for tests that point at a mock server.
type Option func(*Client)

// WithBaseURL overrides the Steam API base URL (host root, no trailing slash).
func WithBaseURL(base string) Option {
	return func(c *Client) { c.base = strings.TrimRight(base, "/") }
}

// WithHTTPClient injects a custom *http.Client (e.g., for tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithAPIKey sets the Steam Web API key used by the modern endpoints.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// New returns a Client with sensible defaults.
func New(opts ...Option) *Client {
	c := &Client{
		http: &http.Client{Timeout: 10 * time.Second},
		base: defaultBaseURL,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// GetCollectionDetails resolves one or more collection ids to their children.
// Uses the legacy ISteamRemoteStorage endpoint, which does not require a key
// and continues to return correct results for collection expansion.
func (c *Client) GetCollectionDetails(ctx context.Context, ids []string) ([]CollectionDetails, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	raw, err := c.SteamRemoteStorageGetCollectionDetails(ctx, SteamRemoteStorageGetCollectionDetailsRequest{
		Collectioncount:  uint32(len(ids)),
		PublishedFileIDs: ids,
	})
	if err != nil {
		return nil, err
	}
	var env envelope[collectionDetailsResponse]
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("steam: decode collection details: %w", err)
	}
	if env.Response.Result != resultOK {
		return nil, fmt.Errorf("steam: collection details result=%d", env.Response.Result)
	}
	return env.Response.CollectionDetails, nil
}

// publishedFileDetailsBatch caps ids per IPublishedFileService/GetDetails call.
// Steam's gateway enforces GET on this endpoint and rejects URLs over 8 KB;
// 100 indexed ids leaves comfortable headroom (~4 KB).
const publishedFileDetailsBatch = 100

// GetPublishedFileDetails resolves one or more workshop ids to their metadata.
// Uses the modern IPublishedFileService endpoint, which returns unlisted /
// non-default-visibility items that the legacy endpoint masked as result=9.
// Requires a Steam Web API key (configured via WithAPIKey).
//
// Stays hand-written rather than delegating to the generated
// PublishedFileServiceGetDetails: GetSupportedAPIList types `publishedfileids`
// as a single uint64, but Steam actually requires the legacy indexed form
// (publishedfileids[0]=…&publishedfileids[1]=…) to batch multiple ids. Calls
// are chunked because the indexed query string blows past Steam's 8 KB URL
// limit at ~250 ids.
func (c *Client) GetPublishedFileDetails(ctx context.Context, ids []string) ([]PublishedFileDetails, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("steam: api key not configured")
	}
	out := make([]PublishedFileDetails, 0, len(ids))
	for start := 0; start < len(ids); start += publishedFileDetailsBatch {
		end := min(start+publishedFileDetailsBatch, len(ids))
		chunk := ids[start:end]

		q := url.Values{}
		q.Set("key", c.apiKey)
		for i, id := range chunk {
			q.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
		}
		var env envelope[publishedFileDetailsResponse]
		if err := c.get(ctx, "/IPublishedFileService/GetDetails/v1/", q, &env); err != nil {
			return nil, err
		}
		out = append(out, env.Response.PublishedFileDetails...)
	}
	return out, nil
}

// ExpandCollection recursively resolves a top-level collection to every
// COMMUNITY (mod) workshop id reachable from it, walking nested sub-collections.
func (c *Client) ExpandCollection(ctx context.Context, collectionID string) ([]string, error) {
	top, err := c.GetCollectionDetails(ctx, []string{collectionID})
	if err != nil {
		return nil, err
	}
	if len(top) != 1 {
		return nil, fmt.Errorf("steam: expected 1 collection, got %d", len(top))
	}
	return c.expand(ctx, top[0])
}

func (c *Client) expand(ctx context.Context, cd CollectionDetails) ([]string, error) {
	var ids []string
	for _, child := range cd.Children {
		switch child.FileType {
		case WorkshopFileCommunity:
			ids = append(ids, child.PublishedFileID)
		case WorkshopFileCollection:
			sub, err := c.GetCollectionDetails(ctx, []string{child.PublishedFileID})
			if err != nil {
				return nil, err
			}
			if len(sub) != 1 {
				return nil, fmt.Errorf("steam: sub-collection %s returned %d results", child.PublishedFileID, len(sub))
			}
			subIDs, err := c.expand(ctx, sub[0])
			if err != nil {
				return nil, err
			}
			ids = append(ids, subIDs...)
		}
	}
	return ids, nil
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("steam: new request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, path, out)
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("steam: new request %s: %w", path, err)
	}
	return c.do(req, path, out)
}

func (c *Client) do(req *http.Request, path string, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("steam: %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		snippet := strings.TrimSpace(string(body))
		if snippet == "" {
			return fmt.Errorf("steam: %s: status %d (empty body)", path, resp.StatusCode)
		}
		return fmt.Errorf("steam: %s: status %d: %s", path, resp.StatusCode, snippet)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("steam: decode %s: %w", path, err)
	}
	return nil
}
