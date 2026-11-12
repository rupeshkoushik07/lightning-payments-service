package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
)

type fakeUsers struct{ sessions int }

func (f *fakeUsers) UserForLogin(_ context.Context, name string) (store.User, string, error) {
	return store.User{ID: 1, Username: name, Role: "receiver"}, "", nil
}
func (f *fakeUsers) CreateSession(context.Context, []byte, int64, time.Time) error {
	f.sessions++
	return nil
}
func (f *fakeUsers) SessionUser(context.Context, []byte) (store.User, error) {
	return store.User{}, store.ErrNotFound
}
func (f *fakeUsers) DeleteSession(context.Context, []byte) error { return nil }

func demoLogin(a *Auth, body, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/demo-login", strings.NewReader(body))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	a.DemoLogin(w, r)
	return w
}

// With demo mode off, the one-click door does not exist.
func TestDemoLoginOffByDefault(t *testing.T) {
	users := &fakeUsers{}
	w := demoLogin(&Auth{Users: users}, `{"username":"admin"}`, "")
	if w.Code != http.StatusNotFound || users.sessions != 0 {
		t.Fatalf("got %d with %d sessions, want 404 and none", w.Code, users.sessions)
	}
}

func TestDemoLogin(t *testing.T) {
	cases := []struct {
		name, body, origin string
		want               int
	}{
		{"a demo user gets a session", `{"username":"alice"}`, "http://localhost:8080", http.StatusOK},
		{"any other name is refused", `{"username":"root"}`, "", http.StatusBadRequest},
		{"another website is refused", `{"username":"alice"}`, "http://evil.example", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			users := &fakeUsers{}
			w := demoLogin(&Auth{Users: users, Demo: true}, c.body, c.origin)
			if w.Code != c.want {
				t.Fatalf("got %d, want %d", w.Code, c.want)
			}
			if (users.sessions == 1) != (c.want == http.StatusOK) {
				t.Fatalf("sessions created: %d", users.sessions)
			}
		})
	}
}
