// Package identity carries the caller identity that the dingle arm gateway (or
// the console BFF) stamps onto internal requests. See design section 7,
// "Identity propagation".
package identity

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

// Headers set by the gateway and trusted by internal services.
const (
	HeaderSubject = "X-Encabulator-Subject"
	HeaderRoles   = "X-Encabulator-Roles"
)

// HeaderPrefix is the prefix the gateway strips from inbound client requests
// before setting its own values.
const HeaderPrefix = "X-Encabulator-"

// The four roles in the system.
const (
	RoleCustomer = "customer"
	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleAdmin    = "admin"
)

// Identity is a subject and its roles.
type Identity struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

// FromRequest reads the identity headers. Missing headers yield a zero Identity.
func FromRequest(r *http.Request) Identity {
	return Identity{
		Subject: strings.TrimSpace(r.Header.Get(HeaderSubject)),
		Roles:   ParseRoles(r.Header.Get(HeaderRoles)),
	}
}

// ParseRoles splits a comma-separated role list, lowercasing and dropping blanks.
func ParseRoles(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	roles := make([]string, 0, len(parts))
	for _, p := range parts {
		if r := strings.ToLower(strings.TrimSpace(p)); r != "" && !slices.Contains(roles, r) {
			roles = append(roles, r)
		}
	}
	return roles
}

// HasRole reports whether the identity holds any of the given roles.
func (i Identity) HasRole(roles ...string) bool {
	for _, want := range roles {
		if slices.Contains(i.Roles, strings.ToLower(want)) {
			return true
		}
	}
	return false
}

// IsStaff reports whether the identity may see other customers' data, that is
// whether it holds operator, viewer or admin.
func (i Identity) IsStaff() bool {
	return i.HasRole(RoleOperator, RoleViewer, RoleAdmin)
}

// RolesHeader renders the roles as a header value.
func (i Identity) RolesHeader() string { return strings.Join(i.Roles, ",") }

// Apply writes the identity onto an outbound request's headers, replacing any
// values already present.
func (i Identity) Apply(h http.Header) {
	h.Set(HeaderSubject, i.Subject)
	h.Set(HeaderRoles, i.RolesHeader())
}

// Strip removes every X-Encabulator-* header. The gateway calls it on inbound
// client requests so that clients cannot forge an identity.
func Strip(h http.Header) {
	for name := range h {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), HeaderPrefix) {
			h.Del(name)
		}
	}
}

type ctxKey struct{}

// NewContext returns a context carrying the identity.
func NewContext(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the identity carried by ctx, or the zero Identity.
func FromContext(ctx context.Context) Identity {
	id, _ := ctx.Value(ctxKey{}).(Identity)
	return id
}
