package provider

import "testing"

func TestFingerprintIgnoresKeyOrderAndWhitespace(t *testing.T) {
	a, err := Fingerprint("movie", "tmdb", `{"sort_by":"popularity.desc","with_genres":"28"}`)
	if err != nil {
		t.Fatalf("Fingerprint(a): %v", err)
	}
	b, err := Fingerprint("movie", "tmdb", `{
		"with_genres": "28",
		"sort_by":     "popularity.desc"
	}`)
	if err != nil {
		t.Fatalf("Fingerprint(b): %v", err)
	}
	if a != b {
		t.Fatalf("fingerprints differ for equivalent params: %q vs %q", a, b)
	}
}

func TestFingerprintChangesWithFilter(t *testing.T) {
	a, err := Fingerprint("movie", "tmdb", `{"with_genres":"28"}`)
	if err != nil {
		t.Fatalf("Fingerprint(a): %v", err)
	}
	b, err := Fingerprint("movie", "tmdb", `{"with_genres":"35"}`)
	if err != nil {
		t.Fatalf("Fingerprint(b): %v", err)
	}
	if a == b {
		t.Fatalf("fingerprints match for different genre filters: %q", a)
	}
}

func TestFingerprintChangesWithType(t *testing.T) {
	a, err := Fingerprint("movie", "tmdb", `{"with_genres":"28"}`)
	if err != nil {
		t.Fatalf("Fingerprint(movie): %v", err)
	}
	b, err := Fingerprint("series", "tmdb", `{"with_genres":"28"}`)
	if err != nil {
		t.Fatalf("Fingerprint(series): %v", err)
	}
	if a == b {
		t.Fatalf("fingerprints match across catalog types: %q", a)
	}
}

func TestFingerprintRejectsUnknownType(t *testing.T) {
	if _, err := Fingerprint("documentary", "tmdb", `{}`); err == nil {
		t.Fatal("expected an error for an unrecognized catalog type, got nil")
	}
}
