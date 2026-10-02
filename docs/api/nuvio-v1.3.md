# Nuvio public API

> **Version:** 1.3 &middot; **Last updated:** August 20, 2026
> **Base URL:** `https://api.nuvio.tv`

The Nuvio API gives third-party clients public REST and RPC endpoints for working with Nuvio user data. Its Supabase-compatible interface supports authenticated operations for profiles, addons, library, watch progress, watch history, collections, and profile settings.

> **Documentation and API use notice**
>
> The Nuvio open-source project includes this documentation under the same project license unless stated otherwise. That license does not grant rights to Nuvio names, logos, branding, or access to the hosted Nuvio service. Separate terms, trademark rules, or service restrictions may apply.

This reference documents the officially supported public API. Endpoints and workflows not listed here are internal and may change without notice. It does not cover private or operational endpoints or destructive account flows.

---

## Table of contents

- [Getting started](#getting-started)
  - [Base URLs](#base-urls)
  - [Publishable key](#publishable-key)
  - [Authentication](#authentication)
  - [Making requests](#making-requests)
  - [Error handling](#error-handling)
  - [Rate limits](#rate-limits)
- [Authentication](#authentication-1)
  - [Sign up (email/password)](#sign-up-emailpassword)
  - [Sign in (email/password)](#sign-in-emailpassword)
  - [Refresh token](#refresh-token)
  - [Sign out](#sign-out)
  - [Get current user](#get-current-user)
- [Profiles](#profiles)
  - [List profiles](#list-profiles)
  - [Update profiles](#update-profiles)
  - [Delete profile data](#delete-profile-data)
- [Addons](#addons)
  - [List addons](#list-addons)
  - [Sync addons (push)](#sync-addons-push)
- [Library](#library)
  - [Get library](#get-library)
  - [Get library delta cursor](#get-library-delta-cursor)
  - [Get library delta](#get-library-delta)
  - [Upsert library items](#upsert-library-items)
  - [Delete library items](#delete-library-items)
  - [Sync library (push): legacy full replace](#sync-library-push-legacy-full-replace)
- [Watch progress](#watch-progress)
  - [Get watch progress](#get-watch-progress)
  - [Get watch progress delta](#get-watch-progress-delta)
  - [Get watch progress delta cursor](#get-watch-progress-delta-cursor)
  - [Sync watch progress (push)](#sync-watch-progress-push)
  - [Delete watch progress (single)](#delete-watch-progress-single)
  - [Delete watch progress (batch)](#delete-watch-progress-batch)
- [Watch history](#watch-history)
  - [Get watch history](#get-watch-history)
  - [Get watch history delta](#get-watch-history-delta)
  - [Get watch history delta cursor](#get-watch-history-delta-cursor)
  - [Sync watch history (push)](#sync-watch-history-push)
  - [Delete watch history](#delete-watch-history)
- [Profile settings](#profile-settings)
  - [Get settings](#get-settings)
  - [Update settings](#update-settings)
- [Home catalog settings](#home-catalog-settings)
  - [Get home catalog settings](#get-home-catalog-settings)
  - [Update home catalog settings](#update-home-catalog-settings)
- [Collections](#collections)
  - [Get collections](#get-collections)
  - [Update collections](#update-collections)
- [Avatars](#avatars)
  - [List avatars](#list-avatars)
- [Supporter Wall](#supporter-wall)
- [Sync overview](#sync-overview)
- [Health check](#health-check)
- [Concepts](#concepts)
  - [Profile system](#profile-system)
  - [Sync strategies](#sync-strategies)
  - [Incremental sync](#incremental-sync)
  - [Progress key format](#progress-key-format)

---

## Getting started

### Base URLs

| Service | URL |
|---------|-----|
| REST API | `https://api.nuvio.tv/rest/v1/` |
| Auth | `https://api.nuvio.tv/auth/v1/` |
| Edge Functions | `https://api.nuvio.tv/functions/v1/` |

### Publishable key

Use the Nuvio publishable key below for public clients. Most `auth/v1` and `rest/v1` requests require it.

```
apikey: sb_publishable_1Clq8rlTVACkdcZuqr6_AD__xUUC_EN
```

Notes:

- External developers need this key for normal client access. They cannot discover it unless Nuvio publishes it in documentation or another public configuration.
- Use the Nuvio API publishable key for public clients and public documentation.
- The legacy anonymous key is also public, but new integrations should use the publishable key.
- Do not expose a service role key in client apps, browser code, mobile apps, or public docs.

Examples may use placeholders, but the public documentation or SDK setup instructions must provide the real publishable key.

### Authentication

Signing in returns an `access_token`. Include it with every authenticated request:

```
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

### Making requests

The API supports two request styles:

**1. RPC calls** (recommended for all data sync operations):

```
POST /rest/v1/rpc/<function_name>
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>

{ "param1": "value1", "param2": "value2" }
```

**2. Direct table queries** (for simple reads with filtering):

```
GET /rest/v1/<table_name>?select=*&column=eq.value
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

Direct table queries use the [PostgREST query syntax](https://postgrest.org/en/stable/references/api/tables_views.html).

### Error handling

API errors use standard HTTP status codes. The response body provides more detail:

```json
{
  "code": "PGRST202",
  "message": "Could not find the function ...",
  "details": null,
  "hint": null
}
```

| Status | Meaning |
|--------|---------|
| `200` | Success |
| `201` | Created |
| `204` | No content (void RPCs) |
| `400` | Bad request / invalid parameters |
| `401` | Missing or expired authentication |
| `403` | Forbidden (RLS policy violation) |
| `404` | Not found |
| `409` | Conflict (duplicate key) |
| `422` | Unprocessable entity |
| `429` | Rate limited |

### Rate limits

Nuvio may apply rate limits to protect the service. As a client-side safety guideline, keep each user at or below **100 requests per second** and batch operations through RPC functions when possible.

---

## Authentication

### Sign up (email/password)

```
POST /auth/v1/signup
Content-Type: application/json
apikey: <publishable_key>
```

**Request body:**

```json
{
  "email": "user@example.com",
  "password": "securepassword123"
}
```

**Response (`200`):**

```json
{
  "access_token": "eyJ...",
  "token_type": "bearer",
  "expires_in": 3600,
  "refresh_token": "abc123...",
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "created_at": "2026-01-01T00:00:00Z"
  }
}
```

Nuvio automatically creates two default metadata/subtitle integrations for each new user.

### Sign in (email/password)

```
POST /auth/v1/token?grant_type=password
Content-Type: application/json
apikey: <publishable_key>
```

**Request body:**

```json
{
  "email": "user@example.com",
  "password": "securepassword123"
}
```

**Response (`200`):** Same shape as sign-up.

### Refresh token

```
POST /auth/v1/token?grant_type=refresh_token
Content-Type: application/json
apikey: <publishable_key>
```

**Request body:**

```json
{
  "refresh_token": "your_refresh_token"
}
```

**Response (`200`):** Returns a new `access_token` and `refresh_token`.

### Sign out

```
POST /auth/v1/logout
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

### Get current user

```
GET /auth/v1/user
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Response (`200`):**

```json
{
  "id": "uuid",
  "email": "user@example.com",
  "created_at": "2026-01-01T00:00:00Z"
}
```

---

## Profiles

Current public Nuvio clients support up to **6 profiles**. Addons, library, watch progress, watch history, settings, and collections are scoped by `profile_id`.

For profile sync, `p_client_max_profiles` tells the backend which profile slots the client understands. Use `6` for current public Nuvio clients. If you omit the argument, the server retains the legacy 4-profile deletion behavior for backward compatibility.

### List profiles

```
POST /rest/v1/rpc/sync_pull_profiles
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

This request has no body.

**Response (`200`):**

```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "profile_index": 1,
    "name": "Main",
    "avatar_color_hex": "#1E88E5",
    "uses_primary_addons": false,
    "avatar_id": "avatar_cat_01",
    "avatar_url": null,
    "pin_enabled": false,
    "pin_locked_until": null,
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z"
  }
]
```

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | uuid | Profile record ID |
| `user_id` | uuid | Owner user ID |
| `profile_index` | integer | 1 to 6, unique per user in the public client surface |
| `name` | string | Display name |
| `avatar_color_hex` | string | Hex color (e.g. `#1E88E5`) |
| `uses_primary_addons` | boolean | Whether this profile shares addons with profile 1 |
| `avatar_id` | string \| null | Reference to avatar catalog entry |
| `avatar_url` | string \| null | Custom profile avatar image URL |
| `pin_enabled` | boolean | Read-only lock state for clients that need to show locked profiles |
| `pin_locked_until` | timestamp \| null | Read-only lock expiry, if the profile is temporarily locked |
| `created_at` | timestamp | Creation time |
| `updated_at` | timestamp | Last update time |

### Update profiles

This endpoint fully replaces profiles within the declared client range. It **deletes** profiles omitted from the array for slots `1..p_client_max_profiles`.

```
POST /rest/v1/rpc/sync_push_profiles
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_client_max_profiles": 6,
  "p_profiles": [
    {
      "profile_index": 1,
      "name": "Main",
      "avatar_color_hex": "#1E88E5",
      "uses_primary_addons": false,
      "avatar_id": "avatar_cat_01",
      "avatar_url": null
    },
    {
      "profile_index": 2,
      "name": "Kids",
      "avatar_color_hex": "#FF5722",
      "uses_primary_addons": true
    }
  ]
}
```

**Parameters:**

Top-level body fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `p_profiles` | array | Yes | Complete profile list for the declared profile range |
| `p_client_max_profiles` | integer | No | Highest profile slot this client manages. Use `6` for current public clients. Defaults to `4` when omitted. |

Profile object fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `profile_index` | integer | Yes | 1 to 6 in the public client surface |
| `name` | string | Yes | Display name |
| `avatar_color_hex` | string | No | Hex color code |
| `uses_primary_addons` | boolean | No | Share addons with profile 1 |
| `avatar_id` | string | No | Avatar catalog ID; existing value is kept if omitted or null and `avatar_url` is not provided |
| `avatar_url` | string \| null | No | Custom avatar image URL. When provided, it takes precedence over `avatar_id`; send `null` or `""` to clear it. |

> **Note:** This endpoint never overwrites PIN fields. PIN management is an app-specific Nuvio flow and is not part of the public API.

**Response:** `204 No Content`

### Delete profile data

This endpoint deletes **all** public data associated with a profile: addons, collections, watch progress, library, watched items, and the profile row itself.

```
POST /rest/v1/rpc/sync_delete_profile_data
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 2
}
```

**Response:** `204 No Content`

---

## Addons

Addons are compatible integration URLs tied to a user profile. Depending on the manifest, they can provide catalog, metadata, subtitle, or playback details.

### List addons

Use a direct table query:

```
GET /rest/v1/addons?select=*&profile_id=eq.1&order=sort_order
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Response (`200`):**

```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "profile_id": 1,
    "url": "https://catalog.example.com",
    "name": "Example Catalog",
    "enabled": true,
    "sort_order": 0,
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z"
  }
]
```

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | uuid | Record ID |
| `user_id` | uuid | Owner user ID |
| `profile_id` | integer | Profile index (1 to 6 in the public client surface) |
| `url` | string | Addon manifest URL |
| `name` | string \| null | Display name |
| `enabled` | boolean | Whether addon is active |
| `sort_order` | integer | Display order (0-based) |
| `created_at` | timestamp | Creation time |
| `updated_at` | timestamp | Last update time |

### Sync addons (push)

This endpoint fully replaces the specified profile's addons. It **deletes** any addon omitted from the array.

```
POST /rest/v1/rpc/sync_push_addons
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_addons": [
    {
      "url": "https://catalog.example.com/manifest.json",
      "name": "Example Catalog",
      "enabled": true,
      "sort_order": 0
    },
    {
      "url": "https://metadata.example.com/manifest.json",
      "name": "Example Metadata",
      "enabled": true,
      "sort_order": 1
    }
  ]
}
```

**Addon object fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | Addon manifest URL |
| `name` | string | No | Display name |
| `enabled` | boolean | No | Default `true` |
| `sort_order` | integer | No | Display order, default `0` |

**Deduplication:** Addons are keyed by `md5(url)` per user and profile. A push updates a row only when `name`, `enabled`, or `sort_order` has changed.

**Response:** `204 No Content`

---

## Library

The library stores content that each profile has bookmarked or favorited.

New clients should use cursor-based delta pulls with incremental upserts and deletes. The original snapshot pull and full-replace push remain available for existing integrations.

Bootstrap a client in this order:

1. Capture the current cursor with `sync_get_library_delta_cursor`.
2. Pull every `sync_pull_library` snapshot page.
3. Pull and apply every `sync_pull_library_delta` page after the captured cursor.
4. Store the highest applied `event_id` and use delta pulls for later refreshes.

Apply delta events in ascending `event_id` order. Identify each item by `(content_type, content_id)`.

### Get library

This endpoint returns one page of the current library snapshot. It remains available for bootstrap, recovery, and older clients.

```
POST /rest/v1/rpc/sync_pull_library
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_limit": 500,
  "p_offset": 0
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | `1` | Profile index |
| `p_limit` | integer | `500` | Max items per page |
| `p_offset` | integer | `0` | Pagination offset |

Results are ordered by `added_at DESC`.

Increase `p_offset` by the requested `p_limit` until the response contains fewer rows than the limit. The recommended page size for large libraries is 500 rows.

**Response (`200`):**

```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "profile_id": 1,
    "content_id": "tmdb:550",
    "content_type": "movie",
    "name": "Fight Club",
    "poster": "https://image.tmdb.org/t/p/w500/...",
    "poster_shape": "POSTER",
    "background": "https://image.tmdb.org/t/p/original/...",
    "description": "An insomniac office worker...",
    "release_info": "1999",
    "imdb_rating": 8.8,
    "genres": ["Drama", "Thriller"],
    "addon_base_url": "https://catalog.example.com",
    "added_at": 1711600000000,
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z"
  }
]
```

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `content_id` | string | Content identifier (e.g. `tmdb:550`) |
| `content_type` | string | `movie` or `series` |
| `name` | string | Content title |
| `poster` | string \| null | Poster image URL |
| `poster_shape` | string | `POSTER`, `LANDSCAPE`, or `SQUARE` |
| `background` | string \| null | Background image URL |
| `description` | string \| null | Synopsis |
| `release_info` | string \| null | Year or year range |
| `imdb_rating` | float \| null | IMDb rating |
| `genres` | string[] | Genre list |
| `addon_base_url` | string \| null | Related addon URL |
| `added_at` | integer | Epoch milliseconds when added |

### Get library delta cursor

This endpoint returns the highest library event ID available for the profile, or `0` if the profile has no events.

```
POST /rest/v1/rpc/sync_get_library_delta_cursor
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | `1` | Profile index |

**Response (`200`):**

```json
481
```

Capture this cursor before pulling the paginated snapshot. When the snapshot is complete, pull events after this cursor to apply changes that occurred during bootstrap.

### Get library delta

This endpoint returns library changes after a stored event cursor. Results are ordered by `event_id ASC`.

```
POST /rest/v1/rpc/sync_pull_library_delta
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_since_event_id": 481,
  "p_limit": 500
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | `1` | Profile index |
| `p_since_event_id` | integer | `0` | Return events with an ID greater than this cursor |
| `p_limit` | integer | `1000` | Page size, clamped to `1..1000` |

**Response (`200`):**

```json
[
  {
    "event_id": 482,
    "operation": "upsert",
    "content_id": "tmdb:550",
    "content_type": "movie",
    "name": "Fight Club",
    "poster": "https://image.tmdb.org/t/p/w500/...",
    "poster_shape": "POSTER",
    "background": "https://image.tmdb.org/t/p/original/...",
    "description": "An insomniac office worker...",
    "release_info": "1999",
    "imdb_rating": 8.8,
    "genres": ["Drama", "Thriller"],
    "addon_base_url": "https://catalog.example.com",
    "added_at": 1711600000000
  },
  {
    "event_id": 483,
    "operation": "delete",
    "content_id": "tmdb:1396",
    "content_type": "series",
    "name": "Breaking Bad",
    "poster": null,
    "poster_shape": "POSTER",
    "background": null,
    "description": null,
    "release_info": null,
    "imdb_rating": null,
    "genres": [],
    "addon_base_url": null,
    "added_at": 1711500000000
  }
]
```

| Field | Type | Description |
|-------|------|-------------|
| `event_id` | integer | Monotonically increasing cursor |
| `operation` | string | `upsert` or `delete` |
| Remaining fields | library item fields | Current item data for an upsert; use the identity fields for a delete |

After each page, save the highest `event_id`. Request the next page from that cursor until a page contains fewer rows than `p_limit`.

### Upsert library items

This endpoint inserts or updates only the supplied items. It preserves existing library items that are not in the request.

```
POST /rest/v1/rpc/sync_push_library_items
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_origin_client_id": "my-client-installation-01",
  "p_items": [
    {
      "content_id": "tmdb:550",
      "content_type": "movie",
      "name": "Fight Club",
      "poster": "https://image.tmdb.org/t/p/w500/...",
      "poster_shape": "POSTER",
      "background": "https://image.tmdb.org/t/p/original/...",
      "description": "An insomniac office worker...",
      "release_info": "1999",
      "imdb_rating": 8.8,
      "genres": ["Drama", "Thriller"],
      "addon_base_url": "https://catalog.example.com",
      "added_at": 1711600000000
    }
  ]
}
```

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `p_profile_id` | integer | No | `1` | Profile index |
| `p_items` | object[] | Yes | Not applicable | Items to insert or update |
| `p_origin_client_id` | string \| null | No | `null` | Stable installation identifier attached to emitted sync events |

**Library item fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content_id` | string | Yes | Content identifier |
| `content_type` | string | Yes | `movie` or `series` |
| `name` | string | No | Title |
| `poster` | string | No | Poster URL |
| `poster_shape` | string | No | Default `POSTER` |
| `background` | string | No | Background URL |
| `description` | string | No | Synopsis |
| `release_info` | string | No | Year |
| `imdb_rating` | float | No | IMDb rating |
| `genres` | string[] | No | Genre list |
| `addon_base_url` | string | No | Related addon |
| `added_at` | integer | No | Epoch milliseconds |

Send large writes in batches. Nuvio clients use at most 500 items per request.

**Response:** `204 No Content`

### Delete library items

This endpoint deletes only the supplied `(content_id, content_type)` keys. It preserves items not listed in the request.

```
POST /rest/v1/rpc/sync_delete_library_items
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_origin_client_id": "my-client-installation-01",
  "p_keys": [
    {
      "content_id": "tmdb:550",
      "content_type": "movie"
    }
  ]
}
```

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `p_profile_id` | integer | No | `1` | Profile index |
| `p_keys` | object[] | Yes | Not applicable | Exact library identities to delete |
| `p_origin_client_id` | string \| null | No | `null` | Stable installation identifier attached to emitted sync events |

Send large deletes in batches. Nuvio clients use at most 500 keys per request.

**Response:** `204 No Content`

### Sync library (push): legacy full replace

This compatibility endpoint performs a full replace for existing clients. It **deletes** items omitted from the array for the specified profile.

New clients should use `sync_push_library_items` and `sync_delete_library_items` instead. Do not send partial state to this legacy endpoint.

```
POST /rest/v1/rpc/sync_push_library
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_items": [
    {
      "content_id": "tmdb:550",
      "content_type": "movie",
      "name": "Fight Club",
      "poster": "https://image.tmdb.org/t/p/w500/...",
      "poster_shape": "POSTER",
      "background": "https://image.tmdb.org/t/p/original/...",
      "description": "An insomniac office worker...",
      "release_info": "1999",
      "imdb_rating": 8.8,
      "genres": ["Drama", "Thriller"],
      "addon_base_url": "https://catalog.example.com",
      "added_at": 1711600000000
    }
  ]
}
```

**Response:** `204 No Content`

---

## Watch progress

Watch progress stores playback positions for the "continue watching" feature. Pushes use a **non-destructive merge**: they upsert entries and do not delete entries omitted from the payload.

### Get watch progress

```
POST /rest/v1/rpc/sync_pull_watch_progress
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_since_last_watched": 1711600000000,
  "p_limit": 200
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | `1` | Profile index |
| `p_since_last_watched` | integer | `null` | Optional epoch-millisecond cursor. When set, returns rows with `last_watched` greater than this value. |
| `p_limit` | integer | `200` | Optional row limit. Without `p_since_last_watched`, the server caps the result at 200 rows. |

Without `p_since_last_watched`, this endpoint returns the latest progress entries for the profile in `last_watched DESC` order. Use the delta endpoints below when you need delete events or a stable event cursor.

**Response (`200`):**

```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "profile_id": 1,
    "content_id": "tmdb:550",
    "content_type": "movie",
    "video_id": "tmdb:550",
    "season": null,
    "episode": null,
    "progress_key": "tmdb:550",
    "position": 3600000,
    "duration": 7920000,
    "last_watched": 1711600000000
  },
  {
    "id": "uuid",
    "user_id": "uuid",
    "profile_id": 1,
    "content_id": "tmdb:1396",
    "content_type": "series",
    "video_id": "tmdb:1396:1:1",
    "season": 1,
    "episode": 1,
    "progress_key": "tmdb:1396_s1e1",
    "position": 1800000,
    "duration": 3480000,
    "last_watched": 1711600000000
  }
]
```

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `content_id` | string | Content identifier (e.g. `tmdb:550`) |
| `content_type` | string | `movie` or `series` |
| `video_id` | string | Specific playback item ID |
| `season` | integer \| null | Season number (series only) |
| `episode` | integer \| null | Episode number (series only) |
| `progress_key` | string | Unique key (see [Progress Key Format](#progress-key-format)) |
| `position` | integer | Playback position in milliseconds |
| `duration` | integer | Total duration in milliseconds |
| `last_watched` | integer | Epoch milliseconds of last playback |

### Get watch progress delta

This endpoint returns upsert and delete events after a stored event cursor. Use it for incremental sync after the client completes its initial snapshot pull.

```
POST /rest/v1/rpc/sync_pull_watch_progress_delta
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_since_event_id": 12345,
  "p_limit": 1000
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | `1` | Profile index |
| `p_since_event_id` | integer | `0` | Return events with `event_id` greater than this value |
| `p_limit` | integer | `1000` | Max events to return, capped at `1000` |

**Response (`200`):**

```json
[
  {
    "event_id": 12346,
    "operation": "upsert",
    "progress_key": "tmdb:550",
    "content_id": "tmdb:550",
    "content_type": "movie",
    "video_id": "tmdb:550",
    "season": null,
    "episode": null,
    "position": 3600000,
    "duration": 7920000,
    "last_watched": 1711600000000
  },
  {
    "event_id": 12347,
    "operation": "delete",
    "progress_key": "tmdb:1396_s1e1",
    "content_id": "tmdb:1396",
    "content_type": "series",
    "video_id": "tmdb:1396:1:1",
    "season": 1,
    "episode": 1,
    "position": 1800000,
    "duration": 3480000,
    "last_watched": 1711600000000
  }
]
```

Apply events in ascending `event_id` order and save the highest `event_id` you process.

### Get watch progress delta cursor

This endpoint returns the current maximum watch-progress event ID for the profile. After a full snapshot sync, call it and save the returned value as the next `p_since_event_id`.

```
POST /rest/v1/rpc/sync_get_watch_progress_delta_cursor
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1
}
```

**Response (`200`):**

```json
12347
```

### Sync watch progress (push)

This endpoint uses a non-destructive merge. It upserts entries and does **not** delete entries omitted from the request.

```
POST /rest/v1/rpc/sync_push_watch_progress
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_entries": [
    {
      "content_id": "tmdb:550",
      "content_type": "movie",
      "video_id": "tmdb:550",
      "position": 3600000,
      "duration": 7920000,
      "last_watched": 1711600000000
    },
    {
      "content_id": "tmdb:1396",
      "content_type": "series",
      "video_id": "tmdb:1396:1:1",
      "season": 1,
      "episode": 1,
      "position": 1800000,
      "duration": 3480000,
      "last_watched": 1711600000000
    }
  ]
}
```

**Watch progress entry fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content_id` | string | Yes | Content identifier |
| `content_type` | string | Yes | `movie` or `series` |
| `video_id` | string | Yes | Playback item identifier |
| `season` | integer | No | Season number (for series) |
| `episode` | integer | No | Episode number (for series) |
| `position` | integer | Yes | Playback position in ms |
| `duration` | integer | Yes | Total duration in ms |
| `last_watched` | integer | Yes | Epoch milliseconds |

> **Completion handling:** When an entry has `duration >= 60000` and `position` is at least 90% of `duration`, the server also upserts a matching watched-history item. The server may ignore tiny progress changes to reduce write volume.

**Response:** `204 No Content`

### Delete watch progress (single)

```
POST /rest/v1/rpc/sync_delete_watch_progress
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_progress_key": "tmdb:550",
  "p_profile_id": 1
}
```

**Response:** `204 No Content`

### Delete watch progress (batch)

```
POST /rest/v1/rpc/sync_delete_watch_progress
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_keys": ["tmdb:550", "tmdb:1396_s1e1"],
  "p_profile_id": 1
}
```

**Response:** `204 No Content`

---

## Watch history

Watch history records content that a profile has watched. Pushes use a **non-destructive merge**.

### Get watch history

```
POST /rest/v1/rpc/sync_pull_watched_items
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_page": 1,
  "p_page_size": 500
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | `1` | Profile index |
| `p_page` | integer | `1` | Page number (1-indexed) |
| `p_page_size` | integer | `100000` | Items per page |

Results are ordered by `watched_at DESC`.

**Response (`200`):**

```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "profile_id": 1,
    "content_id": "tmdb:550",
    "content_type": "movie",
    "title": "Fight Club",
    "season": null,
    "episode": null,
    "watched_at": 1711600000000,
    "created_at": "2026-01-01T00:00:00Z"
  }
]
```

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `content_id` | string | Content identifier |
| `content_type` | string | `movie` or `series` |
| `title` | string | Content title |
| `season` | integer \| null | Season number (series only) |
| `episode` | integer \| null | Episode number (series only) |
| `watched_at` | integer | Epoch milliseconds |

### Get watch history delta

This endpoint returns watched-history upsert and delete events after a stored event cursor. Use it for incremental sync after the client completes its initial snapshot pull.

```
POST /rest/v1/rpc/sync_pull_watched_items_delta
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_since_event_id": 98765,
  "p_limit": 1000
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | `1` | Profile index |
| `p_since_event_id` | integer | `0` | Return events with `event_id` greater than this value |
| `p_limit` | integer | `1000` | Max events to return, capped at `1000` |

**Response (`200`):**

```json
[
  {
    "event_id": 98766,
    "operation": "upsert",
    "content_id": "tmdb:550",
    "content_type": "movie",
    "title": "Fight Club",
    "season": null,
    "episode": null,
    "watched_at": 1711600000000
  },
  {
    "event_id": 98767,
    "operation": "delete",
    "content_id": "tmdb:1396",
    "content_type": "series",
    "title": "Breaking Bad S01E01",
    "season": 1,
    "episode": 1,
    "watched_at": 1711600000000
  }
]
```

Apply events in ascending `event_id` order and save the highest `event_id` you process.

### Get watch history delta cursor

This endpoint returns the current maximum watched-history event ID for the profile. After a full snapshot sync, call it and save the returned value as the next `p_since_event_id`.

```
POST /rest/v1/rpc/sync_get_watched_items_delta_cursor
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1
}
```

**Response (`200`):**

```json
98767
```

### Sync watch history (push)

This endpoint uses a non-destructive merge and only upserts entries.

```
POST /rest/v1/rpc/sync_push_watched_items
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_items": [
    {
      "content_id": "tmdb:550",
      "content_type": "movie",
      "title": "Fight Club",
      "watched_at": 1711600000000
    },
    {
      "content_id": "tmdb:1396",
      "content_type": "series",
      "title": "Breaking Bad S01E01",
      "season": 1,
      "episode": 1,
      "watched_at": 1711600000000
    }
  ]
}
```

**Watched item fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content_id` | string | Yes | Content identifier |
| `content_type` | string | Yes | `movie` or `series` |
| `title` | string | No | Title |
| `season` | integer | No | Season number |
| `episode` | integer | No | Episode number |
| `watched_at` | integer | Yes | Epoch milliseconds |

**Response:** `204 No Content`

### Delete watch history

```
POST /rest/v1/rpc/sync_delete_watched_items
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_keys": [
    { "content_id": "tmdb:550" },
    { "content_id": "tmdb:1396", "season": 1, "episode": 1 }
  ]
}
```

**Key object fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content_id` | string | Yes | Content identifier |
| `season` | integer | No | Required for series episodes |
| `episode` | integer | No | Required for series episodes |

**Response:** `204 No Content`

---

## Profile settings

Profile settings use a generic JSON key-value store for each profile. Examples include themes, player preferences, and UI options. Supplying `p_platform` gives the settings a platform namespace.

### Get settings

```
POST /rest/v1/rpc/sync_pull_profile_settings_blob
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_platform": "tv"
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | Required | Profile index |
| `p_platform` | string | `tv` | Optional platform namespace. Recommended for new clients. |

**Response (`200`):**

```json
[
  {
    "profile_id": 1,
    "settings_json": {
      "theme": "dark",
      "player_quality": "auto",
      "subtitle_language": "en",
      "auto_play_next": true
    },
    "updated_at": "2026-01-01T00:00:00Z"
  }
]
```

The `settings_json` field accepts any JSON object. The server does not enforce a schema; your application defines the structure.

### Update settings

This atomic upsert fully replaces the settings blob for the profile.

```
POST /rest/v1/rpc/sync_push_profile_settings_blob
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_platform": "tv",
  "p_settings_json": {
    "theme": "dark",
    "player_quality": "auto",
    "subtitle_language": "en",
    "auto_play_next": true
  }
}
```

**Response:** `204 No Content`

---

## Home catalog settings

Nuvio stores each profile's home catalog configuration as a JSON blob. It is separate from the general profile settings blob, which lets clients sync home layout and catalog preferences independently.

### Get home catalog settings

```
POST /rest/v1/rpc/sync_pull_home_catalog_settings
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_platform": "tv"
}
```

**Response (`200`):**

```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "profile_id": 1,
    "platform": "tv",
    "settings_json": {
      "rows": [],
      "hidden_catalogs": []
    },
    "updated_at": "2026-01-01T00:00:00Z"
  }
]
```

The `settings_json` payload is application-defined JSON.

### Update home catalog settings

```
POST /rest/v1/rpc/sync_push_home_catalog_settings
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_platform": "tv",
  "p_settings_json": {
    "rows": [],
    "hidden_catalogs": []
  }
}
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `p_profile_id` | integer | Required | Profile index |
| `p_platform` | string | `tv` | Optional platform namespace. Recommended for new clients. |
| `p_settings_json` | object | Required | Full home catalog settings payload |

**Response:** `204 No Content`

---

## Collections

Each profile stores its custom content collections in a JSON blob.

### Get collections

```
POST /rest/v1/rpc/sync_pull_collections
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1
}
```

**Response (`200`):**

```json
[
  {
    "profile_id": 1,
    "collections_json": [
      {
        "id": "collection-1",
        "title": "Weekend Picks",
        "backdropImageUrl": "https://cdn.example.com/backdrops/weekend.jpg",
        "pinToTop": true,
        "viewMode": "TABBED_GRID",
        "showAllTab": true,
        "folders": [
          {
            "id": "folder-1",
            "title": "Sci-Fi",
            "coverImageUrl": "https://cdn.example.com/folders/scifi.jpg",
            "coverEmoji": "🚀",
            "tileShape": "LANDSCAPE",
            "hideTitle": false,
            "catalogSources": [
              {
                "addonId": "com.example.catalog",
                "type": "movie",
                "catalogId": "top"
              }
            ]
          }
        ]
      }
    ],
    "updated_at": "2026-01-01T00:00:00Z"
  }
]
```

**Collection JSON fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique collection ID |
| `title` | string | Collection name |
| `backdropImageUrl` | string | Optional backdrop image |
| `pinToTop` | boolean | Pin to top of home screen |
| `viewMode` | string | `TABBED_GRID`, `ROWS`, or `FOLLOW_LAYOUT` |
| `showAllTab` | boolean | Show "All" tab in tabbed view |
| `folders` | array | Array of folder objects |

**Folder object:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique folder ID |
| `title` | string | Folder name |
| `coverImageUrl` | string | Optional cover image |
| `coverEmoji` | string | Optional emoji icon |
| `tileShape` | string | `POSTER`, `LANDSCAPE`, or `SQUARE` |
| `hideTitle` | boolean | Hide the tile title text |
| `catalogSources` | array | Array of catalog references |

**Catalog reference:**

| Field | Type | Description |
|-------|------|-------------|
| `addonId` | string | Addon identifier |
| `type` | string | Content type (e.g. `movie`, `series`) |
| `catalogId` | string | Catalog identifier |

### Update collections

This endpoint fully replaces the profile's collections blob. Send an empty array (`[]`) to clear it.

```
POST /rest/v1/rpc/sync_push_collections
Content-Type: application/json
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Request body:**

```json
{
  "p_profile_id": 1,
  "p_collections_json": [
    {
      "id": "collection-1",
      "title": "Weekend Picks",
      "viewMode": "TABBED_GRID",
      "folders": []
    }
  ]
}
```

**Response:** `204 No Content`

---

## Avatars

The avatar catalog lists the profile avatars currently available. Clients should also support `avatar_color_hex` and `avatar_url` from profile rows because a custom avatar URL does not require a catalog entry.

### List avatars

This endpoint does not require authentication.

```
POST /rest/v1/rpc/get_avatar_catalog
apikey: <publishable_key>
```

**Response (`200`):**

```json
[
  {
    "id": "avatar_cat_01",
    "display_name": "Cool Cat",
    "storage_path": "avatars/avatar_cat_01.png",
    "category": "character",
    "sort_order": 0,
    "is_active": true,
    "bg_color": "#FFB74D",
    "created_at": "2026-01-01T00:00:00Z"
  }
]
```

The response may be an empty array if no catalog avatars are currently published.

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Avatar ID for use in profile `avatar_id` |
| `display_name` | string | Human-readable name |
| `storage_path` | string | Storage path for the image |
| `category` | string | Category (e.g. `character`) |
| `sort_order` | integer | Display order |
| `is_active` | boolean | Whether available for selection |
| `bg_color` | string \| null | Suggested background color |

---

## Supporter Wall

This public website endpoint returns the Top and Recent views of The Wall in one request. It does not require authentication and exposes only the same public name, avatar, tier, and member-since fields shown on the Support page.
The response permits cross-origin reads, so it can be used directly by other public Nuvio sites.

```
GET https://nuvio.tv/api/supporters/wall?limit=48&offset=0
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | `48` | Members per list. Clamped to `1`–`100`. |
| `offset` | integer | `0` | Shared pagination offset for both lists. |

**Response (`200`):**

```json
{
  "top": {
    "members": [
      {
        "displayName": "Nuvio Fan",
        "avatarUrl": "https://example.com/avatar.png",
        "membershipLevel": "SUPPORTER_PLUS",
        "supporterSince": "2026-08-20T00:00:00Z"
      }
    ],
    "totalCount": 24
  },
  "recent": {
    "members": [],
    "totalCount": 24
  },
  "pagination": {
    "limit": 48,
    "offset": 0
  }
}
```

`supporterSince` is `null` for one-time memberships. Members who opt out of The Wall are excluded from both lists. Payment amounts, account IDs, emails, and provider details are never returned.

---

## Sync overview

This endpoint returns core sync data counts for each profile. Clients can use the counts in dashboards and status displays.

```
POST /rest/v1/rpc/get_sync_overview
Authorization: Bearer <access_token>
apikey: <publishable_key>
```

**Response (`200`):**

```json
{
  "addons": { "1": 5, "2": 3 },
  "library_items": { "1": 42, "2": 10 },
  "watch_progress": { "1": 150, "2": 30 },
  "watched_items": { "1": 200, "2": 50 },
  "profiles": {
    "1": { "name": "Main", "color": "#1E88E5" },
    "2": { "name": "Kids", "color": "#FF5722" }
  }
}
```

Each key in the data objects is a profile index. The `profiles` entry includes the display name and color. This overview currently documents addons, library items, watch progress, watched items, and profiles only; it does not include collections, profile settings, or home catalog settings.

---

## Health check

Use this endpoint to check the API and database health. It does not require authentication.

```
GET /functions/v1/health-check
```

**Response (`200`):**

```json
{
  "status": "healthy",
  "database": "connected",
  "latency_ms": 45,
  "timestamp": "2026-04-07T12:00:00.000Z"
}
```

| Status | Meaning |
|--------|---------|
| `healthy` | Everything operational |
| `slow` | Database responding slowly |
| `degraded` | Partial issues |
| `down` | Database unreachable |

You can use the lightweight RPC ping instead:

```
POST /rest/v1/rpc/health_ping
apikey: <publishable_key>
```

The ping returns `true` if the database is reachable.

---

## Concepts

### Profile system

Current public Nuvio clients support up to **6 profiles**, indexed 1 to 6. The `profile_id` parameter scopes every documented data resource to a profile, including addons, library, watch progress, watch history, settings, and collections.

Profiles can optionally share addons with profile 1 by setting `uses_primary_addons` to `true`.

### Sync strategies

The API supports four sync strategies:

| Strategy | Used by | Behavior |
|----------|---------|----------|
| **Full replace** | Addons, Profiles, Collections, legacy Library | The payload represents the **complete** current state. Items not present in the push payload are **deleted** server-side. Profile replacement is scoped to `1..p_client_max_profiles`. |
| **Incremental item mutations** | Library | Upsert and delete calls affect only the supplied identities. Existing items not included in a request are preserved. |
| **Atomic blob upsert** | Profile Settings, Home Catalog Settings | The JSON payload fully replaces one `(user, profile, platform)` blob. |
| **Non-destructive merge** | Watch Progress, Watch History | Push only upserts records. Existing records not in the payload are **preserved**. Use explicit delete endpoints to remove individual records. |

> **Caution with full-replace endpoints:** Always send the complete list when pushing addons, profiles, collections, or calling the legacy `sync_push_library` endpoint. Sending a partial list will delete everything not included.

### Incremental sync

Library, watch progress, and watch history support event-cursor delta endpoints. Bootstrap a client in this order:

1. Call the matching `sync_get_*_delta_cursor` RPC and capture the returned event ID.
2. Pull the full snapshot, including every page for paginated resources.
3. Call the matching `sync_pull_*_delta` RPC with the captured event ID.
4. Apply events in ascending `event_id` order and persist the highest processed event ID.

Use snapshot pulls for bootstrap or recovery. Use delta pulls for ongoing sync, especially when a client needs to observe deletions.

### Progress key format

Watch progress entries are deduplicated by a `progress_key`:

| Content Type | Format | Example |
|---|---|---|
| Movie | `{content_id}` | `tmdb:550` |
| Series episode | `{content_id}_s{season}e{episode}` | `tmdb:1396_s1e1` |

The server uses a trigger to normalize progress keys. Provide `content_id`, `season`, and `episode`; the server computes the progress key.

---

## Client libraries

The API is Supabase-compatible, so you can call it with these [Supabase client libraries](https://supabase.com/docs/reference):

| Language | Library |
|----------|---------|
| JavaScript/TypeScript | [`@supabase/supabase-js`](https://github.com/supabase/supabase-js) |
| Python | [`supabase-py`](https://github.com/supabase-community/supabase-py) |
| Kotlin | [`supabase-kt`](https://github.com/supabase-community/supabase-kt) |
| Swift | [`supabase-swift`](https://github.com/supabase-community/supabase-swift) |
| Dart/Flutter | [`supabase-flutter`](https://github.com/supabase-community/supabase-flutter) |
| C# | [`supabase-csharp`](https://github.com/supabase-community/supabase-csharp) |

### Quick start (JavaScript)

```javascript
import { createClient } from '@supabase/supabase-js'

const supabase = createClient(
  'https://api.nuvio.tv',
  '<publishable_key>'
)

// Sign in
const { data: auth } = await supabase.auth.signInWithPassword({
  email: 'user@example.com',
  password: 'password'
})

// Get addons for profile 1
const { data: addons } = await supabase
  .from('addons')
  .select('*')
  .eq('profile_id', 1)
  .order('sort_order')

// Push addons
await supabase.rpc('sync_push_addons', {
  p_profile_id: 1,
  p_addons: [
    { url: 'https://catalog.example.com', name: 'Example Catalog', enabled: true, sort_order: 0 }
  ]
})

// Capture the library cursor before snapshot bootstrap
const { data: libraryCursor } = await supabase.rpc('sync_get_library_delta_cursor', {
  p_profile_id: 1
})

// Pull every snapshot page by increasing p_offset until a short page is returned
const { data: libraryPage } = await supabase.rpc('sync_pull_library', {
  p_profile_id: 1,
  p_limit: 500,
  p_offset: 0
})

// Reconcile changes that happened during bootstrap
const { data: libraryDelta } = await supabase.rpc('sync_pull_library_delta', {
  p_profile_id: 1,
  p_since_event_id: libraryCursor,
  p_limit: 500
})

// Incrementally upsert without replacing the rest of the library
await supabase.rpc('sync_push_library_items', {
  p_profile_id: 1,
  p_origin_client_id: 'my-client-installation-01',
  p_items: [
    {
      content_id: 'tmdb:550',
      content_type: 'movie',
      name: 'Fight Club',
      added_at: Date.now()
    }
  ]
})

// Get watch progress
const { data: progress } = await supabase.rpc('sync_pull_watch_progress', {
  p_profile_id: 1
})

// Get sync overview
const { data: overview } = await supabase.rpc('get_sync_overview')
```

### Quick start (Python)

```python
from supabase import create_client

supabase = create_client(
    "https://api.nuvio.tv",
    "<publishable_key>"
)

# Sign in
auth = supabase.auth.sign_in_with_password({
    "email": "user@example.com",
    "password": "password"
})

# Get addons
addons = supabase.table("addons") \
    .select("*") \
    .eq("profile_id", 1) \
    .order("sort_order") \
    .execute()

# Push watch progress
supabase.rpc("sync_push_watch_progress", {
    "p_profile_id": 1,
    "p_entries": [
        {
            "content_id": "tmdb:550",
            "content_type": "movie",
            "video_id": "tmdb:550",
            "position": 3600000,
            "duration": 7920000,
            "last_watched": 1711600000000
        }
    ]
}).execute()
```

### Quick start (cURL)

```bash
# Sign in
curl -X POST 'https://api.nuvio.tv/auth/v1/token?grant_type=password' \
  -H 'apikey: <publishable_key>' \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password"}'

# Get addons
curl 'https://api.nuvio.tv/rest/v1/addons?select=*&profile_id=eq.1&order=sort_order' \
  -H 'Authorization: Bearer <access_token>' \
  -H 'apikey: <publishable_key>'

# Push addons via RPC
curl -X POST 'https://api.nuvio.tv/rest/v1/rpc/sync_push_addons' \
  -H 'Authorization: Bearer <access_token>' \
  -H 'apikey: <publishable_key>' \
  -H 'Content-Type: application/json' \
  -d '{
    "p_profile_id": 1,
    "p_addons": [
      {"url":"https://catalog.example.com","name":"Example Catalog","enabled":true,"sort_order":0}
    ]
  }'

# Health check (no auth needed)
curl 'https://api.nuvio.tv/functions/v1/health-check'
```

---

## Complete RPC reference

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `rpc/sync_pull_profiles` | POST | Yes | List all profiles |
| `rpc/sync_push_profiles` | POST | Yes | Full replace profiles within the declared client profile range |
| `rpc/sync_delete_profile_data` | POST | Yes | Delete all profile data |
| `rpc/sync_push_addons` | POST | Yes | Full replace addons |
| `rpc/sync_pull_library` | POST | Yes | Paginated library pull |
| `rpc/sync_get_library_delta_cursor` | POST | Yes | Current library event cursor |
| `rpc/sync_pull_library_delta` | POST | Yes | Incremental library events |
| `rpc/sync_push_library_items` | POST | Yes | Incrementally upsert library items |
| `rpc/sync_delete_library_items` | POST | Yes | Delete explicit library identities |
| `rpc/sync_push_library` | POST | Yes | Legacy full replace library |
| `rpc/sync_pull_watch_progress` | POST | Yes | Latest or timestamp-filtered watch progress |
| `rpc/sync_pull_watch_progress_delta` | POST | Yes | Incremental progress events |
| `rpc/sync_get_watch_progress_delta_cursor` | POST | Yes | Current progress event cursor |
| `rpc/sync_push_watch_progress` | POST | Yes | Upsert watch progress |
| `rpc/sync_delete_watch_progress` | POST | Yes | Delete progress entries |
| `rpc/sync_pull_watched_items` | POST | Yes | Paginated history pull |
| `rpc/sync_pull_watched_items_delta` | POST | Yes | Incremental history events |
| `rpc/sync_get_watched_items_delta_cursor` | POST | Yes | Current history event cursor |
| `rpc/sync_push_watched_items` | POST | Yes | Upsert watch history |
| `rpc/sync_delete_watched_items` | POST | Yes | Delete history entries |
| `rpc/sync_pull_profile_settings_blob` | POST | Yes | Get profile settings |
| `rpc/sync_push_profile_settings_blob` | POST | Yes | Update profile settings |
| `rpc/sync_pull_home_catalog_settings` | POST | Yes | Get home catalog settings |
| `rpc/sync_push_home_catalog_settings` | POST | Yes | Update home catalog settings |
| `rpc/sync_pull_collections` | POST | Yes | Get collections |
| `rpc/sync_push_collections` | POST | Yes | Update collections |
| `rpc/get_avatar_catalog` | POST | No | List available avatars |
| `rpc/get_sync_overview` | POST | Yes | Data count summary |
| `rpc/health_ping` | POST | No | Database ping |
| `functions/v1/health-check` | GET | No | Full health check |

---

## Changelog

### v1.2, July 29, 2026

- Added incremental library upsert and explicit delete endpoints
- Added library delta cursor and paginated delta pull endpoints
- Documented cursor-based snapshot bootstrap and 500-item client batching
- Kept the legacy library snapshot and full-replace APIs for existing integrations

### v1.1, June 11, 2026

- Clarified the documentation scope: this reference covers the officially supported public API endpoints
- Updated profile docs for 6-profile public clients, `p_client_max_profiles`, `avatar_url`, and read-only lock-state fields
- Documented platform-aware profile settings and home catalog settings sync
- Updated watch progress pull behavior and added progress/history delta cursor endpoints for incremental sync
- Corrected watched-completion behavior and added caveats for sync overview and avatar catalog responses

### v1.0, April 2026

- Initial public API documentation release
- Covers the public integration surface for auth, profiles, addons, library, watch progress, watch history, collections, settings, overview, and health checks
