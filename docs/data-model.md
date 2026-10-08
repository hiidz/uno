# Data model

Uno stores everything in one SQLite file (`VAULT_DB`). The schema is [`internal/vault/schema.sql`](../internal/vault/schema.sql), embedded in the binary. Uno creates it the first time it runs on an empty database.

## Concepts

- **Profile**: one Nuvio profile slot (1 to 6) of one Nuvio account. Created when you first pick it. Its random token is its addon URL.
- **Catalog**: a named TMDB recipe for movies or series. A *listed* catalog is in your library. A *scoped* catalog belongs to one collection.
- **Collection**: a set of **folders**, each with artwork and an ordered list of catalogs. A folder can narrow a catalog to one genre.
- **Home**: the ordered catalogs and collections on your home screen. Only a push changes it.
- **Publication**: a snapshot of a catalog or collection shared in Community. A **subscription** links someone's copy to it.
- **Push record**: what the profile's last push put in Nuvio. The addon serves from it.

## Tables

```mermaid
erDiagram
  PROFILES ||--o{ CATALOGS : owns
  PROFILES ||--o{ COLLECTIONS : owns
  COLLECTIONS ||--o{ FOLDERS : contains
  COLLECTIONS ||--o{ CATALOGS : scopes
  FOLDERS ||--o{ FOLDER_CATALOGS : contains
  CATALOGS ||--o{ FOLDER_CATALOGS : "referenced via"
  CATALOGS |o--o| PUBLICATIONS : "published as"
  COLLECTIONS |o--o| PUBLICATIONS : "published as"
  PUBLICATIONS ||--o{ SUBSCRIPTIONS : "followed by"
  SUBSCRIPTIONS |o--|| CATALOGS : "copy"
  SUBSCRIPTIONS |o--|| COLLECTIONS : "copy"
  ACCOUNT_KEYS ||..o{ PROFILES : "keys every profile of"
  PROFILES ||--o| PUSH_RECORDS : "last pushed"

  PUBLICATIONS {
    uuid id PK "kept when an update is published"
    uuid publisher_id FK
    string kind "catalog | collection"
    uuid catalog_id FK "the source, for a catalog; deleting it deletes the publication"
    uuid collection_id FK "the source, for a collection; deleting it deletes the publication"
    string title
    json snapshot "format uno-publication, version 1"
    string content_hash "sha256 hex of snapshot"
    int subscriber_count
    string published_at
    string updated_at
  }
  SUBSCRIPTIONS {
    uuid id PK
    uuid subscriber_id FK
    uuid publication_id FK "deleting the publication deletes it"
    uuid catalog_id FK "nullable — the copy, for a catalog"
    uuid collection_id FK "nullable — the copy, for a collection"
    string subscribed_hash "the content hash the copy was last written from"
    string created_at
  }

  PROFILES {
    uuid id PK
    string token UK
    string nuvio_user_id
    int nuvio_profile_index
    string nuvio_profile_uuid
    int home_revision "raised by each push; the Home a push is built from"
  }
  CATALOGS {
    uuid id PK
    string name
    string type "movie | series"
    string provider "tmdb"
    json params "canonical form"
    uuid owner_id FK
    uuid collection_id FK "nullable — NULL means listed"
    int home_sort_order "nullable — place on Home, one numbering with the other table; NULL means not on Home"
    bool show_in_home
    string sub_key "nullable — in a subscribed collection, its snapshot key"
    string created_at
    string updated_at
    int revision "raised by each content write"
  }
  COLLECTIONS {
    uuid id PK
    string title
    uuid owner_id FK
    bool pin_to_top "Pin, as last pushed; only push writes it"
    string view_mode "TABBED_GRID | ROWS"
    bool show_all_tab
    string backdrop_image_url
    bool focus_glow_enabled "defaults to 1, matching Nuvio"
    int home_sort_order "nullable — place on Home, one numbering with the other table; NULL means not on Home"
    string created_at
    string updated_at
    int revision "raised by each content write"
  }
  ACCOUNT_KEYS {
    string nuvio_user_id PK "as profiles.nuvio_user_id"
    string provider PK "as catalogs.provider"
    blob key_ciphertext "nonce || AES-GCM sealed key"
    string key_last4
    string updated_at
  }
  PUSH_RECORDS {
    uuid profile_id PK, FK
    string nuvio_profile_uuid "the Nuvio profile it was pushed to"
    json record "vault.PushRecord: what the last push sent"
    string pushed_at
  }
  FOLDERS {
    uuid id PK
    uuid collection_id FK
    string title
    int sort_order
    string tile_shape "POSTER | LANDSCAPE | SQUARE; no default"
    bool hide_title
    string cover_emoji
    string cover_image_url
    string focus_gif_url
    bool focus_gif_enabled "defaults to 1, matching Nuvio"
    string hero_backdrop_url
    string hero_video_url
    string title_logo_url
    string sub_key "nullable — in a subscribed collection, its snapshot key"
  }
  FOLDER_CATALOGS {
    uuid folder_id PK, FK
    uuid catalog_id PK, FK
    string genre PK "'' means unfiltered"
    int sort_order
  }
```

| Table | Holds |
|---|---|
| `profiles` | One row per Nuvio account and slot, with its addon token. |
| `catalogs` | Recipes. `collection_id` is set for a scoped catalog. |
| `collections` | Collections and their settings. |
| `folders` | A collection's folders and their artwork. |
| `folder_catalogs` | Which catalogs each folder shows, in order, with an optional genre. |
| `publications` | Published snapshots, as JSON. |
| `subscriptions` | Which copy came from which publication. |
| `account_keys` | Per-account TMDB keys, encrypted. |
| `push_records` | Each profile's last push, as JSON. |

Catalogs, collections and Home carry a `revision`. A save built from an outdated revision is refused, so two tabs can't overwrite each other.

## Limits

| Limit | Value |
|---|---|
| Catalogs in a profile's library | 200. Catalogs made inside a collection don't count. |
| Collections in a profile | 50 |
| Folders in a collection | 10 |
| Catalogs in a folder | 20. A catalog split by genre counts once per genre. |
| Catalogs one push gives Nuvio | 1,000, counting those inside collections. |
| Titles in a catalog row | 500. Nuvio sees the row end there. |

A save, import, Add or Duplicate that would go past one of the first four is refused, and so is a push past 1,000 catalogs. A profile already past a limit, from before it existed, still loads and pushes.

## Recipes

A catalog's `params` is a JSON object of TMDB discover filters, stored as a string, for example:

```json
{"sort_by":"popularity.desc","vote_count_gte":200,"with_genres":"878"}
```

Uno checks the params on every save and stores them in a canonical form. The fields are the structs in `internal/provider/models.go`.

## Bundles

Export and import use a JSON file (`"format": "uno"`, `"version": 1`) with catalogs and collections. In the file, row ids are replaced by keys. The types are in `internal/vault/bundle.go`.
