package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fakeapate/pzsm/internal/activity"
	"github.com/fakeapate/pzsm/internal/store"
)

const serveriniFixture = "# Players can hurt and kill other players\n" +
	"PVP=true\n" +
	"\n" +
	"# RCON password (Pick a strong password)\n" +
	"RCONPassword=hunter2\n" +
	"\n" +
	"# port for RCON\n" +
	"RCONPort=27015\n" +
	"\n" +
	"PublicName=My PZ Server\n" +
	"Mods=mod1;mod2\n" +
	"WorkshopItems=1;2\n" +
	"Map=Muldraugh, KY\n"

func newTestAPI(t *testing.T, iniContent string) (*API, string) {
	t.Helper()
	dir := t.TempDir()
	iniPath := filepath.Join(dir, "servertest.ini")
	if err := os.WriteFile(iniPath, []byte(iniContent), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	dbPath := filepath.Join(dir, "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(nil, nil, activity.New(st.DB(), log), log, "", iniPath), iniPath
}

func TestHandleGetServerini(t *testing.T) {
	a, _ := newTestAPI(t, serveriniFixture)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/serverini", nil)
	a.handleGetServerini(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", rr.Code, rr.Body)
	}
	var got []serveriniEntryDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	byKey := make(map[string]serveriniEntryDTO, len(got))
	for _, e := range got {
		byKey[e.Key] = e
	}
	for _, hidden := range []string{"Mods", "WorkshopItems", "Map"} {
		if _, ok := byKey[hidden]; ok {
			t.Errorf("hidden key %q leaked in response", hidden)
		}
	}
	if e, ok := byKey["RCONPassword"]; !ok {
		t.Errorf("missing RCONPassword")
	} else if e.Policy != policySecret || e.Value != "" {
		t.Errorf("RCONPassword: want policy=%s value=\"\", got policy=%s value=%q", policySecret, e.Policy, e.Value)
	}
	if e, ok := byKey["RCONPort"]; !ok {
		t.Errorf("missing RCONPort")
	} else if e.Policy != policyReadonly || e.Value != "27015" {
		t.Errorf("RCONPort: want policy=%s value=27015, got policy=%s value=%q", policyReadonly, e.Policy, e.Value)
	}
	if e, ok := byKey["PVP"]; !ok {
		t.Errorf("missing PVP")
	} else if e.Policy != policyEditable || e.Comment == "" {
		t.Errorf("PVP: want editable+comment, got %+v", e)
	}
}

func TestHandleGetServerini_NotConfigured(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := New(nil, nil, nil, log, "", "")
	rr := httptest.NewRecorder()
	a.handleGetServerini(rr, httptest.NewRequest(http.MethodGet, "/api/serverini", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func putServerini(t *testing.T, a *API, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/serverini", bytes.NewBufferString(body))
	a.handlePutServerini(rr, req)
	return rr
}

func TestHandlePutServerini_Updates(t *testing.T) {
	a, path := newTestAPI(t, serveriniFixture)
	rr := putServerini(t, a, `{"updates":{"PVP":"false","PublicName":"Renamed"}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "PVP=false\n") {
		t.Errorf("PVP not rewritten: %q", got)
	}
	if !strings.Contains(string(got), "PublicName=Renamed\n") {
		t.Errorf("PublicName not rewritten: %q", got)
	}
	// Hidden/managed lines untouched.
	if !strings.Contains(string(got), "Mods=mod1;mod2\n") {
		t.Errorf("Mods= changed unexpectedly: %q", got)
	}
}

func TestHandlePutServerini_RejectsHidden(t *testing.T) {
	a, _ := newTestAPI(t, serveriniFixture)
	rr := putServerini(t, a, `{"updates":{"Mods":"x"}}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d (%s)", rr.Code, rr.Body)
	}
	if !strings.Contains(rr.Body.String(), "managed elsewhere") {
		t.Errorf("expected 'managed elsewhere' message, got %s", rr.Body)
	}
}

func TestHandlePutServerini_RejectsReadonly(t *testing.T) {
	a, _ := newTestAPI(t, serveriniFixture)
	rr := putServerini(t, a, `{"updates":{"RCONPort":"1234"}}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d (%s)", rr.Code, rr.Body)
	}
	if !strings.Contains(rr.Body.String(), "read-only") {
		t.Errorf("expected 'read-only' message, got %s", rr.Body)
	}
}

func TestHandlePutServerini_RejectsUnknownKey(t *testing.T) {
	a, _ := newTestAPI(t, serveriniFixture)
	rr := putServerini(t, a, `{"updates":{"NotARealKey":"x"}}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d (%s)", rr.Code, rr.Body)
	}
	if !strings.Contains(rr.Body.String(), "unknown key") {
		t.Errorf("expected 'unknown key' message, got %s", rr.Body)
	}
}

func TestHandlePutServerini_EmptySecretIsNoOp(t *testing.T) {
	a, path := newTestAPI(t, serveriniFixture)
	rr := putServerini(t, a, `{"updates":{"RCONPassword":""}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body)
	}
	var resp serveriniUpdateResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Updated) != 0 {
		t.Errorf("expected no updates, got %v", resp.Updated)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "RCONPassword=hunter2\n") {
		t.Errorf("RCONPassword unexpectedly changed: %q", got)
	}
}

func TestHandlePutServerini_NonEmptySecretWrites(t *testing.T) {
	a, path := newTestAPI(t, serveriniFixture)
	rr := putServerini(t, a, `{"updates":{"RCONPassword":"newpw"}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "RCONPassword=newpw\n") {
		t.Errorf("expected secret rewritten: %q", got)
	}
}

func TestHandlePutServerini_InvalidJSON(t *testing.T) {
	a, _ := newTestAPI(t, serveriniFixture)
	rr := putServerini(t, a, `{not json`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}
