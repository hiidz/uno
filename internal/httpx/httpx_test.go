package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorAnswersJSONWords(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, `unknown field "name"`)
	if w.Code != http.StatusBadRequest || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("answer = %d %q, want 400 application/json", w.Code, w.Header().Get("Content-Type"))
	}
	if want := `{"error":"unknown field \"name\""}` + "\n"; w.Body.String() != want {
		t.Errorf("body = %q, want %q", w.Body.String(), want)
	}
}

func TestWriteCodedErrorNamesTheError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteCodedError(w, http.StatusNotFound, "profile not found", "profile_not_found")
	if want := `{"error":"profile not found","code":"profile_not_found"}` + "\n"; w.Body.String() != want {
		t.Errorf("body = %q, want %q", w.Body.String(), want)
	}
}

func TestPathUUIDRefusesInJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("catalogID", "nope")
	w := httptest.NewRecorder()
	if _, ok := PathUUID(w, r, "catalogID", "catalog id"); ok {
		t.Fatal("PathUUID accepted a malformed id")
	}
	if want := `{"error":"invalid catalog id"}` + "\n"; w.Code != http.StatusBadRequest || w.Body.String() != want {
		t.Errorf("answer = %d %q, want 400 %q", w.Code, w.Body.String(), want)
	}
}
