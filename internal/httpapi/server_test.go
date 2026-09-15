package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ductringuyen-0618/feature-flag-api/internal/httpapi"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/cached"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/memory"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	db := memory.New()
	svc := cached.New(db, nil, time.Hour)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return httpapi.New(svc)
}

func TestCreateAndEvaluate(t *testing.T) {
	h := newTestServer(t)

	body := `{"name":"dark-mode","description":"ui","enabled":true,"rollout_percent":100}`
	req := httptest.NewRequest(http.MethodPost, "/v1/flags", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/evaluate/dark-mode?user_id=alice", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("eval status=%d body=%s", rr.Code, rr.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["enabled"] != true {
		t.Fatalf("enabled=%v", res["enabled"])
	}
}

func TestKillSwitchAndOverride(t *testing.T) {
	h := newTestServer(t)

	create := `{"name":"checkout","enabled":true,"rollout_percent":50}`
	req := httptest.NewRequest(http.MethodPost, "/v1/flags", bytes.NewBufferString(create))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}

	// Kill switch
	patch := `{"enabled":false}`
	req = httptest.NewRequest(http.MethodPatch, "/v1/flags/checkout", bytes.NewBufferString(patch))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/evaluate/checkout?user_id=bob", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var res map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	if res["enabled"] != false || res["reason"] != "FLAG_DISABLED" {
		t.Fatalf("kill switch eval=%v", res)
	}

	// Override wins
	req = httptest.NewRequest(http.MethodPut, "/v1/flags/checkout/users/bob", bytes.NewBufferString(`{"enabled":true}`))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("override: %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/evaluate/checkout?user_id=bob", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	if res["enabled"] != true || res["reason"] != "OVERRIDE" {
		t.Fatalf("override eval=%v", res)
	}
}

func TestBulkEvaluate(t *testing.T) {
	h := newTestServer(t)
	for _, name := range []string{"a", "b"} {
		body := `{"name":"` + name + `","enabled":true,"rollout_percent":100}`
		req := httptest.NewRequest(http.MethodPost, "/v1/flags", bytes.NewBufferString(body))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: %d", name, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewBufferString(`{"user_id":"u","flags":["a","b","missing"]}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bulk: %d %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Results map[string]map[string]any `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results=%v", out.Results)
	}
}

func TestRejectsInvalidIdentifiers(t *testing.T) {
	h := newTestServer(t)
	longName := strings.Repeat("a", 65)
	cases := []struct {
		title  string
		method string
		url    string
		body   string
		err    string
	}{
		{"create space", http.MethodPost, "/v1/flags", `{"name":"bad name","enabled":true}`, "invalid name"},
		{"create too long", http.MethodPost, "/v1/flags", `{"name":"` + longName + `","enabled":true}`, "invalid name"},
		{"path name", http.MethodGet, "/v1/flags/bad%20name", "", "invalid name"},
		{"eval name", http.MethodGet, "/v1/evaluate/bad%20name?user_id=u", "", "invalid name"},
		{"eval user", http.MethodGet, "/v1/evaluate/ok?user_id=bad%20user", "", "invalid user_id"},
		{"override user", http.MethodPut, "/v1/flags/ok/users/bad%20user", `{"enabled":true}`, "invalid user_id"},
		{"bulk flag", http.MethodPost, "/v1/evaluate", `{"user_id":"u","flags":["bad name"]}`, "invalid name"},
		{"bulk user", http.MethodPost, "/v1/evaluate", `{"user_id":"bad user","flags":["ok"]}`, "invalid user_id"},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.url, bytes.NewBufferString(tc.body))
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			var out map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out["error"] != tc.err {
				t.Fatalf("error=%q want %q", out["error"], tc.err)
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz %d", rr.Code)
	}
}
