package adapter

import (
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-github/internal/protocol"
)

// TestCreatePullRequestHappyPath wires the seven-step Git Data API recipe
// against an in-process stub doGitHubRequest and asserts that the
// resolution sequence (ref → commit → blob → tree → commit → ref → PR)
// fires exactly once each, with the right payload shapes.
func TestCreatePullRequestHappyPath(t *testing.T) {
	previous := doGitHubRequest
	defer func() { doGitHubRequest = previous }()

	var calls []string
	doGitHubRequest = func(_ string, _ string, method, path string, _ any, _ ...int) ([]byte, int, error) {
		calls = append(calls, method+" "+path)
		switch {
		case method == "GET" && strings.HasPrefix(path, "/repos/dakasa-co/dakasa-hall/git/refs/heads/main"):
			return []byte(`{"object":{"sha":"basesha"}}`), 200, nil
		case method == "GET" && strings.HasPrefix(path, "/repos/dakasa-co/dakasa-hall/git/commits/basesha"):
			return []byte(`{"tree":{"sha":"basetree"}}`), 200, nil
		case method == "POST" && strings.HasSuffix(path, "/git/blobs"):
			return []byte(`{"sha":"blobsha"}`), 201, nil
		case method == "POST" && strings.HasSuffix(path, "/git/trees"):
			return []byte(`{"sha":"treesha"}`), 201, nil
		case method == "POST" && strings.HasSuffix(path, "/git/commits"):
			return []byte(`{"sha":"commitsha"}`), 201, nil
		case method == "POST" && strings.HasSuffix(path, "/git/refs"):
			return []byte(`{}`), 201, nil
		case method == "POST" && strings.HasSuffix(path, "/pulls"):
			return []byte(`{"html_url":"https://github.com/dakasa-co/dakasa-hall/pull/42","number":42}`), 201, nil
		}
		t.Fatalf("unexpected call %s %s", method, path)
		return nil, 0, nil
	}

	resp, err := Execute(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationCreatePullRequest,
		Auth:      map[string]any{"token": "ghp_test"},
		Input: map[string]any{
			"repository":     "dakasa-co/dakasa-hall",
			"branch":         "fix/missing-rabbitmq-queue-dakasa-hall",
			"commit_message": "🐛 declare missing rabbitmq queue (template: queue_declare_passive)",
			"pr_title":       "Heimdall hotfix: missing rabbitmq queue",
			"pr_body":        "Auto-generated from heimdall.propose_code_fix → template_kind=queue_declare_passive_missing_queue",
			"files": []any{
				map[string]any{
					"path":    "config/definitions.json",
					"content": "{\"queues\": [{\"name\": \"dakasa.hall.events\"}]}",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute(create_pull_request) error = %v", err)
	}
	if resp.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", resp.Status)
	}
	out := resp.Output.(map[string]any)
	if out["pr_number"] != 42 {
		t.Fatalf("pr_number = %v, want 42", out["pr_number"])
	}
	if out["pr_url"] != "https://github.com/dakasa-co/dakasa-hall/pull/42" {
		t.Fatalf("pr_url = %v", out["pr_url"])
	}
	if out["commit_sha"] != "commitsha" {
		t.Fatalf("commit_sha = %v, want commitsha", out["commit_sha"])
	}

	wantSteps := 7
	if len(calls) != wantSteps {
		t.Fatalf("calls = %d (%v), want %d (ref→commit→blob→tree→commit→ref→PR)", len(calls), calls, wantSteps)
	}
}

func TestCreatePullRequestRequiresRepository(t *testing.T) {
	_, err := createPullRequest(protocol.AdapterExecuteIntegrationRequest{
		Auth:  map[string]any{"token": "ghp_test"},
		Input: map[string]any{"branch": "x", "commit_message": "y", "files": []any{}},
	})
	if err == nil {
		t.Fatal("createPullRequest error = nil, want repository required")
	}
}

func TestCreatePullRequestRequiresFiles(t *testing.T) {
	_, err := createPullRequest(protocol.AdapterExecuteIntegrationRequest{
		Auth: map[string]any{"token": "ghp_test"},
		Input: map[string]any{
			"repository":     "dakasa-co/dakasa-hall",
			"branch":         "x",
			"commit_message": "y",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "files is required") {
		t.Fatalf("createPullRequest error = %v, want files-required error", err)
	}
}

func TestCreatePullRequestRejectsBadRepository(t *testing.T) {
	_, err := createPullRequest(protocol.AdapterExecuteIntegrationRequest{
		Auth: map[string]any{"token": "ghp_test"},
		Input: map[string]any{
			"repository":     "no-slash",
			"branch":         "x",
			"commit_message": "y",
			"files":          []any{map[string]any{"path": "a", "content": "b"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("createPullRequest error = %v, want owner/repo error", err)
	}
}

func TestCreatePullRequestPropagatesAPIError(t *testing.T) {
	previous := doGitHubRequest
	defer func() { doGitHubRequest = previous }()

	doGitHubRequest = func(_ string, _ string, method, path string, _ any, _ ...int) ([]byte, int, error) {
		// First call (GET refs) succeeds; second (GET commit) fails.
		if method == "GET" && strings.Contains(path, "/git/refs/heads/main") {
			return []byte(`{"object":{"sha":"basesha"}}`), 200, nil
		}
		return nil, 500, &fakeAPIErr{msg: "github 500"}
	}

	_, err := createPullRequest(protocol.AdapterExecuteIntegrationRequest{
		Auth: map[string]any{"token": "ghp_test"},
		Input: map[string]any{
			"repository":     "dakasa-co/dakasa-hall",
			"branch":         "x",
			"commit_message": "y",
			"files":          []any{map[string]any{"path": "a", "content": "b"}},
		},
	})
	if err == nil {
		t.Fatal("createPullRequest error = nil, want propagated 500")
	}
	if !strings.Contains(err.Error(), "github 500") {
		t.Fatalf("createPullRequest error = %v, want propagated 500 message", err)
	}
}

type fakeAPIErr struct{ msg string }

func (e *fakeAPIErr) Error() string { return e.msg }
