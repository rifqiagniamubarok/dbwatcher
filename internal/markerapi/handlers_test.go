package markerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore records every pushed event, satisfying the Pusher interface.
type fakeStore struct {
	mu   sync.Mutex
	seen []store.Event
}

func (f *fakeStore) Push(e store.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, e)
}

func (f *fakeStore) snapshot() []store.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.Event, len(f.seen))
	copy(out, f.seen)
	return out
}

func newTestHandlers() (*handlers, *fakeStore) {
	fs := &fakeStore{}
	h := &handlers{
		store:   fs,
		startAt: time.Now().Add(-10 * time.Second),
		version: "test",
	}
	return h, fs
}

func TestHandleMarker_TextPlain(t *testing.T) {
	h, fs := newTestHandlers()

	req := httptest.NewRequest(http.MethodPost, "/marker", strings.NewReader("TEST: create order"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	h.handleMarker(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	seen := fs.snapshot()
	require.Len(t, seen, 1)
	assert.Equal(t, store.KindMarker, seen[0].Kind)
	assert.Equal(t, "TEST: create order", seen[0].Label)
	assert.Equal(t, store.MarkerColorDefault, seen[0].Color)
}

func TestHandleMarker_JSON_WithColor(t *testing.T) {
	h, fs := newTestHandlers()

	body := `{"label":"deploy v1.2","color":"yellow"}`
	req := httptest.NewRequest(http.MethodPost, "/marker", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.handleMarker(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	seen := fs.snapshot()
	require.Len(t, seen, 1)
	assert.Equal(t, "deploy v1.2", seen[0].Label)
	assert.Equal(t, store.MarkerColorYellow, seen[0].Color)
}

func TestHandleMarker_RejectsEmptyLabel(t *testing.T) {
	h, fs := newTestHandlers()
	req := httptest.NewRequest(http.MethodPost, "/marker", strings.NewReader("   "))
	rec := httptest.NewRecorder()
	h.handleMarker(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, fs.snapshot())
}

func TestHandleMarker_RejectsLongLabel(t *testing.T) {
	h, fs := newTestHandlers()
	long := strings.Repeat("x", maxLabelLen+1)
	req := httptest.NewRequest(http.MethodPost, "/marker", strings.NewReader(long))
	rec := httptest.NewRecorder()
	h.handleMarker(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, fs.snapshot())
}

func TestHandleMarker_RejectsUnknownColor(t *testing.T) {
	h, fs := newTestHandlers()
	body := `{"label":"x","color":"purple"}`
	req := httptest.NewRequest(http.MethodPost, "/marker", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.handleMarker(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, fs.snapshot())
}

func TestHandleMarker_RejectsWrongMethod(t *testing.T) {
	h, _ := newTestHandlers()
	req := httptest.NewRequest(http.MethodGet, "/marker", nil)
	rec := httptest.NewRecorder()
	h.handleMarker(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandleLog_TextPlain(t *testing.T) {
	h, fs := newTestHandlers()

	req := httptest.NewRequest(http.MethodPost, "/log", strings.NewReader("starting suite"))
	rec := httptest.NewRecorder()
	h.handleLog(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	seen := fs.snapshot()
	require.Len(t, seen, 1)
	assert.Equal(t, store.KindLog, seen[0].Kind)
	assert.Equal(t, "starting suite", seen[0].Message)
}

func TestHandleLog_JSON(t *testing.T) {
	h, fs := newTestHandlers()

	body := `{"message":"migrations completed"}`
	req := httptest.NewRequest(http.MethodPost, "/log", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.handleLog(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	seen := fs.snapshot()
	require.Len(t, seen, 1)
	assert.Equal(t, "migrations completed", seen[0].Message)
}

func TestHandleLog_RejectsEmpty(t *testing.T) {
	h, fs := newTestHandlers()
	req := httptest.NewRequest(http.MethodPost, "/log", strings.NewReader(""))
	rec := httptest.NewRecorder()
	h.handleLog(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, fs.snapshot())
}

func TestHandleHealth(t *testing.T) {
	h, _ := newTestHandlers()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.handleHealth(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp healthJSON
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp.Status)
	assert.GreaterOrEqual(t, resp.Uptime, int64(10))
	assert.Equal(t, "test", resp.Version)
}

// End-to-end: spin up a real Server on an ephemeral port and hit it with
// http.DefaultClient. This guards the wiring in ListenAndServe / mux.
func TestServer_RoundtripOnEphemeralPort(t *testing.T) {
	fs := &fakeStore{}
	s := New(Options{
		Bind:    "127.0.0.1",
		Port:    0,
		Store:   fs,
		Version: "test",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.ListenAndServe(ctx)
	}()

	// Wait for the listener to be ready (Addr is set inside ListenAndServe).
	require.Eventually(t, func() bool { return s.Addr() != "" }, time.Second, 10*time.Millisecond)

	base := "http://" + s.Addr()

	// /health
	resp, err := http.Get(base + "/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// /marker text/plain
	resp, err = http.Post(base+"/marker", "text/plain", strings.NewReader("smoke"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// /log JSON
	resp, err = http.Post(base+"/log", "application/json", strings.NewReader(`{"message":"hi"}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// /marker JSON with color
	resp, err = http.Post(base+"/marker", "application/json", bytes.NewReader([]byte(`{"label":"deploy","color":"green"}`)))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	cancel()
	select {
	case err := <-serveErr:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}

	seen := fs.snapshot()
	require.Len(t, seen, 3)
	assert.Equal(t, store.KindMarker, seen[0].Kind)
	assert.Equal(t, "smoke", seen[0].Label)
	assert.Equal(t, store.KindLog, seen[1].Kind)
	assert.Equal(t, "hi", seen[1].Message)
	assert.Equal(t, store.KindMarker, seen[2].Kind)
	assert.Equal(t, store.MarkerColorGreen, seen[2].Color)
}
