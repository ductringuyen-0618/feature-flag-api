package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	svc := cached.New(db, nil, time.Hour, false)
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
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewBufferString(`{"user_id":"u","flags":["a","b","a"]}`))
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

func TestBulkEvaluateMissingFlag(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/flags", bytes.NewBufferString(`{"name":"a","enabled":true,"rollout_percent":100}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewBufferString(`{"user_id":"u","flags":["a","missing"]}`))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("bulk: %d %s", rr.Code, rr.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["error"] != "flag not found" {
		t.Fatalf("error=%q", out["error"])
	}
}

func TestBulkEvaluateCap(t *testing.T) {
	h := newTestServer(t)
	names := make([]string, 101)
	for i := range names {
		names[i] = "f" + strconv.Itoa(i)
	}
	payload, err := json.Marshal(map[string]any{"user_id": "u", "flags": names})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["error"] != "too many flags" {
		t.Fatalf("error=%q", out["error"])
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
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status=%q", body["status"])
	}
}

func TestHealthzDegradedWhenRedisWanted(t *testing.T) {
	db := memory.New()
	svc := cached.New(db, nil, time.Hour, true)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := httpapi.New(svc)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz %d", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "degraded" {
		t.Fatalf("status=%q", body["status"])
	}
}

func TestReadyz(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("readyz %d", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ready" {
		t.Fatalf("status=%q", body["status"])
	}
}

func TestListFlags(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/flags", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var empty struct {
		Flags []map[string]any `json:"flags"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Flags == nil {
		t.Fatal("flags key missing or null")
	}
	if len(empty.Flags) != 0 {
		t.Fatalf("empty flags=%v", empty.Flags)
	}

	for _, name := range []string{"alpha", "beta"} {
		body := `{"name":"` + name + `","enabled":true,"rollout_percent":100}`
		req = httptest.NewRequest(http.MethodPost, "/v1/flags", bytes.NewBufferString(body))
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, rr.Code, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/flags", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Flags []map[string]any `json:"flags"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Flags) != 2 {
		t.Fatalf("flags len=%d body=%s", len(out.Flags), rr.Body.String())
	}
	names := map[string]bool{}
	for _, f := range out.Flags {
		n, _ := f["name"].(string)
		names[n] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Fatalf("names=%v", names)
	}
}

func TestDeleteFlag(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/flags", bytes.NewBufferString(`{"name":"doomed","enabled":true,"rollout_percent":100}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/flags/doomed", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("delete body=%q", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/flags/doomed", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get after delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	var after map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after["error"] != "flag not found" {
		t.Fatalf("error=%q", after["error"])
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/flags/doomed", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("delete missing status=%d body=%s", rr.Code, rr.Body.String())
	}
	var missing map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &missing); err != nil {
		t.Fatal(err)
	}
	if missing["error"] != "flag not found" {
		t.Fatalf("error=%q", missing["error"])
	}
}

func TestDeleteOverride(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/flags", bytes.NewBufferString(`{"name":"checkout","enabled":false,"rollout_percent":100}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/v1/flags/checkout/users/alice", bytes.NewBufferString(`{"enabled":true}`))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("put override: %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/flags/checkout/users/alice", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete override status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("delete override body=%q", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/evaluate/checkout?user_id=alice", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("eval after delete override: %d %s", rr.Code, rr.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["enabled"] != false || res["reason"] != "FLAG_DISABLED" {
		t.Fatalf("eval after delete override=%v", res)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/flags/checkout/users/alice", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("delete missing override status=%d body=%s", rr.Code, rr.Body.String())
	}
	var missing map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &missing); err != nil {
		t.Fatal(err)
	}
	if missing["error"] != "override not found" {
		t.Fatalf("error=%q", missing["error"])
	}
}

func TestGetFlagNotFound(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/flags/missing", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["error"] != "flag not found" {
		t.Fatalf("error=%q", out["error"])
	}
}
