package identity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/identity"
)

func TestFromRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/orders", nil)
	r.Header.Set(identity.HeaderSubject, "wendy")
	r.Header.Set(identity.HeaderRoles, "customer, Operator ,,customer")

	id := identity.FromRequest(r)
	if id.Subject != "wendy" {
		t.Errorf("subject = %q", id.Subject)
	}
	if want := []string{"customer", "operator"}; !reflect.DeepEqual(id.Roles, want) {
		t.Errorf("roles = %v, want %v", id.Roles, want)
	}
	if !id.HasRole(identity.RoleOperator) {
		t.Error("HasRole(operator) = false")
	}
	if !id.IsStaff() {
		t.Error("IsStaff = false for operator")
	}
}

func TestCustomerIsNotStaff(t *testing.T) {
	id := identity.Identity{Subject: "wendy", Roles: []string{identity.RoleCustomer}}
	if id.IsStaff() {
		t.Error("a customer must not be staff")
	}
	if id.HasRole(identity.RoleAdmin) {
		t.Error("customer must not hold admin")
	}
}

func TestStripRemovesForgedHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(identity.HeaderSubject, "attacker")
	r.Header.Set("x-encabulator-roles", "admin")
	r.Header.Set("X-Encabulator-Anything", "nope")
	r.Header.Set("Authorization", "Bearer keep-me")

	identity.Strip(r.Header)

	if got := identity.FromRequest(r); got.Subject != "" || len(got.Roles) != 0 {
		t.Errorf("identity survived Strip: %+v", got)
	}
	if r.Header.Get("Authorization") == "" {
		t.Error("Strip removed unrelated headers")
	}
}

func TestApplyOverwrites(t *testing.T) {
	h := http.Header{}
	h.Set(identity.HeaderSubject, "stale")
	identity.Identity{Subject: "wendy", Roles: []string{"customer", "viewer"}}.Apply(h)

	if h.Get(identity.HeaderSubject) != "wendy" {
		t.Errorf("subject = %q", h.Get(identity.HeaderSubject))
	}
	if h.Get(identity.HeaderRoles) != "customer,viewer" {
		t.Errorf("roles = %q", h.Get(identity.HeaderRoles))
	}
}

func TestContextRoundTrip(t *testing.T) {
	want := identity.Identity{Subject: "wendy", Roles: []string{"admin"}}
	ctx := identity.NewContext(context.Background(), want)
	if got := identity.FromContext(ctx); !reflect.DeepEqual(got, want) {
		t.Errorf("FromContext = %+v, want %+v", got, want)
	}
	if got := identity.FromContext(context.Background()); got.Subject != "" {
		t.Errorf("empty context yielded %+v", got)
	}
}
