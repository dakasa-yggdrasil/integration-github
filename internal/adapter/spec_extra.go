package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dakasa-yggdrasil/integration-github/internal/protocol"
)

// createRunnerToken issues a fresh registration token for self-hosted
// runners scoped to either an org or a repo. The token is single-use and
// expires after about an hour — callers (cloud-init scripts, kustomize
// jobs) consume it immediately. Org-scoped tokens require admin:org +
// `manage_runners:org` PAT scopes.
func createRunnerToken(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterExecuteIntegrationResponse, error) {
	apiBaseURL, token, instanceConfig, _, err := resolveExecuteConfig(req)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	scope := strings.ToLower(strings.TrimSpace(firstString(req.Input, []string{"scope"})))
	if scope == "" {
		scope = "org"
	}

	var path string
	switch scope {
	case "org":
		org := firstString(req.Input, []string{"org", "organization"})
		if org == "" {
			org = firstString(instanceConfig, []string{"org", "default_owner"})
		}
		if org == "" {
			return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("org is required for scope=org")
		}
		path = "/orgs/" + org + "/actions/runners/registration-token"
	case "repo", "repository":
		repository := resolveRepository(firstString(req.Input, []string{"repository"}), firstString(req.Input, []string{"owner"}), instanceConfig)
		if repository == "" {
			return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("repository is required for scope=repo")
		}
		path = "/repos/" + repository + "/actions/runners/registration-token"
	default:
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("unsupported scope %q (expected org|repo)", scope)
	}

	payload, _, reqErr := doGitHubRequest(apiBaseURL, token, http.MethodPost, path, nil, http.StatusCreated)
	if reqErr != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, reqErr
	}
	var parsed struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("decode runner token response: %w", err)
	}
	if parsed.Token == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("runner token response did not include token")
	}

	return protocol.AdapterExecuteIntegrationResponse{
		Operation:  OperationCreateRunnerToken,
		Capability: OperationCreateRunnerToken,
		Status:     "applied",
		Output: map[string]any{
			"registration_token": parsed.Token,
			"expires_at":         parsed.ExpiresAt,
			"scope":              scope,
		},
		Metadata: map[string]any{
			"provider":     Provider,
			"api_base_url": apiBaseURL,
		},
	}, nil
}

// listRunners lists self-hosted runners. When `expect.min_count` and/or
// `expect.min_online_with_labels` are provided in input, the result is
// gated against those — the workflow uses this to wait until an ASG has
// brought N healthy runners online.
func listRunners(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterExecuteIntegrationResponse, error) {
	apiBaseURL, token, instanceConfig, _, err := resolveExecuteConfig(req)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	scope := strings.ToLower(strings.TrimSpace(firstString(req.Input, []string{"scope"})))
	if scope == "" {
		scope = "org"
	}

	var path string
	switch scope {
	case "org":
		org := firstString(req.Input, []string{"org", "organization"})
		if org == "" {
			org = firstString(instanceConfig, []string{"org", "default_owner"})
		}
		if org == "" {
			return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("org is required for scope=org")
		}
		path = "/orgs/" + org + "/actions/runners?per_page=100"
	case "repo", "repository":
		repository := resolveRepository(firstString(req.Input, []string{"repository"}), firstString(req.Input, []string{"owner"}), instanceConfig)
		if repository == "" {
			return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("repository is required for scope=repo")
		}
		path = "/repos/" + repository + "/actions/runners?per_page=100"
	default:
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("unsupported scope %q (expected org|repo)", scope)
	}

	payload, _, reqErr := doGitHubRequest(apiBaseURL, token, http.MethodGet, path, nil, http.StatusOK)
	if reqErr != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, reqErr
	}
	var parsed struct {
		TotalCount int               `json:"total_count"`
		Runners    []runnerListEntry `json:"runners"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("decode list runners response: %w", err)
	}

	expect, _ := req.Input["expect"].(map[string]any)
	requiredLabels := stringSliceFromAny(expect["min_online_with_labels"])
	minCount := firstInt(expect, []string{"min_count"}, 0)

	matching := 0
	for _, runner := range parsed.Runners {
		if !runner.IsOnline() {
			continue
		}
		if hasAllLabels(runner, requiredLabels) {
			matching++
		}
	}

	if minCount > 0 && matching < minCount {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("only %d runners match labels %v (expected >= %d)", matching, requiredLabels, minCount)
	}

	output := make([]map[string]any, 0, len(parsed.Runners))
	for _, runner := range parsed.Runners {
		labels := make([]string, 0, len(runner.Labels))
		for _, l := range runner.Labels {
			labels = append(labels, l.Name)
		}
		output = append(output, map[string]any{
			"id":     runner.ID,
			"name":   runner.Name,
			"os":     runner.OS,
			"status": runner.Status,
			"busy":   runner.Busy,
			"labels": labels,
		})
	}

	return protocol.AdapterExecuteIntegrationResponse{
		Operation:  OperationListRunners,
		Capability: OperationListRunners,
		Status:     "observed",
		Output: map[string]any{
			"total_count":     parsed.TotalCount,
			"matching_count":  matching,
			"required_labels": requiredLabels,
			"runners":         output,
		},
		Metadata: map[string]any{
			"provider":     Provider,
			"api_base_url": apiBaseURL,
		},
	}, nil
}

type runnerLabel struct {
	Name string `json:"name"`
}

type runnerListEntry struct {
	ID     int           `json:"id"`
	Name   string        `json:"name"`
	OS     string        `json:"os"`
	Status string        `json:"status"`
	Busy   bool          `json:"busy"`
	Labels []runnerLabel `json:"labels"`
}

func (r runnerListEntry) IsOnline() bool {
	return strings.EqualFold(r.Status, "online")
}

func hasAllLabels(runner runnerListEntry, required []string) bool {
	if len(required) == 0 {
		return true
	}
	have := make(map[string]struct{}, len(runner.Labels))
	for _, l := range runner.Labels {
		have[strings.ToLower(l.Name)] = struct{}{}
	}
	for _, want := range required {
		if _, ok := have[strings.ToLower(strings.TrimSpace(want))]; !ok {
			return false
		}
	}
	return true
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
