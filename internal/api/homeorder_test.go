package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

// homeOrderFields is every field Nuvio's apps read from a home-order row,
// each of which a row Uno writes must carry.
var homeOrderFields = []string{
	"addon_id", "type", "catalog_id", "enabled", "order", "custom_title", "is_collection", "collection_id", "key",
}

// homeOrderTest is the rows a merge test works with: Uno's catalogs and
// collections, another addon's catalogs and a collection Uno didn't make.
type homeOrderTest struct {
	unoA, unoB, stale  vault.HomeRow
	pinned, collection vault.HomeRow
	foreign            uuid.UUID
	managed            map[string]bool
}

func newHomeOrderTest() homeOrderTest {
	pinned, collection, gone := uuid.New(), uuid.New(), uuid.New()
	return homeOrderTest{
		unoA:       vault.HomeRow{Type: "movie", CatalogID: "tmdb-" + uuid.NewString()},
		unoB:       vault.HomeRow{Type: "series", CatalogID: "tmdb-" + uuid.NewString()},
		stale:      vault.HomeRow{Type: "movie", CatalogID: "tmdb-" + uuid.NewString()},
		pinned:     vault.HomeRow{CollectionID: pinned},
		collection: vault.HomeRow{CollectionID: collection},
		foreign:    uuid.New(),
		managed:    map[string]bool{pinned.String(): true, collection.String(): true, gone.String(): true},
	}
}

// unoRow is row as Nuvio's apps write it into the list, at order, with
// extra fields over the nine they always write.
func unoRow(row vault.HomeRow, order int, extra string) string {
	head := homeRowHead(row)
	return fmt.Sprintf(`{"addon_id":%q,"type":%q,"catalog_id":%q,"enabled":true,"order":%d,"custom_title":"",`+
		`"is_collection":%t,"collection_id":%q,"key":%q%s}`,
		head.AddonID, head.Type, head.CatalogID, order, head.IsCollection, head.CollectionID, head.key(), extra)
}

// otherRow is a catalog of another addon at order, as Nuvio TV writes it:
// no key, and a number past float64's precision that must survive.
func otherRow(catalogID string, order int) string {
	return fmt.Sprintf(`{"addon_id":"com.linvo.cinemeta","type":"movie","catalog_id":%q,"enabled":true,`+
		`"order":%d,"custom_title":"","is_collection":false,"collection_id":"","big":12345678901234567890}`, catalogID, order)
}

// foreignCollectionRow is a collection Uno didn't make, at order.
func foreignCollectionRow(id uuid.UUID, order int) string {
	return fmt.Sprintf(`{"addon_id":"","type":"","catalog_id":"","enabled":false,"order":%d,"custom_title":"Mine",`+
		`"is_collection":true,"collection_id":%q,"key":"collection_%s"}`, order, id, id)
}

func homeList(rows ...string) json.RawMessage {
	return json.RawMessage(`{"show_catalog_type":false,"hide_catalog_underline":false,"future":{"n":12345678901234567890},` +
		`"items":[` + strings.Join(rows, ",") + `]}`)
}

// decodeHomeList splits a merged list into its top-level fields and rows.
func decodeHomeList(t *testing.T, raw json.RawMessage) (map[string]json.RawMessage, []homeItem) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("decoding merged list %s: %v", raw, err)
	}
	var items []homeItem
	if err := json.Unmarshal(top["items"], &items); err != nil {
		t.Fatalf("decoding merged rows %s: %v", top["items"], err)
	}
	return top, items
}

// rowKeys is each row's identity, read the way Nuvio TV reads it (from the
// fields, never key), after checking order runs 0…n down the list.
func rowKeys(t *testing.T, items []homeItem) []string {
	t.Helper()
	keys := make([]string, len(items))
	for i, it := range items {
		var head homeItemHead
		raw, err := json.Marshal(it)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			t.Fatalf("row %d: %v", i, err)
		}
		if head.Order != i {
			t.Fatalf("row %d (%s) has order %d, want %d", i, head.key(), head.Order, i)
		}
		keys[i] = head.key()
	}
	return keys
}

func keyOf(row vault.HomeRow) string { return homeRowHead(row).key() }

func TestMergeHomeOrderPlacesUnoRows(t *testing.T) {
	h := newHomeOrderTest()
	other1, other2 := "com.linvo.cinemeta:movie:top", "com.linvo.cinemeta:movie:year"
	foreign := "collection_" + h.foreign.String()

	tests := []struct {
		name   string
		pulled json.RawMessage
		pinned []vault.HomeRow
		rows   []vault.HomeRow
		want   []string
	}{
		{
			name:   "rows above Uno's first row stay before the block, the rest follow it",
			pulled: homeList(otherRow("top", 0), unoRow(h.unoB, 1, ""), otherRow("year", 2), foreignCollectionRow(h.foreign, 3)),
			rows:   []vault.HomeRow{h.unoA, h.unoB, h.collection},
			want:   []string{other1, keyOf(h.unoA), keyOf(h.unoB), keyOf(h.collection), other2, foreign},
		},
		{
			name:   "rows between Uno's rows follow the block, in their order",
			pulled: homeList(unoRow(h.unoA, 0, ""), otherRow("top", 1), unoRow(h.unoB, 2, ""), foreignCollectionRow(h.foreign, 3)),
			rows:   []vault.HomeRow{h.unoB, h.unoA},
			want:   []string{keyOf(h.unoB), keyOf(h.unoA), other1, foreign},
		},
		{
			name:   "the block takes the place of Uno's first row even one Uno no longer has, which is dropped",
			pulled: homeList(otherRow("top", 0), unoRow(h.stale, 1, ""), unoRow(h.unoB, 2, ""), otherRow("year", 3)),
			rows:   []vault.HomeRow{h.unoA},
			want:   []string{other1, keyOf(h.unoA), other2},
		},
		{
			name:   "a pinned collection's place doesn't count as Uno's first row",
			pulled: homeList(unoRow(h.pinned, 0, ""), otherRow("top", 1), unoRow(h.unoA, 2, ""), otherRow("year", 3), unoRow(h.unoB, 4, "")),
			pinned: []vault.HomeRow{h.pinned},
			rows:   []vault.HomeRow{h.unoA, h.unoB},
			want:   []string{keyOf(h.pinned), other1, keyOf(h.unoA), keyOf(h.unoB), other2},
		},
		{
			name:   "a list with no Uno rows gets them at its end",
			pulled: homeList(otherRow("top", 0), foreignCollectionRow(h.foreign, 1)),
			rows:   []vault.HomeRow{h.unoA, h.collection},
			want:   []string{other1, foreign, keyOf(h.unoA), keyOf(h.collection)},
		},
		{
			name:   "no list is just Uno's rows",
			pinned: []vault.HomeRow{h.pinned},
			rows:   []vault.HomeRow{h.unoA},
			want:   []string{keyOf(h.pinned), keyOf(h.unoA)},
		},
		{
			name:   "pinned collections lead the list",
			pulled: homeList(otherRow("top", 0), unoRow(h.pinned, 1, "")),
			pinned: []vault.HomeRow{h.pinned},
			rows:   []vault.HomeRow{h.unoA},
			want:   []string{keyOf(h.pinned), other1, keyOf(h.unoA)},
		},
		{
			name:   "rows sharing an order keep the order they came in",
			pulled: homeList(otherRow("top", 1), otherRow("year", 0), foreignCollectionRow(h.foreign, 1)),
			want:   []string{other2, other1, foreign},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			merged, err := mergeHomeOrder(tc.pulled, tc.pinned, tc.rows, h.managed)
			if err != nil {
				t.Fatalf("mergeHomeOrder: %v", err)
			}
			_, items := decodeHomeList(t, merged)
			if got := rowKeys(t, items); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rows = %v\nwant   %v", got, tc.want)
			}
		})
	}
}

// homeCase is one generated merge: a pulled list and the push's rows.
type homeCase struct {
	pulled        json.RawMessage
	pinned, rows  []vault.HomeRow
	managed       map[string]bool
	sortedPulled  []homeItemHead
	pinnedKeys    map[string]bool
	unoKeysWanted []string
}

// randomHomeCase builds a list of up to 10 other addons' catalogs and
// collections Uno didn't make, interwoven at random with Uno rows, some of
// which the push no longer sends; orders repeat; and the push's rows are a
// random order of some of Uno's, a few of its collections pinned, some new to
// the list.
func randomHomeCase(r *rand.Rand) homeCase {
	c := homeCase{managed: map[string]bool{}, pinnedKeys: map[string]bool{}}
	var uno []vault.HomeRow
	for i := range 6 {
		uno = append(uno, vault.HomeRow{Type: []string{"movie", "series"}[i%2], CatalogID: "tmdb-" + uuid.NewString()})
		id := uuid.New()
		c.managed[id.String()] = true
		uno = append(uno, vault.HomeRow{CollectionID: id})
	}
	var rows []string
	for _, row := range uno {
		if r.IntN(3) > 0 {
			rows = append(rows, unoRow(row, 0, ""))
		}
	}
	for a := range r.IntN(11) {
		for range 1 + r.IntN(3) {
			rows = append(rows, fmt.Sprintf(`{"addon_id":"addon.%d","type":"movie","catalog_id":%q,"enabled":true,"order":0,`+
				`"custom_title":"","is_collection":false,"collection_id":""}`, a, uuid.NewString()))
		}
	}
	for range r.IntN(4) {
		rows = append(rows, foreignCollectionRow(uuid.New(), 0))
	}
	r.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	for i := range rows {
		rows[i] = strings.Replace(rows[i], `"order":0`, fmt.Sprintf(`"order":%d`, r.IntN(len(rows)/2+1)), 1)
	}
	c.pulled = homeList(rows...)

	r.Shuffle(len(uno), func(i, j int) { uno[i], uno[j] = uno[j], uno[i] })
	for _, row := range uno[:r.IntN(len(uno)+1)] {
		if row.CatalogID == "" && r.IntN(4) == 0 {
			c.pinned = append(c.pinned, row)
			c.pinnedKeys[keyOf(row)] = true
		} else {
			c.rows = append(c.rows, row)
		}
	}
	for _, row := range slices.Concat(c.pinned, c.rows) {
		c.unoKeysWanted = append(c.unoKeysWanted, keyOf(row))
	}
	_, items, err := parseHomeOrder(c.pulled)
	if err != nil {
		panic(err)
	}
	for _, it := range items {
		c.sortedPulled = append(c.sortedPulled, it.head)
	}
	return c
}

// checkHomeGuarantees checks what every merge promises: the pinned rows lead;
// Uno's other rows follow as one block, in the push's order, and no row Uno
// no longer has survives; every other row survives, in its order; and those
// above Uno's first row in the pulled list, pinned rows aside, are exactly
// those before the block.
func checkHomeGuarantees(t *testing.T, c homeCase, keys []string) {
	t.Helper()
	var uno, others []string
	firstOther := -1
	for i, key := range keys {
		if c.managed[strings.TrimPrefix(key, "collection_")] || strings.HasPrefix(key, vault.AddonID+":") {
			uno = append(uno, key)
			continue
		}
		others = append(others, key)
		if firstOther < 0 {
			firstOther = i
		}
	}
	if !slices.Equal(uno, c.unoKeysWanted) {
		t.Fatalf("Uno rows = %v\nwant       %v", uno, c.unoKeysWanted)
	}
	if !slices.Equal(keys[:len(c.pinned)], c.unoKeysWanted[:len(c.pinned)]) {
		t.Fatalf("list starts %v, want the pinned rows %v", keys[:len(c.pinned)], c.unoKeysWanted[:len(c.pinned)])
	}
	var wantOthers []string
	before, blockSeen := 0, false
	for _, head := range c.sortedPulled {
		isUno := head.isUno(c.managed)
		blockSeen = blockSeen || isUno && !c.pinnedKeys[head.key()]
		if !isUno {
			wantOthers = append(wantOthers, head.key())
			if !blockSeen {
				before++
			}
		}
	}
	if !slices.Equal(others, wantOthers) {
		t.Fatalf("other rows = %v\nwant         %v", others, wantOthers)
	}
	above := keys[len(c.pinned) : len(c.pinned)+before]
	if !slices.Equal(above, wantOthers[:before]) {
		t.Fatalf("rows before the block = %v, want %v", above, wantOthers[:before])
	}
	if !slices.Equal(keys[len(c.pinned)+before:len(c.pinned)+before+len(c.rows)], c.unoKeysWanted[len(c.pinned):]) {
		t.Fatalf("list = %v: Uno's rows aren't one block after the %d rows above Uno", keys, before)
	}
}

// Across thousands of generated lists, every merge keeps its promises, and
// merging its own result again changes nothing.
func TestMergeHomeOrderHoldsAcrossRandomLists(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 2026))
	for range 3000 {
		c := randomHomeCase(r)
		merged, err := mergeHomeOrder(c.pulled, c.pinned, c.rows, c.managed)
		if err != nil {
			t.Fatalf("mergeHomeOrder: %v", err)
		}
		_, items := decodeHomeList(t, merged)
		checkHomeGuarantees(t, c, rowKeys(t, items))
		again, err := mergeHomeOrder(merged, c.pinned, c.rows, c.managed)
		if err != nil || !bytes.Equal(again, merged) {
			t.Fatalf("merging the result again = %s, %v\nwant it unchanged: %s", again, err, merged)
		}
	}
}

// Every other row and every top-level field comes back as it came, order
// aside; Uno's rows carry all nine fields, never null, and are shown, with a
// rename made in Nuvio and fields Uno doesn't know kept.
func TestMergeHomeOrderKeepsWhatIsNotUnos(t *testing.T) {
	h := newHomeOrderTest()
	other := otherRow("top", 0)
	foreign := foreignCollectionRow(h.foreign, 2)
	renamed := unoRow(h.unoA, 1, `,"newer_field":[1.50]`)
	renamed = strings.Replace(renamed, `"enabled":true`, `"enabled":false`, 1)
	renamed = strings.Replace(renamed, `"custom_title":""`, `"custom_title":"Renamed in Nuvio"`, 1)
	renamed = strings.Replace(renamed, `"collection_id":""`, `"collection_id":null`, 1)
	pulled := homeList(other, renamed, foreign)

	merged, err := mergeHomeOrder(pulled, nil, []vault.HomeRow{h.unoA, h.unoB}, h.managed)
	if err != nil {
		t.Fatalf("mergeHomeOrder: %v", err)
	}
	top, items := decodeHomeList(t, merged)

	var pulledTop map[string]json.RawMessage
	if err := json.Unmarshal(pulled, &pulledTop); err != nil {
		t.Fatal(err)
	}
	for name, value := range pulledTop {
		if name != "items" && !bytes.Equal(top[name], value) {
			t.Errorf("top-level %s = %s, want %s as it came", name, top[name], value)
		}
	}
	if len(top) != len(pulledTop) {
		t.Errorf("top-level fields = %d, want the %d that came", len(top), len(pulledTop))
	}

	for i, raw := range map[int]string{0: other, 3: foreign} {
		var want homeItem
		if err := json.Unmarshal(json.RawMessage(raw), &want); err != nil {
			t.Fatal(err)
		}
		want["order"] = json.RawMessage(fmt.Sprint(i))
		if !reflect.DeepEqual(items[i], want) {
			t.Errorf("row %d = %s\nwant    %s", i, items[i], raw)
		}
	}

	for _, i := range []int{1, 2} {
		for _, field := range homeOrderFields {
			if value, ok := items[i][field]; !ok || string(value) == "null" {
				t.Errorf("Uno row %d field %s = %s, want it present and not null", i, field, value)
			}
		}
		if string(items[i]["enabled"]) != "true" {
			t.Errorf("Uno row %d enabled = %s, want true", i, items[i]["enabled"])
		}
	}
	if got := string(items[1]["custom_title"]); got != `"Renamed in Nuvio"` {
		t.Errorf("renamed row's custom_title = %s, want the rename kept", got)
	}
	if got := string(items[1]["newer_field"]); got != `[1.50]` {
		t.Errorf("renamed row's newer_field = %s, want it kept as it came", got)
	}
	if got := string(items[1]["collection_id"]); got != `""` {
		t.Errorf("renamed row's collection_id = %s, want the null replaced", got)
	}
	if got := string(items[2]["key"]); got != `"`+keyOf(h.unoB)+`"` {
		t.Errorf("new row's key = %s, want %q", got, keyOf(h.unoB))
	}
}

// A list push can't read stops the merge with errHomeOrderUnreadable; an
// empty list, or one without rows, is no list.
func TestMergeHomeOrderUnreadable(t *testing.T) {
	h := newHomeOrderTest()
	for _, raw := range []string{
		`[]`, `"text"`, `{"items":{}}`, `{"items":[1]}`, `{"items":[null]}`,
		`{"items":[{"order":"3"}]}`, `{"items":[{"is_collection":"yes"}]}`,
	} {
		if _, err := mergeHomeOrder(json.RawMessage(raw), nil, []vault.HomeRow{h.unoA}, h.managed); !errors.Is(err, errHomeOrderUnreadable) {
			t.Errorf("mergeHomeOrder(%s) err = %v, want errHomeOrderUnreadable", raw, err)
		}
	}
	for _, raw := range []string{`null`, `{}`, `{"items":null}`} {
		merged, err := mergeHomeOrder(json.RawMessage(raw), nil, []vault.HomeRow{h.unoA}, h.managed)
		if err != nil {
			t.Fatalf("mergeHomeOrder(%s): %v", raw, err)
		}
		if _, items := decodeHomeList(t, merged); !reflect.DeepEqual(rowKeys(t, items), []string{keyOf(h.unoA)}) {
			t.Errorf("mergeHomeOrder(%s) rows = %s, want just Uno's", raw, merged)
		}
	}
}

// pushHomeOrderFixture is a profile with a catalog on Home, one in Discover
// only, and a pinned and an unpinned collection, and the push body selecting
// them all.
type pushHomeOrderFixture struct {
	s                        *Server
	db                       *vault.DB
	profile                  vault.Profile
	fake                     *fakeNuvio
	onHome, discover         vault.Catalog
	pinnedColl, unpinnedColl vault.CollectionWithFolders
	body                     pushRequest
}

func newPushHomeOrderFixture(t *testing.T, fake *fakeNuvio) pushHomeOrderFixture {
	t.Helper()
	db := newTestVaultDB(t)
	ctx := t.Context()
	profile, err := db.ResolveOrCreateProfile(ctx, "user-home-order", 1, "nuvio-uuid-home-order")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	f := pushHomeOrderFixture{db: db, profile: profile, fake: fake}
	for _, c := range []*vault.Catalog{&f.onHome, &f.discover} {
		if *c, err = db.CreateUserCatalog(ctx, profile.ID, vault.CatalogForm{Type: "movie", Name: "Popular", Provider: "tmdb", Params: popular}); err != nil {
			t.Fatalf("creating catalog: %v", err)
		}
	}
	f.pinnedColl = createPushableCollection(t, ctx, db, profile.ID, "Pinned")
	f.unpinnedColl = createPushableCollection(t, ctx, db, profile.ID, "Unpinned")
	f.body = pushOf(vault.CatalogSelectionForm{Catalogs: []vault.SelectedCatalogInput{
			{CatalogID: f.onHome.ID, ShowInHome: true}, {CatalogID: f.discover.ID, ShowInHome: false},
		}}, vault.CollectionSelectionForm{Collections: []vault.SelectedCollectionInput{
			{CollectionID: f.unpinnedColl.ID}, {CollectionID: f.pinnedColl.ID, PinToTop: true},
		}})
	fake.profiles = liveAs(profile)
	f.s = &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}
	return f
}

func (f pushHomeOrderFixture) push(t *testing.T) (*httptest.ResponseRecorder, pushResult) {
	t.Helper()
	reqCtx := withNuvioToken(withProfileID(t.Context(), f.profile.ID), "token")
	w := httptest.NewRecorder()
	f.s.push(w, newPushRequest(t, reqCtx, f.body))
	return w, decodePushResult(t, w)
}

// selectionWritten reports whether the local write stored any selection.
func (f pushHomeOrderFixture) selectionWritten(t *testing.T) bool {
	t.Helper()
	sel, err := f.db.GetCurrentCollectionSelection(t.Context(), f.profile.ID)
	if err != nil {
		t.Fatalf("GetCurrentCollectionSelection: %v", err)
	}
	return len(sel) != 0
}

// A push writes the home-order list from the selection it carries: the pinned
// collection first, then the Uno block where Uno's first row sat, holding the
// catalog on Home and the unpinned collection. The catalog in Discover only
// gets no row.
func TestPush_WritesHomeOrder(t *testing.T) {
	stale := vault.HomeRow{Type: "movie", CatalogID: "tmdb-" + uuid.NewString()}
	fake := &fakeNuvio{homeOrder: homeList(otherRow("top", 0), unoRow(stale, 1, ""))}
	f := newPushHomeOrderFixture(t, fake)

	if w, result := f.push(t); w.Code != http.StatusOK || !result.Success {
		t.Fatalf("status = %d, result = %+v; want a successful push", w.Code, result)
	}
	if len(fake.pushHomeOrderCalls) != 1 {
		t.Fatalf("PushHomeOrder calls = %d, want 1", len(fake.pushHomeOrderCalls))
	}
	_, items := decodeHomeList(t, fake.pushHomeOrderCalls[0])
	want := []string{
		keyOf(vault.HomeRow{CollectionID: f.pinnedColl.ID}),
		"com.linvo.cinemeta:movie:top",
		keyOf(vault.HomeRow{Type: "movie", CatalogID: vault.ManifestID(f.onHome)}),
		keyOf(vault.HomeRow{CollectionID: f.unpinnedColl.ID}),
	}
	if got := rowKeys(t, items); !reflect.DeepEqual(got, want) {
		t.Fatalf("pushed rows = %v\nwant          %v", got, want)
	}
}

// A list push can't read is refused before anything reaches Nuvio, in words
// the SPA has for it, and nothing is stored.
func TestPush_RefusesUnreadableHomeOrder(t *testing.T) {
	fake := &fakeNuvio{homeOrder: json.RawMessage(`{"items":[1]}`)}
	f := newPushHomeOrderFixture(t, fake)

	w, result := f.push(t)
	if w.Code != http.StatusBadGateway || result.Refused != refusedHomeOrderUnreadable || result.UndoFailed {
		t.Fatalf("status = %d, result = %+v; want 502 refused %q", w.Code, result, refusedHomeOrderUnreadable)
	}
	if len(fake.pushAddonsCalls)+len(fake.pushCollectionsCalls)+len(fake.pushHomeOrderCalls) != 0 {
		t.Fatalf("Nuvio writes = %d addons, %d collections, %d home order; want none",
			len(fake.pushAddonsCalls), len(fake.pushCollectionsCalls), len(fake.pushHomeOrderCalls))
	}
	if f.selectionWritten(t) {
		t.Fatal("the selection was stored despite the refusal")
	}
}

// A home-order pull that fails stops the push before any write; a rejected
// home-order push puts the collections and addons back as they were pulled.
// Neither stores anything.
func TestPush_HomeOrderFailures(t *testing.T) {
	failed := fmt.Errorf("%w: status 400", nuvio.ErrNuvioRequestFailed)
	pulledCollections := []json.RawMessage{json.RawMessage(`{"id":"` + uuid.NewString() + `","title":"Theirs"}`)}

	t.Run("pull fails", func(t *testing.T) {
		fake := &fakeNuvio{pullHomeOrderErr: failed}
		f := newPushHomeOrderFixture(t, fake)
		if w, result := f.push(t); w.Code != http.StatusBadGateway || result.Refused != "" {
			t.Fatalf("status = %d, result = %+v; want a plain 502", w.Code, result)
		}
		if len(fake.pushAddonsCalls)+len(fake.pushCollectionsCalls)+len(fake.pushHomeOrderCalls) != 0 {
			t.Fatal("Nuvio was written to despite the failed pull")
		}
		if f.selectionWritten(t) {
			t.Fatal("the selection was stored despite the failure")
		}
	})

	t.Run("push rejected", func(t *testing.T) {
		fake := &fakeNuvio{pullCollections: pulledCollections, pushHomeOrderErrs: []error{failed}}
		f := newPushHomeOrderFixture(t, fake)
		if w, result := f.push(t); w.Code != http.StatusBadGateway || result.UndoFailed {
			t.Fatalf("status = %d, result = %+v; want 502 with the undo done", w.Code, result)
		}
		if len(fake.pushHomeOrderCalls) != 1 {
			t.Fatalf("PushHomeOrder calls = %d, want just the rejected one", len(fake.pushHomeOrderCalls))
		}
		if len(fake.pushCollectionsCalls) != 2 || !reflect.DeepEqual(fake.pushCollectionsCalls[1], pulledCollections) {
			t.Fatalf("PushCollections calls = %s, want the push, then the pulled blob put back", fake.pushCollectionsCalls)
		}
		if len(fake.pushAddonsCalls) != 2 || len(fake.pushAddonsCalls[1]) != 0 {
			t.Fatalf("PushAddons calls = %+v, want the push, then the pulled (empty) list put back", fake.pushAddonsCalls)
		}
		if f.selectionWritten(t) {
			t.Fatal("the selection was stored despite the failure")
		}
	})
}

// A failed local write puts the home-order list back exactly as it was
// pulled.
func TestPush_RevertRestoresPulledHomeOrder(t *testing.T) {
	pulled := homeList(otherRow("top", 0))
	fake := &fakeNuvio{homeOrder: pulled}
	f := newPushHomeOrderFixture(t, fake)
	fake.onPushCollections = func(call int, _ []json.RawMessage) {
		if call == 0 {
			if err := f.db.DeleteUserCollection(t.Context(), f.profile.ID, f.unpinnedColl.ID); err != nil {
				t.Fatalf("deleting collection mid-push: %v", err)
			}
		}
	}

	if w, _ := f.push(t); w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if len(fake.pushHomeOrderCalls) != 2 || !bytes.Equal(fake.pushHomeOrderCalls[1], pulled) {
		t.Fatalf("PushHomeOrder calls = %s, want the push, then %s put back as pulled", fake.pushHomeOrderCalls, pulled)
	}
}

// A push body is one ordered list of rows: catalogs and collections mix, each
// row's place is the position the vault stores and the selection reads hand
// back, and the home-order list gets Uno's rows in that order, pinned first.
func TestPush_MixesCatalogsAndCollections(t *testing.T) {
	fake := &fakeNuvio{}
	f := newPushHomeOrderFixture(t, fake)
	f.body = pushRequest{Rows: []pushRow{
		{CollectionID: &f.unpinnedColl.ID},
		{CatalogID: &f.onHome.ID, ShowInHome: true},
		{CollectionID: &f.pinnedColl.ID, PinToTop: true},
		{CatalogID: &f.discover.ID},
	}}

	if w, result := f.push(t); w.Code != http.StatusOK || !result.Success {
		t.Fatalf("status = %d, result = %+v; want a successful push", w.Code, result)
	}
	_, items := decodeHomeList(t, fake.pushHomeOrderCalls[0])
	want := []string{
		keyOf(vault.HomeRow{CollectionID: f.pinnedColl.ID}),
		keyOf(vault.HomeRow{CollectionID: f.unpinnedColl.ID}),
		keyOf(vault.HomeRow{Type: "movie", CatalogID: vault.ManifestID(f.onHome)}),
	}
	if got := rowKeys(t, items); !reflect.DeepEqual(got, want) {
		t.Fatalf("pushed rows = %v\nwant          %v", got, want)
	}

	catalogs, err := f.db.GetCurrentCatalogSelection(t.Context(), f.profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	collections, err := f.db.GetCurrentCollectionSelection(t.Context(), f.profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	positions := map[uuid.UUID]int{}
	for _, c := range catalogs {
		positions[c.ID] = *c.HomeSortOrder
	}
	for _, c := range collections {
		positions[c.ID] = *c.HomeSortOrder
	}
	wantPositions := map[uuid.UUID]int{f.unpinnedColl.ID: 0, f.onHome.ID: 1, f.pinnedColl.ID: 2, f.discover.ID: 3}
	if !reflect.DeepEqual(positions, wantPositions) {
		t.Errorf("stored positions = %v, want %v", positions, wantPositions)
	}
	raw, err := json.Marshal(collections[0])
	if err != nil || !strings.Contains(string(raw), `"home_position":0`) {
		t.Errorf("a selection read = %s (%v), want its home_position on the wire", raw, err)
	}
}

// A row naming neither a catalog nor a collection, or both, is a 400 before
// anything reaches Nuvio.
func TestPush_RefusesARowThatIsNotOneThing(t *testing.T) {
	for name, row := range map[string]pushRow{
		"neither": {ShowInHome: true},
		"both":    {CatalogID: new(uuid.New()), CollectionID: new(uuid.New())},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeNuvio{}
			f := newPushHomeOrderFixture(t, fake)
			f.body = pushRequest{Rows: []pushRow{row}}
			if w, _ := f.push(t); w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			if len(fake.pushAddonsCalls)+len(fake.pushCollectionsCalls)+len(fake.pushHomeOrderCalls) != 0 {
				t.Fatal("Nuvio was written to despite the bad row")
			}
		})
	}
}
