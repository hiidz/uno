// What the addon serves a profile: the catalogs its last push put in Nuvio,
// read from the push record, never from the rows as they stand now. A Save, an
// Update or a delete changes what Nuvio is served only once a push has
// carried it.

package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// GetPublishedCatalogs returns the catalogs profileID's last push put in Nuvio,
// as the addon's manifest lists them: the ones with a Home row of their own
// (Home or Discover as pushed, in ShowInHome), then the ones only a folder of
// a pushed collection uses, all off Home. It is empty for a profile Nuvio holds nothing
// for: one that never pushed, or whose Nuvio profile slot was reused since
// (heldRecord). The manifest route reads it on every request.
func (db *DB) GetPublishedCatalogs(ctx context.Context, profileID uuid.UUID) ([]Catalog, error) {
	record, ok, err := heldRecord(ctx, db.conn, profileID)
	if err != nil {
		return nil, fmt.Errorf("loading published catalogs: %w", err)
	}
	if !ok {
		return []Catalog{}, nil
	}
	return record.selectedCatalogs(), nil
}

// ServedCatalog is what an addon catalog route serves: a catalog's params,
// and the Nuvio account that owns its profile with that account's sealed TMDB
// key, nil when the account has saved none.
type ServedCatalog struct {
	Params    string
	Account   string
	SealedKey []byte
}

// ServedCatalog returns the catalog an addon catalog route names: catalogID,
// with catalogType and catalogProvider, in the push record of the profile whose
// token it is — the manifest's set checked for this one catalog, params as the
// last push left them. It comes with the profile owner's account and sealed
// key, read live. It is the route's one lookup and its access check: a
// catalog the last push didn't put in Nuvio, another profile's catalog, an
// unknown token, a profile Nuvio holds nothing for and a type or provider the
// catalog doesn't have are all ErrCatalogNotFound.
func (db *DB) ServedCatalog(ctx context.Context, token string, catalogID uuid.UUID, catalogType, catalogProvider string) (ServedCatalog, error) {
	var served ServedCatalog
	var raw sql.NullString
	err := db.conn.QueryRowContext(ctx, `
		SELECT p.nuvio_user_id, a.tmdb_key_ciphertext, pr.record
		FROM profiles p
		LEFT JOIN accounts a ON a.nuvio_user_id = p.nuvio_user_id
		LEFT JOIN push_records pr ON pr.profile_id = p.id AND pr.nuvio_profile_uuid = p.nuvio_profile_uuid
		WHERE p.token = ?
	`, token).Scan(&served.Account, &served.SealedKey, &raw)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !raw.Valid) {
		return ServedCatalog{}, ErrCatalogNotFound
	}
	if err != nil {
		return ServedCatalog{}, fmt.Errorf("resolving served catalog: %w", err)
	}

	var held struct {
		Catalogs []PushedCatalog `json:"catalogs"`
	}
	if err := json.Unmarshal([]byte(raw.String), &held); err != nil {
		return ServedCatalog{}, fmt.Errorf("decoding push record: %w", err)
	}
	for _, c := range held.Catalogs {
		if c.ID == catalogID && c.Type == catalogType && c.Provider == catalogProvider {
			served.Params = string(c.Params)
			return served, nil
		}
	}
	return ServedCatalog{}, ErrCatalogNotFound
}
