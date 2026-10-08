package api

// The home-order merge: Nuvio's home-order list, the one Nuvio TV, mobile
// and desktop order a profile's home screen rows by, with Uno's rows as one
// block in the order of the push and every other row in its order around it.

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

// errHomeOrderUnreadable is a pulled home-order list push can't read. Push
// stops on it before writing anything to Nuvio: the list is full-replace, so
// a row the merge couldn't read would be gone from Nuvio once pushed back.
var errHomeOrderUnreadable = fmt.Errorf("%w: home order list unreadable", nuvio.ErrNuvioRequestFailed)

// homeItem is one row of the home-order list, each field as the raw JSON
// Nuvio sent, so fields Uno doesn't know pass through unchanged.
type homeItem map[string]json.RawMessage

// homeItemHead is the fields of a row the merge reads: what the row is and
// where it sits. A field a row lacks reads as its zero value, as Nuvio's
// apps read it.
type homeItemHead struct {
	AddonID      string `json:"addon_id"`
	Type         string `json:"type"`
	CatalogID    string `json:"catalog_id"`
	Order        int    `json:"order"`
	IsCollection bool   `json:"is_collection"`
	CollectionID string `json:"collection_id"`
}

// pulledHomeItem is one pulled row: what the merge reads of it, and all of
// it as it came.
type pulledHomeItem struct {
	head   homeItemHead
	fields homeItem
}

// key is the row's identity, in the form Nuvio mobile and desktop key their
// rows by: collection_{id} for a collection, addon:type:catalog otherwise.
func (h homeItemHead) key() string {
	if h.IsCollection {
		return "collection_" + h.CollectionID
	}
	return h.AddonID + ":" + h.Type + ":" + h.CatalogID
}

// isUno reports whether the row is one push manages: a catalog of Uno's
// addon, or a collection in managed (managedCollectionIDs).
func (h homeItemHead) isUno(managed map[string]bool) bool {
	if h.IsCollection {
		return managed[h.CollectionID]
	}
	return h.AddonID == vault.AddonID
}

// homeRowHead is row as the home-order list names it.
func homeRowHead(row vault.HomeRow) homeItemHead {
	if row.CatalogID == "" {
		return homeItemHead{IsCollection: true, CollectionID: row.CollectionID.String()}
	}
	return homeItemHead{AddonID: vault.AddonID, Type: row.Type, CatalogID: row.CatalogID}
}

// parseHomeOrder reads a pulled home-order list: its top-level fields as they
// came, and its rows in the order Nuvio's apps show them, by their order
// field with ties as they came. No list reads as an empty one. A list that
// isn't an object, items that aren't an array, or a row that isn't an object
// or holds a field of the wrong type is errHomeOrderUnreadable.
func parseHomeOrder(raw json.RawMessage) (map[string]json.RawMessage, []pulledHomeItem, error) {
	top, err := homeOrderTop(raw)
	if err != nil {
		return nil, nil, err
	}
	rows, err := homeOrderRows(top["items"])
	if err != nil {
		return nil, nil, err
	}
	items, err := parseHomeItems(rows)
	if err != nil {
		return nil, nil, err
	}
	slices.SortStableFunc(items, func(a, b pulledHomeItem) int { return a.head.Order - b.head.Order })
	return top, items, nil
}

// homeOrderTop is raw's top-level fields as they came; no list, or null, has
// none.
func homeOrderTop(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil && len(raw) > 0 {
		return nil, errHomeOrderUnreadable
	}
	if top == nil {
		top = map[string]json.RawMessage{}
	}
	return top, nil
}

// homeOrderRows is the rows raw, a list's items field, holds as they came;
// an absent or null items field holds none.
func homeOrderRows(raw json.RawMessage) ([]json.RawMessage, error) {
	var rows []json.RawMessage
	if raw == nil {
		return nil, nil
	}
	if json.Unmarshal(raw, &rows) != nil {
		return nil, errHomeOrderUnreadable
	}
	return rows, nil
}

// parseHomeItems reads each of rows, or fails with errHomeOrderUnreadable on
// the first it can't.
func parseHomeItems(rows []json.RawMessage) ([]pulledHomeItem, error) {
	items := make([]pulledHomeItem, len(rows))
	for i, row := range rows {
		item, err := parseHomeItem(row)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}

// parseHomeItem reads one row, or fails with errHomeOrderUnreadable when it
// isn't an object or holds a field the merge reads with the wrong type.
func parseHomeItem(row json.RawMessage) (pulledHomeItem, error) {
	var item pulledHomeItem
	if json.Unmarshal(row, &item.fields) != nil || item.fields == nil || json.Unmarshal(row, &item.head) != nil {
		return pulledHomeItem{}, errHomeOrderUnreadable
	}
	return item, nil
}

// mergeHomeOrder is pulled, a profile's home-order list, with Uno's rows set
// to a push's: pinned and rows as vault.PushRecord.HomeRows gives them.
//   - Uno's pinned collections lead the list, where Nuvio mobile and desktop
//     show them whichever of their startup pulls lands first.
//   - Uno's other rows stay together as one block, in the push's order, where
//     Uno's first such row sits in Nuvio's list. Every other row keeps its
//     order: those above that row stay before the block, the rest follow it.
//     The block goes at the end when the list holds no Uno row. A row Uno no
//     longer has is dropped.
//   - order is rewritten 0…n down the list, so no two rows tie.
//   - Every top-level field passes through as it came.
func mergeHomeOrder(pulled json.RawMessage, pinned, rows []vault.HomeRow, managed map[string]bool) (json.RawMessage, error) {
	top, items, err := parseHomeOrder(pulled)
	if err != nil {
		return nil, err
	}
	own := ownHomeItems(items, managed)
	at := unoBlockAt(items, managed, homeRowKeys(pinned))
	merged := slices.Concat(unoHomeItems(pinned, own), placeUnoBlock(items, unoHomeItems(rows, own), at, managed))
	numberHomeItems(merged)
	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encoding home order: %w", err)
	}
	top["items"] = encoded
	return json.Marshal(top)
}

// ownHomeItems is each of items push manages, by its key.
func ownHomeItems(items []pulledHomeItem, managed map[string]bool) map[string]homeItem {
	own := make(map[string]homeItem)
	for _, it := range items {
		if it.head.isUno(managed) {
			own[it.head.key()] = it.fields
		}
	}
	return own
}

// numberHomeItems sets each of items' order to its place, 0…n.
func numberHomeItems(items []homeItem) {
	for i, it := range items {
		it["order"] = json.RawMessage(strconv.Itoa(i))
	}
}

// unoBlockAt is where Uno's block goes in items, Nuvio's rows in order: the
// place of the first Uno row that isn't one of pinned, the rows that lead the
// list, or the end when items hold none.
func unoBlockAt(items []pulledHomeItem, managed, pinned map[string]bool) int {
	for i, it := range items {
		if it.head.isUno(managed) && !pinned[it.head.key()] {
			return i
		}
	}
	return len(items)
}

// placeUnoBlock is items, Nuvio's rows in order, without Uno's, and with uno,
// Uno's rows in order, as one block at place at.
func placeUnoBlock(items []pulledHomeItem, uno []homeItem, at int, managed map[string]bool) []homeItem {
	placed := make([]homeItem, 0, len(items)+len(uno))
	for i, it := range items {
		if i == at {
			placed = append(placed, uno...)
		}
		if !it.head.isUno(managed) {
			placed = append(placed, it.fields)
		}
	}
	if at >= len(items) {
		placed = append(placed, uno...)
	}
	return placed
}

// homeRowKeys is the key of each of rows.
func homeRowKeys(rows []vault.HomeRow) map[string]bool {
	keys := make(map[string]bool, len(rows))
	for _, row := range rows {
		keys[homeRowHead(row).key()] = true
	}
	return keys
}

// unoHomeItems is each of rows as a home-order row, built on own's pulled row
// for it where there is one.
func unoHomeItems(rows []vault.HomeRow, own map[string]homeItem) []homeItem {
	items := make([]homeItem, len(rows))
	for i, row := range rows {
		head := homeRowHead(row)
		items[i] = unoHomeItem(head, own[head.key()])
	}
	return items
}

// unoHomeItem is one of Uno's rows as the home-order list holds it: every
// field Nuvio's apps read present, since a row missing one makes them throw
// the whole list away, and enabled, since a push shows what it sends. Every
// other field the pulled row has, a rename made in Nuvio among them, is kept
// unless it is null.
func unoHomeItem(head homeItemHead, pulled homeItem) homeItem {
	item := homeItem{
		"addon_id":      jsonString(head.AddonID),
		"type":          jsonString(head.Type),
		"catalog_id":    jsonString(head.CatalogID),
		"enabled":       json.RawMessage(`true`),
		"order":         json.RawMessage(`0`),
		"custom_title":  jsonString(""),
		"is_collection": json.RawMessage(strconv.FormatBool(head.IsCollection)),
		"collection_id": jsonString(head.CollectionID),
		"key":           jsonString(head.key()),
	}
	for name, value := range pulled {
		if name != "enabled" && string(value) != "null" {
			item[name] = value
		}
	}
	return item
}

// jsonString is s as a JSON string. Marshaling a string can't fail.
func jsonString(s string) json.RawMessage {
	encoded, _ := json.Marshal(s)
	return encoded
}
