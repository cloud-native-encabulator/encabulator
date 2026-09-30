/**
 * Server-side role enforcement.
 *
 * The design is unambiguous: "Every mutating route requires `operator` or
 * `admin`, enforced in the BFF. User management requires `admin`." Nothing here
 * trusts the browser. The SPA hides controls a viewer cannot use, but hiding is
 * cosmetic; this file is the gate.
 */

import type { FastifyReply, FastifyRequest } from 'fastify'
import { GUEST, hasAnyRole, type Role, type Session } from './session.js'

export const OPERATOR_ROLES: Role[] = ['operator', 'admin']
export const ADMIN_ROLES: Role[] = ['admin']

declare module 'fastify' {
  interface FastifyRequest {
    session: Session
  }
}

export function sessionOf(request: FastifyRequest): Session {
  return request.session ?? GUEST
}

export interface ErrorBody {
  error: { code: string; message: string }
}

export function errorBody(code: string, message: string): ErrorBody {
  return { error: { code, message } }
}

/**
 * A preHandler that refuses the request unless the session holds one of
 * `required`. Guests get 401 so the SPA can offer a login; signed-in users
 * without the role get 403.
 */
export function requireRole(
  required: Role[],
  onForbidden?: (route: string, role: string) => void,
): (request: FastifyRequest, reply: FastifyReply) => Promise<void> {
  return async function enforce(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    const session = sessionOf(request)
    if (hasAnyRole(session, required)) return

    const route = request.routeOptions?.url ?? request.url
    onForbidden?.(route, session.roles.join('|') || 'none')

    if (!session.authenticated) {
      await reply.code(401).send(
        errorBody(
          'unauthenticated',
          `Sign in as ${describe(required)} to operate the encabulator. Guests may watch.`,
        ),
      )
      return
    }
    await reply
      .code(403)
      .send(
        errorBody(
          'forbidden',
          `This control requires the ${describe(required)} role; you hold ${session.roles.join(', ')}.`,
        ),
      )
  }
}

/** A preHandler that only requires a real signed-in user of any role. */
export function requireSession(
  onForbidden?: (route: string, role: string) => void,
): (request: FastifyRequest, reply: FastifyReply) => Promise<void> {
  return async function enforce(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    const session = sessionOf(request)
    if (session.authenticated && session.token !== null) return
    const route = request.routeOptions?.url ?? request.url
    onForbidden?.(route, 'guest')
    await reply
      .code(401)
      .send(
        errorBody(
          'unauthenticated',
          'The storefront places real orders with your own credentials, so it needs you signed in.',
        ),
      )
  }
}

function describe(required: Role[]): string {
  if (required.length === 1) return required[0] as string
  return `${required.slice(0, -1).join(', ')} or ${required[required.length - 1]}`
}
