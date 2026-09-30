package journey

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeGateway is a stand-in for dingle-arm-gateway: it implements just enough
// of the customer-facing API for the journeys to be exercised end to end, and
// records what it was asked for.
type fakeGateway struct {
	server *httptest.Server

	mu    sync.Mutex
	calls []call
	users map[string]string // username -> password
	// tokens maps a token to its owner. Revoking a token makes the next call
	// answer 401, which is how the refresh path is tested.
	tokens   map[string]string
	issued   int
	orders   []string
	noOrders bool
	// failShipments makes the shipment lookup answer 404, the normal state of
	// an order that has not been dispatched yet.
	failShipments bool
	// quoteStatus, when non-zero, overrides the quote response status.
	quoteStatus int
}

type call struct {
	method string
	route  string
	token  string
}

func newFakeGateway(t *testing.T) *fakeGateway {
	t.Helper()
	g := &fakeGateway{
		users:  map[string]string{},
		tokens: map[string]string{},
		orders: []string{"ord_0000000000000001", "ord_0000000000000002"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", g.register)
	mux.HandleFunc("POST /api/v1/auth/login", g.login)
	mux.HandleFunc("GET /api/v1/parts", g.authed("GET /api/v1/parts", g.parts))
	mux.HandleFunc("POST /api/v1/quotes", g.authed("POST /api/v1/quotes", g.quote))
	mux.HandleFunc("POST /api/v1/orders", g.authed("POST /api/v1/orders", g.order))
	mux.HandleFunc("GET /api/v1/orders", g.authed("GET /api/v1/orders", g.listOrders))
	mux.HandleFunc("GET /api/v1/orders/{id}", g.authed("GET /api/v1/orders/{id}", g.getOrder))
	mux.HandleFunc("GET /api/v1/shipments/{orderId}", g.authed("GET /api/v1/shipments/{orderId}", g.getShipment))
	g.server = httptest.NewServer(mux)
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGateway) URL() string { return g.server.URL }

func (g *fakeGateway) record(method, route, token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, call{method: method, route: route, token: token})
}

func (g *fakeGateway) routeCounts() map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[string]int{}
	for _, c := range g.calls {
		out[c.route]++
	}
	return out
}

func (g *fakeGateway) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.calls)
}

// revokeAll invalidates every issued token, simulating a key rotation or an
// expired token that the grammeter has not noticed yet.
func (g *fakeGateway) revokeAll() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tokens = map[string]string{}
}

func (g *fakeGateway) issuedTokens() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.issued
}

// authed wraps a handler with bearer-token verification.
func (g *fakeGateway) authed(route string, h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		g.record(r.Method, route, token)
		g.mu.Lock()
		_, ok := g.tokens[token]
		g.mu.Unlock()
		if token == "" || !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]string{"code": "unauthenticated", "message": "no"},
			})
			return
		}
		h(w, r)
	}
}

func (g *fakeGateway) register(w http.ResponseWriter, r *http.Request) {
	g.record(r.Method, "POST /api/v1/auth/register", "")
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid"})
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.users[body.Username]; exists {
		writeJSON(w, http.StatusConflict, map[string]string{"code": "user_exists"})
		return
	}
	g.users[body.Username] = body.Password
	writeJSON(w, http.StatusCreated, map[string]string{"username": body.Username})
}

func (g *fakeGateway) login(w http.ResponseWriter, r *http.Request) {
	g.record(r.Method, "POST /api/v1/auth/login", "")
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid"})
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if pw, ok := g.users[body.Username]; !ok || pw != body.Password {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "bad_credentials"})
		return
	}
	g.issued++
	token := fmt.Sprintf("tok-%s-%d", body.Username, g.issued)
	g.tokens[token] = body.Username
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken": token,
		"tokenType":   "Bearer",
		"expiresIn":   3600,
		"user":        map[string]any{"username": body.Username, "roles": []string{"customer"}},
	})
}

func (g *fakeGateway) parts(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]any{
		{"sku": "AMU-BASE", "name": "Prefabulated amulite base-plate", "stock": 1000, "reserved": 0, "perUnit": 1},
	})
}

func (g *fakeGateway) quote(w http.ResponseWriter, _ *http.Request) {
	g.mu.Lock()
	status := g.quoteStatus
	g.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"id": "qte_1", "total": 1944.0, "currency": "USD"})
}

func (g *fakeGateway) order(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]any{"id": "ord_new", "status": "accepted"})
}

func (g *fakeGateway) listOrders(w http.ResponseWriter, _ *http.Request) {
	g.mu.Lock()
	empty, ids := g.noOrders, g.orders
	g.mu.Unlock()
	if empty {
		writeJSON(w, http.StatusOK, map[string]any{"orders": []any{}})
		return
	}
	orders := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		orders = append(orders, map[string]any{"id": id, "status": "fabricating"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": orders})
}

func (g *fakeGateway) getOrder(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "status": "fabricating"})
}

func (g *fakeGateway) getShipment(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	fail := g.failShipments
	g.mu.Unlock()
	if fail {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": map[string]string{"code": "not_found", "message": "no shipment"},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orderId": r.PathValue("orderId"), "carrier": "Trunnion Freight", "trackingNumber": "NT0123456789",
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
