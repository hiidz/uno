// Takedowns: the server's operator removing a publication from Community, and
// blocking the account that published it. A takedown deletes the publication
// as Unpublish does, whoever published it: its subscribers keep their copies
// as their own (publications_release_subscribers), and the publisher keeps
// the source, shown as not published. A block is per account, every profile
// of one nuvio_user_id, and stops publishing only.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Post is one publication as a takedown shows it.
type Post struct {
	ID              uuid.UUID
	Kind            string
	Title           string
	PublishedAt     time.Time
	SubscriberCount int
}

// TakedownPreview is what a takedown of one publication would remove: the
// post, the account that published it, and every post of that account
// across its profiles, the post included, for a takedown that also blocks.
type TakedownPreview struct {
	Post         Post
	Account      string
	AccountPosts []Post
}

// Block is an account stopped from publishing: since when, and the title of
// the post whose takedown made the block.
type Block struct {
	Account   string
	BlockedAt time.Time
	Title     string
}

// postColumns are the columns scanPosts reads, from publications p.
const postColumns = `p.id, p.kind, p.title, p.published_at, p.subscriber_count`

// PreviewTakedown reads what TakeDown of publicationID would remove, or
// ErrPublicationNotFound.
func (db *DB) PreviewTakedown(ctx context.Context, publicationID uuid.UUID) (TakedownPreview, error) {
	var preview TakedownPreview
	err := db.inReadTx(ctx, func(q dbtx) error {
		post, account, err := postAndAccount(ctx, q, publicationID)
		if err != nil {
			return err
		}
		posts, err := accountPosts(ctx, q, account)
		preview = TakedownPreview{Post: post, Account: account, AccountPosts: posts}
		return err
	})
	return preview, err
}

// TakeDown deletes publication publicationID, or ErrPublicationNotFound. With
// block it deletes every publication of the account that published it, as
// they stand now, and blocks the account, in one transaction. Returns what it
// deleted.
func (db *DB) TakeDown(ctx context.Context, publicationID uuid.UUID, block bool) ([]Post, error) {
	var removed []Post
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		post, account, err := postAndAccount(ctx, tx, publicationID)
		if err != nil {
			return err
		}
		removed = []Post{post}
		if block {
			if removed, err = accountPosts(ctx, tx, account); err != nil {
				return err
			}
			if err := blockAccount(ctx, tx, account, post.Title); err != nil {
				return err
			}
		}
		return deletePosts(ctx, tx, removed)
	})
	return removed, err
}

// postAndAccount reads publication publicationID and the account of the
// profile that published it.
func postAndAccount(ctx context.Context, q dbtx, publicationID uuid.UUID) (Post, string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT `+postColumns+`, pr.nuvio_user_id
		FROM publications p JOIN profiles pr ON pr.id = p.publisher_id
		WHERE p.id = ?
	`, publicationID.String())
	if err != nil {
		return Post{}, "", fmt.Errorf("reading publication: %w", err)
	}
	var account string
	posts, err := scanPosts(rows, &account)
	switch {
	case err != nil:
		return Post{}, "", err
	case len(posts) == 0:
		return Post{}, "", ErrPublicationNotFound
	}
	return posts[0], account, nil
}

// accountPosts reads every publication of every profile of account, newest
// first.
func accountPosts(ctx context.Context, q dbtx, account string) ([]Post, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT `+postColumns+`
		FROM publications p JOIN profiles pr ON pr.id = p.publisher_id
		WHERE pr.nuvio_user_id = ?
		ORDER BY p.published_at DESC, p.id
	`, account)
	if err != nil {
		return nil, fmt.Errorf("reading the account's publications: %w", err)
	}
	return scanPosts(rows)
}

// scanPosts reads every row of rows as a Post (postColumns), followed by
// extraDests, and closes rows.
func scanPosts(rows *sql.Rows, extraDests ...any) ([]Post, error) {
	defer func() { _ = rows.Close() }()
	var posts []Post
	for rows.Next() {
		var id, publishedAt string
		var post Post
		dests := append([]any{&id, &post.Kind, &post.Title, &publishedAt, &post.SubscriberCount}, extraDests...)
		if err := rows.Scan(dests...); err != nil {
			return nil, fmt.Errorf("scanning publication: %w", err)
		}
		var p rowParser
		post.ID = p.uuid(id, "publication id")
		post.PublishedAt = p.timestamp(publishedAt, "publication published_at")
		if p.err != nil {
			return nil, p.err
		}
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating publications: %w", err)
	}
	return posts, nil
}

// deletePosts deletes each of posts' publications.
func deletePosts(ctx context.Context, tx *sql.Tx, posts []Post) error {
	for _, post := range posts {
		if _, err := tx.ExecContext(ctx, `DELETE FROM publications WHERE id = ?`, post.ID.String()); err != nil {
			return fmt.Errorf("taking down publication: %w", err)
		}
	}
	return nil
}

// blockAccount blocks account, naming title as the post the block came from.
func blockAccount(ctx context.Context, tx *sql.Tx, account, title string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO blocked_publishers (nuvio_user_id, blocked_at, title) VALUES (?, ?, ?)
		ON CONFLICT (nuvio_user_id) DO UPDATE SET blocked_at = excluded.blocked_at, title = excluded.title
	`, account, utcTimestamp(time.Now()), title); err != nil {
		return fmt.Errorf("blocking the account: %w", err)
	}
	return nil
}

// Unblock lets account publish again. Returns whether it was blocked.
func (db *DB) Unblock(ctx context.Context, account string) (bool, error) {
	res, err := db.conn.ExecContext(ctx, `DELETE FROM blocked_publishers WHERE nuvio_user_id = ?`, account)
	if err != nil {
		return false, fmt.Errorf("unblocking the account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("unblocking the account: %w", err)
	}
	return n > 0, nil
}

// Blocked lists every blocked account, newest block first.
func (db *DB) Blocked(ctx context.Context) ([]Block, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT nuvio_user_id, blocked_at, title FROM blocked_publishers ORDER BY blocked_at DESC, nuvio_user_id
	`)
	if err != nil {
		return nil, fmt.Errorf("reading blocked accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var blocks []Block
	for rows.Next() {
		var b Block
		var blockedAt string
		if err := rows.Scan(&b.Account, &blockedAt, &b.Title); err != nil {
			return nil, fmt.Errorf("scanning blocked account: %w", err)
		}
		if b.BlockedAt, err = parseTimestamp(blockedAt, "blocked_at"); err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating blocked accounts: %w", err)
	}
	return blocks, nil
}

// refuseBlocked fails with ErrBlocked when profileID's account is blocked.
func refuseBlocked(ctx context.Context, q dbtx, profileID uuid.UUID) error {
	var one int
	switch err := q.QueryRowContext(ctx, `
		SELECT 1 FROM blocked_publishers b JOIN profiles p ON p.nuvio_user_id = b.nuvio_user_id WHERE p.id = ?
	`, profileID.String()).Scan(&one); {
	case err == nil:
		return ErrBlocked
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("checking for a block: %w", err)
	}
	return nil
}
