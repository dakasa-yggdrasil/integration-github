package adapter

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-github/internal/protocol"
)

// fakeRoundTripper lets us substitute doGitHubRequest indirection by
// stubbing the underlying HTTP transport. It returns canned bodies per
// path so multi-step tests stay readable.
type fakeRoundTripper struct {
	t          *testing.T
	respByPath map[string]fakeResponse
	calls      []recordedCall
}

type fakeResponse struct {
	status int
	body   string
}

type recordedCall struct {
	method string
	path   string
	body   string
}

func newRoundTripper(t *testing.T, m map[string]fakeResponse) *fakeRoundTripper {
	return &fakeRoundTripper{t: t, respByPath: m}
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		buf, _ := io.ReadAll(req.Body)
		body = string(buf)
	}
	pathKey := req.Method + " " + req.URL.Path
	f.calls = append(f.calls, recordedCall{method: req.Method, path: req.URL.Path, body: body})
	resp, ok := f.respByPath[pathKey]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"message":"unexpected path"}`)),
			Header:     make(http.Header),
		}, nil
	}
	return &http.Response{
		StatusCode: resp.status,
		Body:       io.NopCloser(bytes.NewReader([]byte(resp.body))),
		Header:     make(http.Header),
	}, nil
}

func TestCreateRunnerToken_OrgScopeHappyPath(t *testing.T) {
	rt := newRoundTripper(t, map[string]fakeResponse{
		"POST /orgs/dakasa-co/actions/runners/registration-token": {
			status: http.StatusCreated,
			body:   `{"token":"AABBCC","expires_at":"2026-04-26T18:00:00Z"}`,
		},
	})
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rt}
	defer func() { http.DefaultClient = originalClient }()

	resp, err := createRunnerToken(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationCreateRunnerToken,
		Auth:      map[string]any{"token": "ghp_x"},
		Input: map[string]any{
			"scope": "org",
			"org":   "dakasa-co",
		},
		Integration: protocol.AdapterExecuteIntegrationContext{},
	})
	if err != nil {
		t.Fatalf("create_runner_token: %v", err)
	}
	if got := resp.Output.(map[string]any)["registration_token"]; got != "AABBCC" {
		t.Fatalf("token: %v", got)
	}
}

func TestCreateRunnerToken_RejectsBadScope(t *testing.T) {
	_, err := createRunnerToken(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationCreateRunnerToken,
		Auth:      map[string]any{"token": "x"},
		Input:     map[string]any{"scope": "weird"},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported scope") {
		t.Fatalf("expected scope error, got %v", err)
	}
}

func TestCreateRunnerToken_RequiresOrgWhenScopeIsOrg(t *testing.T) {
	_, err := createRunnerToken(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationCreateRunnerToken,
		Auth:      map[string]any{"token": "x"},
		Input:     map[string]any{"scope": "org"},
	})
	if err == nil || !strings.Contains(err.Error(), "org is required") {
		t.Fatalf("expected org-required error, got %v", err)
	}
}

func TestListRunners_GatesByLabelExpectation(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"total_count": 2,
		"runners": []map[string]any{
			{"id": 1, "name": "runner-a", "os": "linux", "status": "online", "busy": false, "labels": []map[string]any{{"name": "self-hosted"}, {"name": "linux"}, {"name": "x64"}, {"name": "dakasa-co"}}},
			{"id": 2, "name": "runner-b", "os": "linux", "status": "offline", "busy": false, "labels": []map[string]any{{"name": "self-hosted"}}},
		},
	})
	rt := newRoundTripper(t, map[string]fakeResponse{
		"GET /orgs/dakasa-co/actions/runners": {status: http.StatusOK, body: string(body)},
	})
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rt}
	defer func() { http.DefaultClient = originalClient }()

	_, err := listRunners(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationListRunners,
		Auth:      map[string]any{"token": "ghp_x"},
		Input: map[string]any{
			"scope": "org",
			"org":   "dakasa-co",
			"expect": map[string]any{
				"min_count":              2,
				"min_online_with_labels": []any{"self-hosted", "dakasa-co"},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "only 1 runners match") {
		t.Fatalf("expected gate error, got %v", err)
	}
}

func TestListRunners_PassesWhenExpectationsMet(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"total_count": 2,
		"runners": []map[string]any{
			{"id": 1, "name": "runner-a", "status": "online", "labels": []map[string]any{{"name": "self-hosted"}, {"name": "dakasa-co"}}},
			{"id": 2, "name": "runner-b", "status": "online", "labels": []map[string]any{{"name": "self-hosted"}, {"name": "dakasa-co"}}},
		},
	})
	rt := newRoundTripper(t, map[string]fakeResponse{
		"GET /orgs/dakasa-co/actions/runners": {status: http.StatusOK, body: string(body)},
	})
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rt}
	defer func() { http.DefaultClient = originalClient }()

	resp, err := listRunners(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationListRunners,
		Auth:      map[string]any{"token": "ghp_x"},
		Input: map[string]any{
			"org": "dakasa-co",
			"expect": map[string]any{
				"min_count":              2,
				"min_online_with_labels": []any{"self-hosted"},
			},
		},
	})
	if err != nil {
		t.Fatalf("list_runners: %v", err)
	}
	out := resp.Output.(map[string]any)
	if out["matching_count"] != 2 {
		t.Fatalf("matching_count: %v", out["matching_count"])
	}
}

func TestExecuteRouting_NewOps(t *testing.T) {
	rt := newRoundTripper(t, map[string]fakeResponse{
		"POST /orgs/dakasa-co/actions/runners/registration-token": {status: http.StatusCreated, body: `{"token":"X","expires_at":""}`},
		"GET /orgs/dakasa-co/actions/runners":                     {status: http.StatusOK, body: `{"total_count":0,"runners":[]}`},
	})
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rt}
	defer func() { http.DefaultClient = originalClient }()

	for _, op := range []string{OperationCreateRunnerToken, OperationListRunners} {
		_, err := Execute(protocol.AdapterExecuteIntegrationRequest{
			Operation: op,
			Auth:      map[string]any{"token": "ghp_x"},
			Input: map[string]any{
				"scope": "org",
				"org":   "dakasa-co",
			},
		})
		if err != nil {
			t.Errorf("Execute(%s): %v", op, err)
		}
	}
}

