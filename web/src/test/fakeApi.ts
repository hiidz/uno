/**
 * A stand-in for `apiFetch` (`vi.mock('@/api/client', …)`) answering by
 * `METHOD /path`: a value is its JSON body with a 200, a function gets the
 * URL and the request and returns a body or a whole `Response`. A request
 * no route names stays pending, so a test only has to answer what it is
 * about. `calls` records every request, as `METHOD /path?query`.
 */
export type FakeRoute = unknown | ((url: URL, init: RequestInit) => unknown)

export function fakeApi(routes: Record<string, FakeRoute>) {
  const calls: string[] = []
  async function apiFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
    const url = new URL(String(input), 'http://uno.test')
    const method = init.method ?? 'GET'
    calls.push(`${method} ${url.pathname}${url.search}`)
    const route = routes[`${method} ${url.pathname}`]
    if (route === undefined) return new Promise(() => {})
    const value = typeof route === 'function' ? await route(url, init) : route
    if (value instanceof Response) return value
    return new Response(JSON.stringify(value), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }
  return { apiFetch, calls }
}

/** A plain-text error answer, the way the Go handlers write one. */
export function failWith(status: number, message: string): Response {
  return new Response(message, { status })
}

/** The 404 `requireProfile` answers for a profile slot never selected. */
export function profileNotFound(): Response {
  return Response.json({ error: 'profile not found', code: 'profile_not_found' }, { status: 404 })
}
