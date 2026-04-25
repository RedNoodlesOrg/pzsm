package steam

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockSteam serves the captured fixtures from testdata as if it were the real
// ISteamRemoteStorage API. Every POST returns the same canned response per path.
func mockSteam(t *testing.T) *httptest.Server {
	t.Helper()

	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		return b
	}
	coll := read("collection_details.json")
	files := read("published_file_details.json")

	// Pre-parse the captured fixture and index it by id, so the mock can return
	// only the ids actually requested (mirroring real Steam behavior). This is
	// required for the chunked-batch path in GetPublishedFileDetails.
	var filesEnv envelope[publishedFileDetailsResponse]
	if err := json.Unmarshal(files, &filesEnv); err != nil {
		t.Fatalf("parse files fixture: %v", err)
	}
	byID := make(map[string]PublishedFileDetails, len(filesEnv.Response.PublishedFileDetails))
	for _, d := range filesEnv.Response.PublishedFileDetails {
		byID[d.PublishedFileID] = d
	}

	filesHandler := func(w http.ResponseWriter, r *http.Request) {
		var matched []PublishedFileDetails
		for k, v := range r.URL.Query() {
			if !strings.HasPrefix(k, "publishedfileids[") || len(v) == 0 {
				continue
			}
			if d, ok := byID[v[0]]; ok {
				matched = append(matched, d)
			}
		}
		resp := envelope[publishedFileDetailsResponse]{
			Response: publishedFileDetailsResponse{
				Result:               resultOK,
				ResultCount:          len(matched),
				PublishedFileDetails: matched,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /ISteamRemoteStorage/GetCollectionDetails/v1/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(coll)
	})
	mux.HandleFunc("GET /IPublishedFileService/GetDetails/v1/", filesHandler)
	return httptest.NewServer(mux)
}

func TestClient_ExpandCollection(t *testing.T) {
	ts := mockSteam(t)
	defer ts.Close()

	c := New(WithBaseURL(ts.URL))
	ids, err := c.ExpandCollection(context.Background(), "3707778024")
	if err != nil {
		t.Fatalf("ExpandCollection: %v", err)
	}
	if got, want := len(ids), 248; got != want {
		t.Errorf("len(ids) = %d, want %d", got, want)
	}
}

func TestClient_GetPublishedFileDetails(t *testing.T) {
	ts := mockSteam(t)
	defer ts.Close()

	c := New(WithBaseURL(ts.URL), WithAPIKey("test"))
	ids, err := c.ExpandCollection(context.Background(), "3707778024")
	if err != nil {
		t.Fatalf("ExpandCollection: %v", err)
	}
	files, err := c.GetPublishedFileDetails(context.Background(), ids)
	if err != nil {
		t.Fatalf("GetPublishedFileDetails: %v", err)
	}
	if got, want := len(files), 248; got != want {
		t.Errorf("len(files) = %d, want %d", got, want)
	}
	var ok int
	for _, f := range files {
		if f.Result == resultOK {
			ok++
		}
	}
	if got, want := ok, 248; got != want {
		t.Errorf("result-ok count = %d, want %d", got, want)
	}
}

func TestClient_GetPublishedFileDetails_EmptyInputSkipsNetwork(t *testing.T) {
	// Pointing at a URL that would fail if actually hit.
	c := New(WithBaseURL("http://127.0.0.1:1"), WithAPIKey("test"))
	files, err := c.GetPublishedFileDetails(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil files for empty input, got %v", files)
	}
}

func TestClient_GetPublishedFileDetails_RequiresAPIKey(t *testing.T) {
	c := New(WithBaseURL("http://127.0.0.1:1"))
	_, err := c.GetPublishedFileDetails(context.Background(), []string{"1"})
	if err == nil {
		t.Fatal("expected error when api key is missing")
	}
}

// TestClient_GetPublishedFileDetails_ChunksRunConcurrently asserts the chunked
// fan-out actually fans out: with 3 chunks served by a handler that sleeps
// 100ms, the total wall-clock should be near 100ms (concurrent) not ~300ms
// (serial). 200ms is the upper bound that catches a regression to serial
// without flaking on slow CI.
func TestClient_GetPublishedFileDetails_ChunksRunConcurrently(t *testing.T) {
	const perRequestDelay = 100 * time.Millisecond
	var inflight, peak atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("GET /IPublishedFileService/GetDetails/v1/", func(w http.ResponseWriter, r *http.Request) {
		n := inflight.Add(1)
		defer inflight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(perRequestDelay)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"result":1,"resultcount":0,"publishedfiledetails":[]}}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ids := make([]string, 0, 3*publishedFileDetailsBatch)
	for i := 0; i < cap(ids); i++ {
		ids = append(ids, fmt.Sprintf("%d", i+1))
	}

	c := New(WithBaseURL(ts.URL), WithAPIKey("test"))
	start := time.Now()
	if _, err := c.GetPublishedFileDetails(context.Background(), ids); err != nil {
		t.Fatalf("GetPublishedFileDetails: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed >= 2*perRequestDelay {
		t.Errorf("elapsed %v: chunks appear to be serial (>= 2x per-request delay)", elapsed)
	}
	if got := peak.Load(); got < 2 {
		t.Errorf("peak inflight = %d, want >= 2 (chunks did not overlap)", got)
	}
}

// TestGenerated_PublishedFileServiceGetDetails exercises the auto-generated
// raw client method end-to-end against the same fixture used by the curated
// wrapper. Proves the generator's request encoding and transport wiring work.
func TestGenerated_PublishedFileServiceGetDetails(t *testing.T) {
	ts := mockSteam(t)
	defer ts.Close()

	c := New(WithBaseURL(ts.URL))
	raw, err := c.PublishedFileServiceGetDetails(context.Background(), PublishedFileServiceGetDetailsRequest{
		Key:              "test",
		PublishedFileIDs: "12345",
	})
	if err != nil {
		t.Fatalf("PublishedFileServiceGetDetails: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"response"`)) {
		t.Errorf("raw response missing \"response\" key, got %d bytes: %.120s...", len(raw), raw)
	}
}
