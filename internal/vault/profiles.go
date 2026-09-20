package vault

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ResolveOrCreateProfile returns the Uno profile for the (nuvioUserID,
// profileIndex) slot, creating one with a fresh token if none exists yet.
// If the slot's stored Nuvio profile UUID has drifted (the user deleted and
// recreated the Nuvio profile at that index), it's updated in place.
func (db *DB) ResolveOrCreateProfile(ctx context.Context, nuvioUserID string, profileIndex int, nuvioProfileUUID string) (Profile, error) {
	// 1. Try to find an existing row.
	p, err := db.findProfileBySlot(ctx, nuvioUserID, profileIndex)
	if err == nil {
		// 2. Found it — if the UUID drifted (slot reuse), update it.
		if p.NuvioProfileUUID != nuvioProfileUUID {
			if err := db.updateProfileUUID(ctx, p.ID, nuvioProfileUUID); err != nil {
				return Profile{}, fmt.Errorf("updating profile uuid: %w", err)
			}
			p.NuvioProfileUUID = nuvioProfileUUID
		}
		return p, nil
	}
	if !errors.Is(err, ErrProfileNotFound) {
		return Profile{}, fmt.Errorf("resolving profile: %w", err)
	}

	// 3. Not found — create it.
	p, err = db.insertProfile(ctx, nuvioUserID, profileIndex, nuvioProfileUUID)
	if err != nil {
		// 4. Race: another request inserted the same (user, index) between
		// our SELECT and our INSERT. Fall back to a fresh SELECT rather
		// than surfacing the constraint violation as a 500.
		if isUniqueConstraintErr(err) {
			return db.findProfileBySlot(ctx, nuvioUserID, profileIndex)
		}
		return Profile{}, fmt.Errorf("creating profile: %w", err)
	}
	return p, nil
}

// GetProfileBySlot resolves a profile by (nuvioUserID, profileIndex) only —
// no create, no drift handling. Used by the bearer-auth CRUD path, where a
// missing profile is a 404, not something to provision on the fly.
func (db *DB) GetProfileBySlot(ctx context.Context, nuvioUserID string, profileIndex int) (uuid.UUID, error) {
	p, err := db.findProfileBySlot(ctx, nuvioUserID, profileIndex)
	if err != nil {
		return uuid.UUID{}, err
	}
	return p.ID, nil
}

// GetProfileByID looks up a profile by its primary key — used at push time,
// where the caller already has profileID from requireProfile but also needs
// Token and NuvioProfileIndex, which that middleware doesn't stash.
func (db *DB) GetProfileByID(ctx context.Context, id uuid.UUID) (Profile, error) {
	var p Profile
	var idStr string

	err := db.conn.QueryRowContext(ctx,
		`SELECT id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid
		 FROM profiles
		 WHERE id = ?`,
		id.String(),
	).Scan(&idStr, &p.Token, &p.NuvioUserID, &p.NuvioProfileIndex, &p.NuvioProfileUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Profile{}, ErrProfileNotFound
		}
		return Profile{}, fmt.Errorf("finding profile: %w", err)
	}

	p.ID, err = parseUUID(idStr, "profile id")
	if err != nil {
		return Profile{}, err
	}
	return p, nil
}

// ResolveProfileID looks up a profile by token, wrapping sql.ErrNoRows as ErrProfileNotFound.
func (db *DB) ResolveProfileID(ctx context.Context, token string) (uuid.UUID, error) {
	var profileIDStr string
	err := db.conn.QueryRowContext(ctx,
		`SELECT id FROM profiles WHERE token = ?`, token,
	).Scan(&profileIDStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.UUID{}, ErrProfileNotFound
		}
		return uuid.UUID{}, fmt.Errorf("resolving profile: %w", err)
	}

	return parseUUID(profileIDStr, "profile id")
}

func (db *DB) findProfileBySlot(ctx context.Context, nuvioUserID string, profileIndex int) (Profile, error) {
	var p Profile
	var idStr string

	err := db.conn.QueryRowContext(ctx,
		`SELECT id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid
		 FROM profiles
		 WHERE nuvio_user_id = ? AND nuvio_profile_index = ?`,
		nuvioUserID, profileIndex,
	).Scan(&idStr, &p.Token, &p.NuvioUserID, &p.NuvioProfileIndex, &p.NuvioProfileUUID)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Profile{}, ErrProfileNotFound
		}
		return Profile{}, fmt.Errorf("finding profile: %w", err)
	}

	id, err := parseUUID(idStr, "profile id")
	if err != nil {
		return Profile{}, err
	}
	p.ID = id
	return p, nil
}

func (db *DB) insertProfile(ctx context.Context, nuvioUserID string, profileIndex int, nuvioProfileUUID string) (Profile, error) {
	token, err := newToken()
	if err != nil {
		return Profile{}, err
	}

	p := Profile{
		ID:                uuid.New(),
		Token:             token,
		NuvioUserID:       nuvioUserID,
		NuvioProfileIndex: profileIndex,
		NuvioProfileUUID:  nuvioProfileUUID,
	}

	_, err = db.conn.ExecContext(ctx,
		`INSERT INTO profiles (id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid)
		 VALUES (?, ?, ?, ?, ?)`,
		p.ID.String(), p.Token, p.NuvioUserID, p.NuvioProfileIndex, p.NuvioProfileUUID,
	)
	if err != nil {
		return Profile{}, err // caller checks for the unique-constraint case
	}
	return p, nil
}

func newToken() (string, error) {
	b := make([]byte, 16) // 128 bits of entropy
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isUniqueConstraintErr(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
	}
	return false
}

func (db *DB) updateProfileUUID(ctx context.Context, id uuid.UUID, nuvioProfileUUID string) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE profiles SET nuvio_profile_uuid = ? WHERE id = ?`,
		nuvioProfileUUID, id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating profile uuid: %w", err)
	}
	return nil
}
