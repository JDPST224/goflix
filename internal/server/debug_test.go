package server

// White-box tests for the opt-in pprof routes: the docs promise they are
// auth gated, so anonymous (and non-admin) callers must never reach them.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDebugRoutesRequireAdmin(t *testing.T) {
	auth := NewAuthStore("", "")
	admin, msg := auth.register("admin", "pass1234", "", "")
	if msg != "" {
		t.Fatalf("register: %s", msg)
	}
	if _, msg := auth.register("viewer", "pass1234", "", ""); msg != "" {
		t.Fatalf("register viewer: %s", msg)
	}

	d := &Deps{Auth: auth}
	mux := http.NewServeMux()
	d.debugRoutes(mux)

	anonymous := httptest.NewRecorder()
	mux.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if anonymous.Code != http.StatusForbidden {
		t.Fatalf("anonymous pprof status = %d, want 403", anonymous.Code)
	}

	nonAdminReq := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	nonAdminReq = nonAdminReq.WithContext(context.WithValue(nonAdminReq.Context(), ctxUserID, "not-an-admin"))
	nonAdmin := httptest.NewRecorder()
	mux.ServeHTTP(nonAdmin, nonAdminReq)
	if nonAdmin.Code != http.StatusForbidden {
		t.Fatalf("non-admin pprof status = %d, want 403", nonAdmin.Code)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	adminReq = adminReq.WithContext(context.WithValue(adminReq.Context(), ctxUserID, admin.ID))
	adminRec := httptest.NewRecorder()
	mux.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin pprof status = %d, want 200", adminRec.Code)
	}
}
