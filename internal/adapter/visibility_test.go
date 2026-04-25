package adapter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetContainerPackageVisibilityPublic(t *testing.T) {
	var gotMethod, gotURL, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotURL = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	transport := &rewriteRoundTripper{
		target:   server.URL,
		original: "https://api.github.com",
		delegate: http.DefaultTransport,
	}
	client := &http.Client{Transport: transport}
	if err := SetContainerPackageVisibility(client, "tk", "orgs", "dakasa-yggdrasil", "yggdrasil-core", "public"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotURL != "/orgs/dakasa-yggdrasil/packages/container/yggdrasil-core" {
		t.Errorf("url = %q", gotURL)
	}
	if gotAuth != "Bearer tk" {
		t.Errorf("auth = %q", gotAuth)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(gotBody), &parsed); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if parsed["visibility"] != "public" {
		t.Errorf("body.visibility = %q, want public", parsed["visibility"])
	}
}

type rewriteRoundTripper struct {
	target   string
	original string
	delegate http.RoundTripper
}

func (rt *rewriteRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), rt.original) {
		rebuilt := strings.Replace(req.URL.String(), rt.original, rt.target, 1)
		newReq, err := http.NewRequest(req.Method, rebuilt, req.Body)
		if err != nil {
			return nil, err
		}
		newReq.Header = req.Header
		return rt.delegate.RoundTrip(newReq)
	}
	return rt.delegate.RoundTrip(req)
}

func TestSetContainerPackageVisibilityValidation(t *testing.T) {
	cases := map[string]struct {
		ownerType, owner, pkg, vis, token string
	}{
		"bad-owner-type": {"foo", "o", "p", "public", "t"},
		"empty-owner":    {"orgs", "", "p", "public", "t"},
		"empty-package":  {"orgs", "o", "", "public", "t"},
		"bad-visibility": {"orgs", "o", "p", "weird", "t"},
		"empty-token":    {"orgs", "o", "p", "public", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := SetContainerPackageVisibility(nil, c.token, c.ownerType, c.owner, c.pkg, c.vis)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestSetContainerPackageVisibilityServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()

	transport := &rewriteRoundTripper{
		target:   server.URL,
		original: "https://api.github.com",
		delegate: http.DefaultTransport,
	}
	client := &http.Client{Transport: transport}
	err := SetContainerPackageVisibility(client, "tk", "orgs", "o", "p", "public")
	if err == nil {
		t.Fatal("expected error from 401 response")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("error %q must include status code and server message", err)
	}
}
