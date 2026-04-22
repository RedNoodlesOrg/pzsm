package steam

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.steampowered.com/ISteamRemoteStorage"

// Client is the concurrency-safe HTTP client for the ISteamRemoteStorage API.
type Client struct {
	http *http.Client
	base string
}

// Option configures the Client. Useful for tests that point at a mock server.
type Option func(*Client)

// WithBaseURL overrides the Steam API base URL.
func WithBaseURL(base string) Option {
	return func(c *Client) { c.base = base }
}

// WithHTTPClient injects a custom *http.Client (e.g., for tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
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
func (c *Client) GetCollectionDetails(ctx context.Context, ids []string) ([]CollectionDetails, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	form := formIDs("collectioncount", "publishedfileids", ids)
	var env envelope[collectionDetailsResponse]
	if err := c.post(ctx, "/GetCollectionDetails/v1/", form, &env); err != nil {
		return nil, err
	}
	if env.Response.Result != resultOK {
		return nil, fmt.Errorf("steam: collection details result=%d", env.Response.Result)
	}
	return env.Response.CollectionDetails, nil
}

// GetPublishedFileDetails resolves one or more workshop ids to their metadata.
func (c *Client) GetPublishedFileDetails(ctx context.Context, ids []string) ([]PublishedFileDetails, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	form := formIDs("itemcount", "publishedfileids", ids)
	var env envelope[publishedFileDetailsResponse]
	if err := c.post(ctx, "/GetPublishedFileDetails/v1/", form, &env); err != nil {
		return nil, err
	}
	if env.Response.Result != resultOK {
		return nil, fmt.Errorf("steam: file details result=%d", env.Response.Result)
	}
	return env.Response.PublishedFileDetails, nil
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

func formIDs(countKey, idsKey string, ids []string) url.Values {
	form := url.Values{}
	form.Set(countKey, strconv.Itoa(len(ids)))
	for i, id := range ids {
		form.Set(fmt.Sprintf("%s[%d]", idsKey, i), id)
	}
	return form
}

func (c *Client) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("steam: new request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("steam: %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("steam: %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("steam: decode %s: %w", path, err)
	}
	return nil
}
