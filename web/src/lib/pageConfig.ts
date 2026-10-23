/** The runtime settings the server puts in the page it serves
 *  (`internal/static`), as a `<script type="application/json" id="uno-config">`
 *  block. `per-account` means each Nuvio account brings its own TMDB key,
 *  which the picker asks for. `community` is the server's `UNO_COMMUNITY`:
 *  Community as it is, without the image and video addresses of sites the
 *  server doesn't trust, without any, or off. */
export interface PageConfig {
  nuvioBaseURL: string
  nuvioPublishableKey: string
  tmdb_key_mode: 'shared' | 'per-account'
  community: 'on' | 'trusted-images' | 'no-images' | 'off'
}

/** This server's `UNO_COMMUNITY`, from the page's config block, or `on` for a
 *  page that has none: a component rendered on its own. */
export function communitySetting(): PageConfig['community'] {
  if (typeof document === 'undefined' || !document.getElementById('uno-config')) return 'on'
  return pageConfig().community
}

/** The page's `uno-config` block. Read at call time, so a test (or a page)
 *  can set it after this module has loaded. */
export function pageConfig(): PageConfig {
  const block = document.getElementById('uno-config')
  if (!block?.textContent) {
    throw new Error("The page has no Uno settings block. It comes from Uno's server, so start it and reload.")
  }
  return JSON.parse(block.textContent) as PageConfig
}
