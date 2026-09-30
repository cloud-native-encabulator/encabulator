package gateway

import (
	"net/http"
	"strings"
)

// Access says what a route requires of its caller.
type Access int

// Access levels.
const (
	// Public routes need no token; the rate limit keys on the client address.
	Public Access = iota
	// Authenticated routes need a valid token carrying at least one role. This is
	// the design's "customer+" and "any role": the gateway checks that the caller
	// is who it says it is, and each upstream applies its own finer rules (the
	// ledger, for instance, filters a customer's list to its own orders).
	Authenticated
)

// Route is one entry of the gateway's fixed routing table.
type Route struct {
	// Pattern is the inbound Go 1.22 ServeMux pattern, for example
	// "GET /api/v1/orders/{id}".
	Pattern string
	// Upstream is the target service name, which is also the metric's
	// server_address label and the environment variable prefix.
	Upstream string
	// UpstreamPath is the target path, with the same wildcard names as Pattern.
	UpstreamPath string
	// Access is what the route requires of its caller.
	Access Access
}

// Label is the route label used in metrics: the pattern's path, never a raw path.
func (r Route) Label() string {
	if i := strings.IndexByte(r.Pattern, '/'); i > 0 {
		return r.Pattern[i:]
	}
	return r.Pattern
}

// Routes is the gateway's whole surface, straight out of design section 8.1.
// Nothing else is proxied: the gateway is the only public entry point, and a
// route that is not in this table does not exist.
func Routes() []Route {
	return []Route{
		{Pattern: "POST /api/v1/auth/login", Upstream: "unilateral-phase-detractor",
			UpstreamPath: "/v1/auth/login", Access: Public},
		{Pattern: "POST /api/v1/auth/register", Upstream: "unilateral-phase-detractor",
			UpstreamPath: "/v1/auth/register", Access: Public},
		{Pattern: "GET /api/v1/users/me", Upstream: "unilateral-phase-detractor",
			UpstreamPath: "/v1/users/me", Access: Authenticated},
		{Pattern: "GET /api/v1/parts", Upstream: "prefabulated-amulite-depot",
			UpstreamPath: "/v1/parts", Access: Public},
		{Pattern: "POST /api/v1/quotes", Upstream: "spurving-bearing-quoter",
			UpstreamPath: "/v1/quotes", Access: Authenticated},
		{Pattern: "POST /api/v1/orders", Upstream: "panametric-fam-processor",
			UpstreamPath: "/v1/orders", Access: Authenticated},
		{Pattern: "GET /api/v1/orders", Upstream: "hydrocoptic-datastore",
			UpstreamPath: "/v1/orders", Access: Authenticated},
		{Pattern: "GET /api/v1/orders/{id}", Upstream: "hydrocoptic-datastore",
			UpstreamPath: "/v1/orders/{id}", Access: Authenticated},
		{Pattern: "GET /api/v1/shipments/{orderId}", Upstream: "nofer-trunnion-dispatch",
			UpstreamPath: "/v1/shipments/{orderId}", Access: Authenticated},
	}
}

// Upstreams returns the distinct upstream service names the table refers to.
func Upstreams() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 6)
	for _, r := range Routes() {
		if _, ok := seen[r.Upstream]; ok {
			continue
		}
		seen[r.Upstream] = struct{}{}
		out = append(out, r.Upstream)
	}
	return out
}

// resolvePath substitutes the request's path values into an upstream path
// template. Only wildcard names declared in the inbound pattern can appear, so
// nothing a client controls reaches the path except through url-escaped segments
// the router already parsed.
func resolvePath(template string, r *http.Request) string {
	if !strings.ContainsRune(template, '{') {
		return template
	}
	segments := strings.Split(template, "/")
	for i, segment := range segments {
		if len(segment) < 3 || segment[0] != '{' || segment[len(segment)-1] != '}' {
			continue
		}
		name := strings.TrimSuffix(segment[1:len(segment)-1], "...")
		segments[i] = r.PathValue(name)
	}
	return strings.Join(segments, "/")
}
