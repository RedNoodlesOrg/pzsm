package steam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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

	mux := http.NewServeMux()
	mux.HandleFunc("POST /GetCollectionDetails/v1/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(coll)
	})
	mux.HandleFunc("POST /GetPublishedFileDetails/v1/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(files)
	})
	return httptest.NewServer(mux)
}

func TestClient_ExpandCollection(t *testing.T) {
	ts := mockSteam(t)
	defer ts.Close()

	c := New(WithBaseURL(ts.URL))
	ids, err := c.ExpandCollection(context.Background(), "3709742798")
	if err != nil {
		t.Fatalf("ExpandCollection: %v", err)
	}
	if got, want := len(ids), 302; got != want {
		t.Errorf("len(ids) = %d, want %d", got, want)
	}
}

func TestClient_GetPublishedFileDetails(t *testing.T) {
	ts := mockSteam(t)
	defer ts.Close()

	c := New(WithBaseURL(ts.URL))
	ids, err := c.ExpandCollection(context.Background(), "3709742798")
	if err != nil {
		t.Fatalf("ExpandCollection: %v", err)
	}
	files, err := c.GetPublishedFileDetails(context.Background(), ids)
	if err != nil {
		t.Fatalf("GetPublishedFileDetails: %v", err)
	}
	if got, want := len(files), 302; got != want {
		t.Errorf("len(files) = %d, want %d", got, want)
	}
	var ok int
	for _, f := range files {
		if f.Result == resultOK {
			ok++
		}
	}
	if got, want := ok, 301; got != want {
		t.Errorf("result-ok count = %d, want %d", got, want)
	}
}

func TestClient_GetPublishedFileDetails_EmptyInputSkipsNetwork(t *testing.T) {
	// Pointing at a URL that would fail if actually hit.
	c := New(WithBaseURL("http://127.0.0.1:1"))
	files, err := c.GetPublishedFileDetails(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil files for empty input, got %v", files)
	}
}
