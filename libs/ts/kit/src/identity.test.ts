import { describe, expect, it } from 'vitest';
import {
  hasAnyRole,
  hasRole,
  identityFromHeaders,
  identityHeaders,
  isStaff,
  parseRoles,
  ROLE_ADMIN,
  ROLE_CUSTOMER,
  ROLES_HEADER,
  SUBJECT_HEADER,
} from './identity.js';

describe('identity headers', () => {
  it('reads subject and roles', () => {
    const identity = identityFromHeaders({
      [SUBJECT_HEADER]: 'alice',
      [ROLES_HEADER]: 'customer, OPERATOR ',
    });
    expect(identity).toEqual({ subject: 'alice', roles: ['customer', 'operator'] });
  });

  it('yields an anonymous identity when the headers are absent', () => {
    expect(identityFromHeaders({})).toEqual({ subject: '', roles: [] });
  });

  it('takes the first value of a repeated header', () => {
    const identity = identityFromHeaders({ [SUBJECT_HEADER]: ['alice', 'mallory'] });
    expect(identity.subject).toBe('alice');
  });

  it('round-trips through identityHeaders', () => {
    const identity = { subject: 'bob', roles: ['customer'] };
    expect(identityFromHeaders(identityHeaders(identity))).toEqual(identity);
  });

  it('omits empty headers when propagating', () => {
    expect(identityHeaders({ subject: '', roles: [] })).toEqual({});
  });

  it('ignores blank role entries', () => {
    expect(parseRoles('customer,,  ,admin')).toEqual(['customer', 'admin']);
  });
});

describe('role checks', () => {
  const customer = { subject: 'alice', roles: ['customer'] };
  const admin = { subject: 'root', roles: [ROLE_ADMIN] };

  it('matches case-insensitively', () => {
    expect(hasRole({ subject: 'a', roles: ['admin'] }, 'ADMIN')).toBe(true);
  });

  it('supports any-of', () => {
    expect(hasAnyRole(customer, ROLE_ADMIN, ROLE_CUSTOMER)).toBe(true);
    expect(hasAnyRole(customer, ROLE_ADMIN)).toBe(false);
  });

  it('treats operator, viewer and admin as staff', () => {
    expect(isStaff(customer)).toBe(false);
    expect(isStaff(admin)).toBe(true);
    expect(isStaff({ subject: 'v', roles: ['viewer'] })).toBe(true);
  });
});
