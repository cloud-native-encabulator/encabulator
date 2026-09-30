/**
 * Identity propagation (design section 7, "Identity propagation").
 *
 * The gateway strips inbound `X-Encabulator-*` headers and sets `X-Encabulator-Subject`
 * and `X-Encabulator-Roles`. Internal services trust them.
 */

export const SUBJECT_HEADER = 'x-encabulator-subject';
export const ROLES_HEADER = 'x-encabulator-roles';
export const REQUEST_ID_HEADER = 'x-request-id';

export const ROLE_CUSTOMER = 'customer';
export const ROLE_VIEWER = 'viewer';
export const ROLE_OPERATOR = 'operator';
export const ROLE_ADMIN = 'admin';

export const ALL_ROLES = [ROLE_CUSTOMER, ROLE_VIEWER, ROLE_OPERATOR, ROLE_ADMIN] as const;
export type Role = (typeof ALL_ROLES)[number];

export interface Identity {
  subject: string;
  roles: string[];
}

export const ANONYMOUS: Identity = { subject: '', roles: [] };

type HeaderBag = Record<string, string | string[] | undefined>;

function firstValue(bag: HeaderBag, name: string): string | undefined {
  const raw = bag[name] ?? bag[name.toLowerCase()];
  if (raw === undefined) return undefined;
  return Array.isArray(raw) ? raw[0] : raw;
}

export function parseRoles(raw: string | undefined): string[] {
  if (!raw) return [];
  return raw
    .split(',')
    .map((r) => r.trim().toLowerCase())
    .filter((r) => r.length > 0);
}

export function identityFromHeaders(headers: HeaderBag): Identity {
  return {
    subject: firstValue(headers, SUBJECT_HEADER)?.trim() ?? '',
    roles: parseRoles(firstValue(headers, ROLES_HEADER)),
  };
}

/** Headers to forward on an outbound internal call. */
export function identityHeaders(identity: Identity): Record<string, string> {
  const headers: Record<string, string> = {};
  if (identity.subject) headers[SUBJECT_HEADER] = identity.subject;
  if (identity.roles.length > 0) headers[ROLES_HEADER] = identity.roles.join(',');
  return headers;
}

export function hasRole(identity: Identity, role: string): boolean {
  return identity.roles.includes(role.toLowerCase());
}

export function hasAnyRole(identity: Identity, ...roles: string[]): boolean {
  return roles.some((role) => hasRole(identity, role));
}

/**
 * "Staff" means allowed to see the whole factory rather than just their own orders
 * (design 8.6 visibility rule).
 */
export function isStaff(identity: Identity): boolean {
  return hasAnyRole(identity, ROLE_OPERATOR, ROLE_VIEWER, ROLE_ADMIN);
}
