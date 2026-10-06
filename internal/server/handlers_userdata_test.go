package server

// Regression: the client stamps a numeric `at` clock into the avprefs blob
// (storage.js saveAVPrefs), so the server must decode avprefs without
// requiring every value to be a string.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"goflix/internal/db"
)

func TestUserdataSyncAcceptsNumericAVPrefsClock(t *testing.T) {
	d, err := db.Open("")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.EnsureSchema(d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	dep := &Deps{UserData: NewUserDataStore(d)}
	body := `{"mylist":[],"progress":{},"cw":[],"removed":{},"avprefs":{"audio":"eng","sub":"eng","at":1790164110000},"avprefs_at":1790164110000}`
	req := httptest.NewRequest(http.MethodPost, "/api/userdata/sync", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxUserID, "user-1"))
	rec := httptest.NewRecorder()
	dep.userdataSyncHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if want := `"audio":"eng"`; !bytes.Contains(rec.Body.Bytes(), []byte(want)) {
		t.Fatalf("merged response missing avprefs audio: %s", rec.Body.String())
	}
	if want := `"at":1790164110000`; !bytes.Contains(rec.Body.Bytes(), []byte(want)) {
		t.Fatalf("merged response lost the numeric avprefs clock: %s", rec.Body.String())
	}
}
