package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/config"
	"github.com/hiidz/uno/internal/vault"
)

// usage is what a command line Uno doesn't know prints.
const usage = `usage:
  uno                                      serve
  uno takedown <publication id> [--block]  take a post down from Community; --block also takes down
                                           every post of its account and stops it publishing
  uno unblock <account id>                 let a blocked account publish again
  uno blocked                              list blocked accounts`

// errUsage is a command line Uno doesn't know.
var errUsage = errors.New(usage)

// command is one operator command, parsed from its arguments.
type command struct {
	name        string
	publication uuid.UUID // takedown
	block       bool      // takedown --block
	account     string    // unblock
}

// parseCommand reads args, the arguments after the program's name, as one
// command, or errUsage.
func parseCommand(args []string) (command, error) {
	switch {
	case len(args) == 2 && args[0] == "takedown",
		len(args) == 3 && args[0] == "takedown" && args[2] == "--block":
		id, err := uuid.Parse(args[1])
		if err != nil {
			return command{}, fmt.Errorf("%q is not a publication id: %w", args[1], errUsage)
		}
		return command{name: "takedown", publication: id, block: len(args) == 3}, nil
	case len(args) == 2 && args[0] == "unblock" && args[1] != "":
		return command{name: "unblock", account: args[1]}, nil
	case len(args) == 1 && args[0] == "blocked":
		return command{name: "blocked"}, nil
	}
	return command{}, errUsage
}

// runCommand runs the command args name against the vault at VAULT_DB,
// reading answers from in and writing to out. It opens the vault with
// vault.InitDB, so it upgrades an older database as a start would.
func runCommand(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	cmd, err := parseCommand(args)
	if err != nil {
		return err
	}
	db, err := vault.InitDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("failed to open the database: %w", err)
	}
	defer func() { _ = db.Close() }()
	return cmd.run(ctx, db, in, out)
}

// run runs cmd against db.
func (cmd command) run(ctx context.Context, db *vault.DB, in io.Reader, out io.Writer) error {
	switch cmd.name {
	case "takedown":
		return takedown(ctx, db, cmd.publication, cmd.block, in, out)
	case "unblock":
		return unblock(ctx, db, cmd.account, out)
	default:
		return listBlocked(ctx, db, out)
	}
}

// printer writes to w, keeping the first write error in err and writing
// nothing after it.
type printer struct {
	w   io.Writer
	err error
}

// printf writes format with args, as fmt.Fprintf does.
func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

// takedown shows what taking publication down removes, with every post of
// its account and the block when block is set, and acts only once the
// answer read from in is y.
func takedown(ctx context.Context, db *vault.DB, publication uuid.UUID, block bool, in io.Reader, out io.Writer) error {
	preview, err := db.PreviewTakedown(ctx, publication)
	if errors.Is(err, vault.ErrPublicationNotFound) {
		return fmt.Errorf("no publication %s in Community", publication)
	}
	if err != nil {
		return err
	}
	posts := []vault.Post{preview.Post}
	if block {
		posts = preview.AccountPosts
	}
	p := &printer{w: out}
	p.printf("Takes down:\n")
	printPosts(p, posts)
	p.printf("Published by account %s\n", preview.Account)
	if block {
		p.printf("Blocks account %s from publishing.\n", preview.Account)
	}
	p.printf("Go ahead? [y/N] ")
	if p.err != nil {
		return p.err
	}
	if !confirmed(in) {
		p.printf("Nothing changed.\n")
		return p.err
	}

	removed, err := db.TakeDown(ctx, publication, block)
	if errors.Is(err, vault.ErrPublicationNotFound) {
		return fmt.Errorf("publication %s left Community before the takedown; nothing changed", publication)
	}
	if err != nil {
		return err
	}
	p.printf("Took down:\n")
	printPosts(p, removed)
	if block {
		p.printf("Blocked account %s. To lift it: uno unblock %s\n", preview.Account, preview.Account)
	}
	return p.err
}

// confirmed reports whether the first line read from in is y.
func confirmed(in io.Reader) bool {
	line, _ := bufio.NewReader(in).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y")
}

// printPosts writes one line per post. Titles are quoted, so a title's
// control characters reach the terminal escaped.
func printPosts(p *printer, posts []vault.Post) {
	for _, post := range posts {
		p.printf("  %s %q  %s  published %s, %s\n",
			post.Kind, post.Title, post.ID, post.PublishedAt.Format("2006-01-02"), subscribers(post.SubscriberCount))
	}
}

// subscribers is n as a count of subscribers.
func subscribers(n int) string {
	if n == 1 {
		return "1 subscriber"
	}
	return fmt.Sprintf("%d subscribers", n)
}

// unblock lifts account's block, saying whether there was one.
func unblock(ctx context.Context, db *vault.DB, account string, out io.Writer) error {
	ok, err := db.Unblock(ctx, account)
	if err != nil {
		return err
	}
	p := &printer{w: out}
	if ok {
		p.printf("Unblocked account %s. It can publish again; its posts don't come back.\n", account)
	} else {
		p.printf("Account %s isn't blocked.\n", account)
	}
	return p.err
}

// listBlocked writes each blocked account with when it was blocked and the
// title of the post the block came from, newest first.
func listBlocked(ctx context.Context, db *vault.DB, out io.Writer) error {
	blocks, err := db.Blocked(ctx)
	if err != nil {
		return err
	}
	p := &printer{w: out}
	if len(blocks) == 0 {
		p.printf("No account is blocked.\n")
	}
	for _, b := range blocks {
		p.printf("%s  %s  %q\n", b.Account, b.BlockedAt.Format("2006-01-02 15:04 MST"), b.Title)
	}
	return p.err
}
