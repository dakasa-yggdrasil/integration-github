package adapter

import (
	"encoding/json"
	"testing"

	"github.com/dakasa-yggdrasil/integration-github/internal/protocol"
)

func TestDescribe(t *testing.T) {
	response := Describe()

	if response.Provider != Provider {
		t.Fatalf("provider = %q, want %q", response.Provider, Provider)
	}
	if response.Adapter.Queues.Describe != QueueDescribe {
		t.Fatalf("describe queue = %q, want %q", response.Adapter.Queues.Describe, QueueDescribe)
	}
	if response.Adapter.Queues.Execute != QueueExecute {
		t.Fatalf("execute queue = %q, want %q", response.Adapter.Queues.Execute, QueueExecute)
	}
	if len(response.ActionCatalog) != len(SupportedExecuteOperations) {
		t.Fatalf("action catalog = %#v, want %d actions", response.ActionCatalog, len(SupportedExecuteOperations))
	}
}

func TestSupportedExecuteOperationsStayAligned(t *testing.T) {
	for _, operation := range SupportedExecuteOperations {
		if !SupportsExecuteCapability(operation) {
			t.Fatalf("SupportsExecuteCapability(%q) = false", operation)
		}
	}
}

func TestDispatchWorkflowUsesGenericInput(t *testing.T) {
	previous := doGitHubRequest
	defer func() { doGitHubRequest = previous }()

	var gotMethod string
	var gotPath string
	var gotToken string
	var gotBody map[string]any

	doGitHubRequest = func(_ string, token, method, path string, body any, _ ...int) ([]byte, int, error) {
		gotToken = token
		gotMethod = method
		gotPath = path
		payload, _ := json.Marshal(body)
		_ = json.Unmarshal(payload, &gotBody)
		return nil, 204, nil
	}

	response, err := Execute(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationDispatchWorkflow,
		Input: map[string]any{
			"repository": "dakasa-co/yggdrasil-api",
			"workflow":   "deploy.yml",
			"inputs": map[string]any{
				"image": "sha256:456",
			},
		},
		Auth: map[string]any{
			"token": "generic-token",
		},
		Integration: protocol.AdapterExecuteIntegrationContext{
			InstanceSpec: protocol.IntegrationInstanceManifestSpec{
				Config: map[string]any{
					"default_ref": "main",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if response.Status != "dispatched" {
		t.Fatalf("status = %q, want dispatched", response.Status)
	}
	if gotToken != "generic-token" {
		t.Fatalf("token = %q, want generic-token", gotToken)
	}
	if gotMethod != "POST" {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/repos/dakasa-co/yggdrasil-api/actions/workflows/deploy.yml/dispatches" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["ref"] != "main" {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestCreateRepositoryUsesOrganizationEndpoint(t *testing.T) {
	previous := doGitHubRequest
	defer func() { doGitHubRequest = previous }()

	var gotPath string
	var gotBody map[string]any

	doGitHubRequest = func(_ string, _ string, method, path string, body any, _ ...int) ([]byte, int, error) {
		if method != "POST" {
			t.Fatalf("method = %q, want POST", method)
		}
		gotPath = path
		payload, _ := json.Marshal(body)
		_ = json.Unmarshal(payload, &gotBody)
		return []byte(`{"full_name":"dakasa-co/new-repo","private":true}`), 201, nil
	}

	response, err := Execute(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationCreateRepository,
		Input: map[string]any{
			"name": "new-repo",
		},
		Auth: map[string]any{
			"token": "github-token",
		},
		Integration: protocol.AdapterExecuteIntegrationContext{
			InstanceSpec: protocol.IntegrationInstanceManifestSpec{
				Config: map[string]any{
					"default_owner":      "dakasa-co",
					"default_visibility": "private",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if response.Status != "created" {
		t.Fatalf("status = %q, want created", response.Status)
	}
	if gotPath != "/orgs/dakasa-co/repos" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["visibility"] != "private" {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestUpsertEnvironmentUsesRepositoryPath(t *testing.T) {
	previous := doGitHubRequest
	defer func() { doGitHubRequest = previous }()

	var gotPath string

	doGitHubRequest = func(_ string, _ string, method, path string, body any, _ ...int) ([]byte, int, error) {
		if method != "PUT" {
			t.Fatalf("method = %q, want PUT", method)
		}
		gotPath = path
		payload, _ := json.Marshal(body)
		if string(payload) == "{}" {
			t.Fatalf("expected non-empty environment body")
		}
		return []byte(`{"name":"production"}`), 200, nil
	}

	_, err := Execute(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationUpsertEnvironment,
		Input: map[string]any{
			"repository":  "dakasa-co/yggdrasil-api",
			"environment": "production",
			"wait_timer":  30,
		},
		Auth: map[string]any{
			"token": "github-token",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotPath != "/repos/dakasa-co/yggdrasil-api/environments/production" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestGrantTeamRepositoryAccessUsesOrganizationScope(t *testing.T) {
	previous := doGitHubRequest
	defer func() { doGitHubRequest = previous }()

	var gotPath string
	var gotBody map[string]any

	doGitHubRequest = func(_ string, _ string, method, path string, body any, _ ...int) ([]byte, int, error) {
		if method != "PUT" {
			t.Fatalf("method = %q, want PUT", method)
		}
		gotPath = path
		payload, _ := json.Marshal(body)
		_ = json.Unmarshal(payload, &gotBody)
		return nil, 204, nil
	}

	_, err := Execute(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationGrantTeamRepositoryAccess,
		Input: map[string]any{
			"team_slug":  "platform",
			"repository": "dakasa-co/yggdrasil-api",
			"permission": "admin",
		},
		Auth: map[string]any{
			"token": "github-token",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotPath != "/orgs/dakasa-co/teams/platform/repos/dakasa-co/yggdrasil-api" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["permission"] != "admin" {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestCatalogDiscoverUsesCustomPropertiesAndTopics(t *testing.T) {
	previous := doGitHubRequest
	defer func() { doGitHubRequest = previous }()

	doGitHubRequest = func(_ string, _ string, method, path string, body any, _ ...int) ([]byte, int, error) {
		if method != "GET" {
			t.Fatalf("method = %q, want GET", method)
		}
		if path != "/orgs/dakasa-yggdrasil/repos?per_page=50&type=all" {
			t.Fatalf("path = %q", path)
		}
		if body != nil {
			t.Fatalf("expected no body for catalog discover")
		}
		return []byte(`[
			{
				"name":"integration-rabbitmq",
				"full_name":"dakasa-yggdrasil/integration-rabbitmq",
				"html_url":"https://github.com/dakasa-yggdrasil/integration-rabbitmq",
				"description":"RabbitMQ governance plugin",
				"default_branch":"main",
				"visibility":"public",
				"topics":["yggdrasil-integration"],
				"custom_properties":{
					"yggdrasil_kind":"integration",
					"yggdrasil_domain":"rabbitmq",
					"yggdrasil_section":"operations",
					"yggdrasil_entry":"api",
					"yggdrasil_display_name":"RabbitMQ"
				}
			},
			{
				"name":"payments-api",
				"full_name":"dakasa-yggdrasil/payments-api",
				"html_url":"https://github.com/dakasa-yggdrasil/payments-api",
				"description":"Payments surface",
				"default_branch":"main",
				"visibility":"private",
				"topics":["yggdrasil-surface"]
			}
		]`), 200, nil
	}

	response, err := Execute(protocol.AdapterExecuteIntegrationRequest{
		Operation: OperationCatalogDiscover,
		Auth: map[string]any{
			"token": "github-token",
		},
		Integration: protocol.AdapterExecuteIntegrationContext{
			InstanceSpec: protocol.IntegrationInstanceManifestSpec{
				Config: map[string]any{
					"catalog_owner": "dakasa-yggdrasil",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, ok := response.Output.(map[string]any)
	if !ok {
		t.Fatalf("output = %#v", response.Output)
	}
	items, ok := output["items"].([]map[string]any)
	if ok {
		if len(items) != 2 {
			t.Fatalf("items = %#v", items)
		}
		return
	}

	raw, _ := json.Marshal(output["items"])
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded = %#v", decoded)
	}
	if decoded[0]["kind"] != "integration" || decoded[1]["kind"] != "surface" {
		t.Fatalf("decoded = %#v", decoded)
	}
}
