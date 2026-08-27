package api

// import (
// 	"encoding/json"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"

// 	"github.com/hiidz/uno/internal/provider"
// )

// func TestListGenres_InvalidKind(t *testing.T) {
// 	srv, _ := newTestServer(t)

// 	req := httptest.NewRequest(http.MethodGet, "/genres/anime", nil)
// 	w := httptest.NewRecorder()

// 	srv.ServeHTTP(w, req)

// 	if w.Code != http.StatusBadRequest {
// 		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
// 	}
// }

// func TestListGenres_Success(t *testing.T) {
// 	mockTMDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		if r.URL.Path != "/genre/movie/list" {
// 			t.Errorf("unexpected path: %s", r.URL.Path)
// 		}
// 		w.Header().Set("Content-Type", "application/json")
// 		json.NewEncoder(w).Encode(map[string]any{
// 			"genres": []map[string]any{
// 				{"id": 28, "name": "Action"},
// 				{"id": 12, "name": "Adventure"},
// 			},
// 		})
// 	}))
// 	defer mockTMDB.Close()

// 	srv, _ := newTestServer(t)
// 	srv.provider = provider.NewTMDBClientWithBaseURL("test-key", mockTMDB.URL)

// 	req := httptest.NewRequest(http.MethodGet, "/genres/movie", nil)
// 	w := httptest.NewRecorder()

// 	srv.ServeHTTP(w, req)

// 	if w.Code != http.StatusOK {
// 		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
// 	}

// 	var genres []provider.Genre
// 	if err := json.Unmarshal(w.Body.Bytes(), &genres); err != nil {
// 		t.Fatalf("decode response: %v", err)
// 	}
// 	if len(genres) != 2 {
// 		t.Fatalf("got %d genres, want 2", len(genres))
// 	}
// 	if genres[0].Name != "Action" {
// 		t.Errorf("genres[0].Name = %q, want %q", genres[0].Name, "Action")
// 	}
// }
