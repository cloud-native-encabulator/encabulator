/**
 * Sessions.
 *
 * `POST /bff/login` calls the unilateral phase detractor, which issues an RS256
 * JWT. The BFF stores that token in a signed, `httpOnly`, `SameSite=Strict`
 * cookie and never hands it to JavaScript. Every request decodes the cookie to
 * learn the subject and roles.
 *
 * An unauthenticated visitor is not rejected: the design says they are `guest`
 * with the `viewer` role, so they can watch the factory run but cannot touch it.
 * The role check that enforces that lives in `roles.ts` and runs on the server.
 *
 * Why decoding without verifying the signature is correct here: the token
 * reached us over a trusted channel (our own call to the detractor) and is
 * stored in a cookie signed with `SESSION_SECRET`, so its integrity is already
 * established. The token's own RS256 signature is verified where it matters, by
 * the dingle-arm gateway, on every storefront call.
 */

export type Role = 'customer' | 'viewer' | 'operator' | 'admin'

export const ALL_ROLES: Role[] = ['customer', 'viewer', 'operator', 'admin']

export const GUEST: Session = {
  authenticated: false,
  username: 'guest',
  displayName: 'Guest',
  roles: ['viewer'],
  token: null,
  expiresAt: null,
}

export interface Session {
  authenticated: boolean
  username: string
  displayName: string
  roles: Role[]
  /** The detractor's JWT. Only used for storefront calls through the gateway. */
  token: string | null
  /** RFC 3339, or null for a guest. */
  expiresAt: string | null
}

interface JwtClaims {
  sub?: unknown
  roles?: unknown
  name?: unknown
  displayName?: unknown
  exp?: unknown
}

function decodeSegment(segment: string): unknown {
  const padded = segment.replace(/-/g, '+').replace(/_/g, '/')
  const json = Buffer.from(padded, 'base64').toString('utf8')
  return JSON.parse(json)
}

export function parseRoles(raw: unknown): Role[] {
  const values: string[] = Array.isArray(raw)
    ? raw.filter((v): v is string => typeof v === 'string')
    : typeof raw === 'string'
      ? raw.split(',')
      : []
  const roles = values
    .map((v) => v.trim().toLowerCase())
    .filter((v): v is Role => (ALL_ROLES as string[]).includes(v))
  return [...new Set(roles)]
}

/**
 * Turn a detractor JWT into a session, or null when the token is malformed or
 * expired. `now` is injectable so the expiry rule is testable.
 */
export function sessionFromToken(
  token: string,
  now: Date = new Date(),
  fallbackDisplayName?: string,
): Session | null {
  const parts = token.split('.')
  if (parts.length !== 3) return null
  let claims: JwtClaims
  try {
    claims = decodeSegment(parts[1] as string) as JwtClaims
  } catch {
    return null
  }
  if (claims === null || typeof claims !== 'object') return null
  const sub = typeof claims.sub === 'string' ? claims.sub : null
  if (sub === null || sub === '') return null

  const expSeconds = typeof claims.exp === 'number' ? claims.exp : null
  if (expSeconds === null) return null
  const expiresAt = new Date(expSeconds * 1000)
  if (expiresAt.getTime() <= now.getTime()) return null

  const roles = parseRoles(claims.roles)
  const display =
    (typeof claims.displayName === 'string' ? claims.displayName : null) ??
    (typeof claims.name === 'string' ? claims.name : null) ??
    fallbackDisplayName ??
    sub

  return {
    authenticated: true,
    username: sub,
    displayName: display,
    // A signed-in user with no roles at all can still watch.
    roles: roles.length > 0 ? roles : ['viewer'],
    token,
    expiresAt: expiresAt.toISOString(),
  }
}

/** Seconds until the token expires, floored at zero. */
export function secondsUntilExpiry(session: Session, now: Date = new Date()): number {
  if (session.expiresAt === null) return 0
  const delta = Math.floor((Date.parse(session.expiresAt) - now.getTime()) / 1000)
  return delta > 0 ? delta : 0
}

/** What the SPA is told about the current session. Never includes the token. */
export function publicSession(session: Session): {
  authenticated: boolean
  username: string
  displayName: string
  roles: Role[]
  expiresAt: string | null
  canOperate: boolean
  canAdminister: boolean
} {
  return {
    authenticated: session.authenticated,
    username: session.username,
    displayName: session.displayName,
    roles: session.roles,
    expiresAt: session.expiresAt,
    canOperate: hasAnyRole(session, ['operator', 'admin']),
    canAdminister: hasAnyRole(session, ['admin']),
  }
}

export function hasAnyRole(session: Session, required: Role[]): boolean {
  return required.some((role) => session.roles.includes(role))
}
