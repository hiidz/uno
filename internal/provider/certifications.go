package provider

import (
	"context"
	"fmt"
)

// Certifications returns TMDB's age-rating systems, grouped by country. Each
// country has its own scale (US movie ratings are G/PG/PG-13/R/NC-17; US TV
// ratings are a different scale entirely), so the country is part of the
// answer, not a filter on it — the caller needs the whole map to let someone
// pick a country and then see that country's ratings.
//
// catalogType is Uno/Stremio's vocabulary ("movie"/"series"), matching
// Genres above; translated via externalIDsMediaType.
func (c *TMDBClient) Certifications(ctx context.Context, catalogType string) (map[string][]Certification, error) {
	kind, ok := externalIDsMediaType[catalogType]
	if !ok {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}

	var out certificationListResponse
	if err := c.get(ctx, fmt.Sprintf("/certification/%s/list", kind), nil, &out); err != nil {
		return nil, err
	}
	if out.Certifications == nil {
		out.Certifications = map[string][]Certification{}
	}
	return out.Certifications, nil
}
