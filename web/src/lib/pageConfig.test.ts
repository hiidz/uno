// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { communitySetting } from './pageConfig'

function setBlock(community: string) {
  const block = document.createElement('script')
  block.type = 'application/json'
  block.id = 'uno-config'
  block.textContent = JSON.stringify({ nuvioBaseURL: '', nuvioPublishableKey: '', tmdb_key_mode: 'shared', community })
  document.head.append(block)
}

describe('communitySetting', () => {
  afterEach(() => document.getElementById('uno-config')?.remove())

  it('reads the server’s setting from the page', () => {
    setBlock('off')
    expect(communitySetting()).toBe('off')
  })

  it('reads as on for a page with no settings block', () => {
    expect(communitySetting()).toBe('on')
  })
})
