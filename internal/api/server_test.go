package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Whitedukecmr/openstack-go-api/internal/cloud"
)

// fakeCloud is an in-memory Cloud used for handler tests.
type fakeCloud struct {
	servers map[string]cloud.Server
	actions []string
}

func newFake() *fakeCloud {
	return &fakeCloud{servers: map[string]cloud.Server{"s1": {ID: "s1", Name: "vm1", Status: "ACTIVE"}}}
}

func (f *fakeCloud) ListServers(context.Context) ([]cloud.Server, error) {
	out := []cloud.Server{}
	for _, s := range f.servers {
		out = append(out, s)
	}
	return out, nil
}
func (f *fakeCloud) GetServer(_ context.Context, id string) (*cloud.Server, error) {
	s, ok := f.servers[id]
	if !ok {
		return nil, cloud.ErrNotFound
	}
	return &s, nil
}
func (f *fakeCloud) CreateServer(_ context.Context, r cloud.CreateServerRequest) (*cloud.Server, error) {
	s := cloud.Server{ID: "s2", Name: r.Name, Status: "BUILD"}
	f.servers[s.ID] = s
	return &s, nil
}
func (f *fakeCloud) DeleteServer(_ context.Context, id string) error {
	if _, ok := f.servers[id]; !ok {
		return cloud.ErrNotFound
	}
	delete(f.servers, id)
	return nil
}
func (f *fakeCloud) ServerAction(_ context.Context, id, a string) error {
	f.actions = append(f.actions, id+":"+a)
	return nil
}
func (f *fakeCloud) ListFlavors(context.Context) ([]cloud.Flavor, error) {
	return []cloud.Flavor{{ID: "1", Name: "m1.tiny", VCPUs: 1, RAM: 512, Disk: 1}}, nil
}
func (f *fakeCloud) ListImages(context.Context) ([]cloud.Image, error)     { return nil, nil }
func (f *fakeCloud) ListNetworks(context.Context) ([]cloud.Network, error) { return nil, nil }
func (f *fakeCloud) CreateNetwork(_ context.Context, r cloud.CreateNetworkRequest) (*cloud.Network, error) {
	return &cloud.Network{ID: "n1", Name: r.Name}, nil
}
func (f *fakeCloud) DeleteNetwork(context.Context, string) error         { return nil }
func (f *fakeCloud) ListVolumes(context.Context) ([]cloud.Volume, error) { return nil, nil }
func (f *fakeCloud) CreateVolume(_ context.Context, r cloud.CreateVolumeRequest) (*cloud.Volume, error) {
	return &cloud.Volume{ID: "v1", Name: r.Name, SizeGB: r.SizeGB}, nil
}
func (f *fakeCloud) DeleteVolume(context.Context, string) error { return nil }

func do(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func handler(f *fakeCloud, token string) http.Handler {
	return New(f, token, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestHealthzNoAuth(t *testing.T) {
	h := handler(newFake(), "secret")
	if rec := do(t, h, "GET", "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestAuth(t *testing.T) {
	h := handler(newFake(), "secret")
	if rec := do(t, h, "GET", "/api/v1/servers", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/v1/servers", "", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestServerLifecycle(t *testing.T) {
	f := newFake()
	h := handler(f, "")

	rec := do(t, h, "POST", "/api/v1/servers", `{"name":"web","flavor_id":"1","image_id":"img","network_id":"net"}`, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create: want 202, got %d (%s)", rec.Code, rec.Body)
	}
	var s cloud.Server
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Name != "web" {
		t.Fatalf("unexpected name %q", s.Name)
	}

	if rec := do(t, h, "POST", "/api/v1/servers/s2/stop", "", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("stop: want 202, got %d", rec.Code)
	}
	if len(f.actions) != 1 || f.actions[0] != "s2:stop" {
		t.Fatalf("action not forwarded: %v", f.actions)
	}
	if rec := do(t, h, "POST", "/api/v1/servers/s2/explode", "", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad action: want 400, got %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/v1/servers/s2", "", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/v1/servers/s2", "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get deleted: want 404, got %d", rec.Code)
	}
}

func TestValidation(t *testing.T) {
	h := handler(newFake(), "")
	cases := []struct{ path, body string }{
		{"/api/v1/servers", `{"name":"x"}`},
		{"/api/v1/servers", `{not json}`},
		{"/api/v1/networks", `{"name":"n","cidr":"nope"}`},
		{"/api/v1/volumes", `{"name":"v","size_gb":0}`},
	}
	for _, c := range cases {
		if rec := do(t, h, "POST", c.path, c.body, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s %s: want 400, got %d", c.path, c.body, rec.Code)
		}
	}
	if rec := do(t, h, "POST", "/api/v1/networks", `{"name":"n","cidr":"10.10.0.0/24"}`, ""); rec.Code != http.StatusCreated {
		t.Errorf("valid network: want 201, got %d", rec.Code)
	}
}
