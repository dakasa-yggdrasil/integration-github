package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/dakasa-yggdrasil/integration-github/internal/protocol"
)

const (
	Provider                           = "github"
	AdapterVersion                     = "1.2.0"
	OperationDispatchWorkflow          = protocol.WorkflowDispatchOperation
	OperationCatalogDiscover           = "catalog_discover"
	OperationCreateRepository          = "create_repository"
	OperationUpsertEnvironment         = "upsert_environment"
	OperationGrantTeamRepositoryAccess = "grant_team_repository_access"
	OperationCreateRunnerToken         = "create_runner_token"
	OperationListRunners               = "list_runners"
	OperationCreatePullRequest         = "create_pull_request"
	DefaultAPIBaseURL                  = "https://api.github.com"
	DefaultRef                         = "main"

	QueueDescribe = "yggdrasil.adapter.github.describe"
	QueueExecute  = "yggdrasil.adapter.github.execute"
)

var SupportedExecuteOperations = []string{
	OperationDispatchWorkflow,
	OperationCatalogDiscover,
	OperationCreateRepository,
	OperationUpsertEnvironment,
	OperationGrantTeamRepositoryAccess,
	OperationCreateRunnerToken,
	OperationListRunners,
	OperationCreatePullRequest,
}

var doGitHubRequest = doGitHubRequestHTTP

// Describe returns the normalized contract exposed by this adapter.
// Transport + addressing mirror what main.go chose at startup via
// YGGDRASIL_TRANSPORT (default http_json). The core uses this to
// verify the adapter's live shape matches the stored
// integration_type manifest — mismatches abort execution before a
// dispatch crosses the wire.
func Describe() protocol.AdapterDescribeResponse {
	transport := strings.ToLower(strings.TrimSpace(os.Getenv("YGGDRASIL_TRANSPORT")))
	if transport == "" {
		transport = "http"
	}
	adapterSpec := protocol.IntegrationAdapterSpec{
		Version:        AdapterVersion,
		TimeoutSeconds: 30,
	}
	switch transport {
	case "amqp", "rabbitmq":
		adapterSpec.Transport = "rabbitmq"
		adapterSpec.Queues = protocol.IntegrationAdapterQueue{
			Describe: QueueDescribe,
			Execute:  QueueExecute,
		}
	default:
		adapterSpec.Transport = "http_json"
		adapterSpec.Endpoints = protocol.IntegrationAdapterRoute{
			Describe: "/rpc/describe",
			Execute:  "/rpc/execute",
		}
	}
	return protocol.AdapterDescribeResponse{
		Provider:     Provider,
		Adapter:      adapterSpec,
		Capabilities: []string{"describe", "execute"},
		CredentialSchema: protocol.IntegrationSchemaSpec{
			Mode: "inline",
			Properties: map[string]protocol.IntegrationSchemaProperty{
				"token": {
					Type:        "string",
					Description: "GitHub token used when the caller does not provide one.",
					Secret:      true,
				},
			},
		},
		InstanceSchema: protocol.IntegrationSchemaSpec{
			Mode: "inline",
			Properties: map[string]protocol.IntegrationSchemaProperty{
				"default_owner": {
					Type:        "string",
					Description: "Default repository owner used when the request omits one.",
				},
				"catalog_owner": {
					Type:        "string",
					Description: "Organization or user scanned when catalog_discover omits one.",
				},
				"catalog_owner_type": {
					Type:        "string",
					Description: "Whether catalog_discover should scan an organization or user scope.",
					Default:     "org",
					Enum:        []any{"org", "user"},
				},
				"default_ref": {
					Type:        "string",
					Description: "Default Git ref used for workflow dispatches.",
					Default:     DefaultRef,
				},
				"default_workflow": {
					Type:        "string",
					Description: "Default workflow filename used when the request omits one.",
				},
				"default_visibility": {
					Type:        "string",
					Description: "Default repository visibility for create_repository.",
					Default:     "private",
				},
				"api_base_url": {
					Type:        "string",
					Description: "GitHub API base URL, useful for GitHub Enterprise.",
					Default:     DefaultAPIBaseURL,
				},
				"base_url": {
					Type:        "string",
					Description: "HTTP adapter base URL used when the integration_type declares transport=http_json.",
				},
			},
		},
		ResourceTypes: []protocol.IntegrationResourceType{
			{
				Name:             "repository",
				CanonicalPrefix:  "thirdparty.github.repository",
				IdentityTemplate: "repository.{repository}",
				Discoverable:     false,
				DefaultActions:   []string{OperationDispatchWorkflow, OperationCreateRepository},
			},
			{
				Name:             "environment",
				CanonicalPrefix:  "thirdparty.github.environment",
				IdentityTemplate: "environment.{repository}.{environment}",
				Discoverable:     false,
				DefaultActions:   []string{OperationUpsertEnvironment},
			},
			{
				Name:             "team_repository_access",
				CanonicalPrefix:  "thirdparty.github.team_repository_access",
				IdentityTemplate: "team_repository_access.{organization}.{team_slug}.{repository}",
				Discoverable:     false,
				DefaultActions:   []string{OperationGrantTeamRepositoryAccess},
			},
			{
				Name:             "catalog_entry",
				CanonicalPrefix:  "thirdparty.github.catalog_entry",
				IdentityTemplate: "catalog_entry.{name}",
				Discoverable:     false,
				DefaultActions:   []string{OperationCatalogDiscover},
			},
			{
				Name:             "runner_registration",
				CanonicalPrefix:  "thirdparty.github.runner_registration",
				IdentityTemplate: "runner_registration.{scope}.{owner}",
				Discoverable:     false,
				DefaultActions:   []string{OperationCreateRunnerToken},
			},
			{
				Name:             "runner_pool",
				CanonicalPrefix:  "thirdparty.github.runner_pool",
				IdentityTemplate: "runner_pool.{scope}.{owner}",
				Discoverable:     true,
				DefaultActions:   []string{OperationListRunners},
			},
		},
		ActionCatalog: describeActionCatalog(),
		Discovery: protocol.IntegrationDiscoverySpec{
			Mode:   "push",
			Cursor: "none",
		},
		Normalization: protocol.IntegrationNormalizationSpec{
			ExternalIDPath:         "full_name",
			NamePath:               "name",
			OwnerPath:              "owner.login",
			FallbackResourcePrefix: "thirdparty.github.custom",
		},
		Execution: protocol.IntegrationExecutionSpec{
			SupportsDryRun: false,
		},
		Extensions: protocol.IntegrationExtensionsSpec{},
	}
}

func NormalizeExecuteOperation(operation string, capability string) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = strings.TrimSpace(capability)
	}
	if operation == "" {
		return OperationDispatchWorkflow
	}
	return operation
}

func NormalizeExecuteCapability(capability string, operation string) string {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return NormalizeExecuteOperation(operation, capability)
	}
	return capability
}

func SupportsExecuteCapability(value string) bool {
	value = strings.TrimSpace(value)
	for _, supported := range SupportedExecuteOperations {
		if value == supported {
			return true
		}
	}
	return value == ""
}

// DispatchWorkflow dispatches one GitHub Actions workflow run.
func DispatchWorkflow(req protocol.AdapterDispatchWorkflowRequest) (protocol.AdapterDispatchWorkflowResponse, error) {
	repository, workflowName, ref, apiBaseURL, token, err := resolveDispatchRequest(req)
	if err != nil {
		return protocol.AdapterDispatchWorkflowResponse{}, err
	}

	body := map[string]any{
		"ref":    ref,
		"inputs": normalizeInput(req.Workflow.Inputs),
	}
	if _, _, err := doGitHubRequest(apiBaseURL, token, http.MethodPost, "/repos/"+repository+"/actions/workflows/"+workflowName+"/dispatches", body, http.StatusNoContent); err != nil {
		return protocol.AdapterDispatchWorkflowResponse{}, err
	}

	workflow := req.Workflow
	workflow.Repository = repository
	workflow.Workflow = workflowName
	workflow.Ref = ref

	return protocol.AdapterDispatchWorkflowResponse{
		Operation: OperationDispatchWorkflow,
		Status:    "dispatched",
		Workflow:  workflow,
		Metadata: map[string]any{
			"provider":     Provider,
			"repository":   repository,
			"workflow":     workflowName,
			"ref":          ref,
			"api_base_url": apiBaseURL,
		},
	}, nil
}

// Execute runs one generic integration operation.
func Execute(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterExecuteIntegrationResponse, error) {
	switch NormalizeExecuteOperation(req.Operation, req.Capability) {
	case "", OperationDispatchWorkflow:
		dispatchReq, err := genericRequestToDispatchWorkflow(req)
		if err != nil {
			return protocol.AdapterExecuteIntegrationResponse{}, err
		}

		response, err := DispatchWorkflow(dispatchReq)
		if err != nil {
			return protocol.AdapterExecuteIntegrationResponse{}, err
		}

		return protocol.AdapterExecuteIntegrationResponse{
			Operation:  OperationDispatchWorkflow,
			Capability: OperationDispatchWorkflow,
			Status:     response.Status,
			Output: map[string]any{
				"repository": response.Workflow.Repository,
				"workflow":   response.Workflow.Workflow,
				"ref":        response.Workflow.Ref,
				"inputs":     response.Workflow.Inputs,
				"metadata":   response.Workflow.Metadata,
			},
			Metadata: response.Metadata,
		}, nil
	case OperationCatalogDiscover:
		return catalogDiscover(req)
	case OperationCreateRepository:
		return createRepository(req)
	case OperationUpsertEnvironment:
		return upsertEnvironment(req)
	case OperationGrantTeamRepositoryAccess:
		return grantTeamRepositoryAccess(req)
	case OperationCreateRunnerToken:
		return createRunnerToken(req)
	case OperationListRunners:
		return listRunners(req)
	case OperationCreatePullRequest:
		return createPullRequest(req)
	default:
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func describeActionCatalog() []protocol.IntegrationActionDefinition {
	return []protocol.IntegrationActionDefinition{
		{
			Name:          OperationDispatchWorkflow,
			Description:   "Dispatch one GitHub Actions workflow run.",
			ResourceTypes: []string{"repository"},
		},
		{
			Name:          OperationCatalogDiscover,
			Description:   "Discover integration and surface candidates from one GitHub owner scope.",
			ResourceTypes: []string{"catalog_entry"},
			Idempotent:    true,
		},
		{
			Name:          OperationCreateRepository,
			Description:   "Create one GitHub repository in a user or organization scope.",
			ResourceTypes: []string{"repository"},
			Idempotent:    false,
		},
		{
			Name:          OperationUpsertEnvironment,
			Description:   "Create or update one GitHub environment on a repository.",
			ResourceTypes: []string{"environment"},
			Idempotent:    true,
		},
		{
			Name:          OperationGrantTeamRepositoryAccess,
			Description:   "Ensure one GitHub team has the desired permission on a repository.",
			ResourceTypes: []string{"team_repository_access"},
			Idempotent:    true,
		},
		{
			Name:          OperationCreateRunnerToken,
			Description:   "Issue a registration token for a self-hosted runner (org or repo scoped).",
			ResourceTypes: []string{"runner_registration"},
			Idempotent:    false,
		},
		{
			Name:          OperationListRunners,
			Description:   "List self-hosted runners with optional label/min-count expectation gating.",
			ResourceTypes: []string{"runner_pool"},
			Idempotent:    true,
		},
		{
			Name:          OperationCreatePullRequest,
			Description:   "Atomically commit a set of files to a feature branch and open a pull request against the base branch. Used by Heimdall to land code-fix proposals automatically while keeping a human-in-the-loop merge.",
			ResourceTypes: []string{"repository"},
			Idempotent:    false,
		},
	}
}

func createRepository(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterExecuteIntegrationResponse, error) {
	apiBaseURL, token, instanceConfig, _, err := resolveExecuteConfig(req)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	owner := firstString(req.Input, []string{"owner"})
	if owner == "" {
		owner = firstString(instanceConfig, []string{"default_owner"})
	}

	name := firstString(req.Input, []string{"name"})
	if name == "" {
		name = lastRepositorySegment(firstString(req.Input, []string{"repository"}))
	}
	if name == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("repository name is required")
	}

	visibility := firstString(req.Input, []string{"visibility"})
	if visibility == "" {
		visibility = firstString(instanceConfig, []string{"default_visibility"})
	}
	if visibility == "" {
		visibility = "private"
	}

	body := map[string]any{
		"name":         name,
		"description":  firstString(req.Input, []string{"description"}),
		"homepage":     firstString(req.Input, []string{"homepage"}),
		"visibility":   visibility,
		"private":      firstBool(req.Input, []string{"private"}, visibility == "private"),
		"auto_init":    firstBool(req.Input, []string{"auto_init"}, false),
		"has_issues":   firstBool(req.Input, []string{"has_issues"}, true),
		"has_projects": firstBool(req.Input, []string{"has_projects"}, true),
		"has_wiki":     firstBool(req.Input, []string{"has_wiki"}, true),
	}

	path := "/user/repos"
	if owner != "" {
		path = "/orgs/" + owner + "/repos"
	}

	var response map[string]any
	payload, _, err := doGitHubRequest(apiBaseURL, token, http.MethodPost, path, body, http.StatusCreated)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("decode create_repository response: %w", err)
	}

	return protocol.AdapterExecuteIntegrationResponse{
		Operation:  OperationCreateRepository,
		Capability: OperationCreateRepository,
		Status:     "created",
		Output:     response,
		Metadata: map[string]any{
			"provider":     Provider,
			"repository":   response["full_name"],
			"visibility":   visibility,
			"api_base_url": apiBaseURL,
		},
	}, nil
}

func upsertEnvironment(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterExecuteIntegrationResponse, error) {
	apiBaseURL, token, _, _, err := resolveExecuteConfig(req)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	repository := resolveRepository(firstString(req.Input, []string{"repository"}), firstString(req.Input, []string{"owner"}), nil)
	if repository == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("repository is required")
	}

	environment := firstString(req.Input, []string{"environment", "name"})
	if environment == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("environment is required")
	}

	body := map[string]any{}
	if waitTimer, ok := req.Input["wait_timer"]; ok {
		body["wait_timer"] = waitTimer
	}
	if reviewers, ok := req.Input["reviewers"]; ok {
		body["reviewers"] = reviewers
	}
	if branchPolicy, ok := req.Input["deployment_branch_policy"]; ok {
		body["deployment_branch_policy"] = branchPolicy
	}

	payload, _, err := doGitHubRequest(apiBaseURL, token, http.MethodPut, "/repos/"+repository+"/environments/"+environment, body, http.StatusOK, http.StatusCreated)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	var response map[string]any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &response); err != nil {
			return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("decode upsert_environment response: %w", err)
		}
	}

	return protocol.AdapterExecuteIntegrationResponse{
		Operation:  OperationUpsertEnvironment,
		Capability: OperationUpsertEnvironment,
		Status:     "ensured",
		Output:     response,
		Metadata: map[string]any{
			"provider":     Provider,
			"repository":   repository,
			"environment":  environment,
			"api_base_url": apiBaseURL,
		},
	}, nil
}

func grantTeamRepositoryAccess(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterExecuteIntegrationResponse, error) {
	apiBaseURL, token, instanceConfig, _, err := resolveExecuteConfig(req)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	repository := resolveRepository(
		firstString(req.Input, []string{"repository"}),
		firstString(req.Input, []string{"repository_owner"}),
		instanceConfig,
	)
	if repository == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("repository is required")
	}

	teamSlug := firstString(req.Input, []string{"team_slug"})
	if teamSlug == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("team_slug is required")
	}

	organization := firstString(req.Input, []string{"organization", "owner"})
	if organization == "" {
		organization = firstString(instanceConfig, []string{"default_owner"})
	}
	if organization == "" {
		organization = repositoryOwner(repository)
	}
	if organization == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("organization is required")
	}

	permission := firstString(req.Input, []string{"permission"})
	if permission == "" {
		permission = "push"
	}

	body := map[string]any{"permission": permission}
	if _, _, err := doGitHubRequest(apiBaseURL, token, http.MethodPut, "/orgs/"+organization+"/teams/"+teamSlug+"/repos/"+repository, body, http.StatusNoContent); err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	return protocol.AdapterExecuteIntegrationResponse{
		Operation:  OperationGrantTeamRepositoryAccess,
		Capability: OperationGrantTeamRepositoryAccess,
		Status:     "ensured",
		Output: map[string]any{
			"organization": organization,
			"team_slug":    teamSlug,
			"repository":   repository,
			"permission":   permission,
		},
		Metadata: map[string]any{
			"provider":     Provider,
			"api_base_url": apiBaseURL,
		},
	}, nil
}

func catalogDiscover(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterExecuteIntegrationResponse, error) {
	apiBaseURL, token, instanceConfig, _, err := resolveExecuteConfig(req)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	owner := firstString(req.Input, []string{"owner", "organization", "catalog_owner"})
	if owner == "" {
		owner = firstString(instanceConfig, []string{"catalog_owner", "default_owner"})
	}
	if owner == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("catalog owner is required")
	}

	ownerType := strings.ToLower(strings.TrimSpace(firstString(req.Input, []string{"owner_type", "catalog_owner_type"})))
	if ownerType == "" {
		ownerType = strings.ToLower(strings.TrimSpace(firstString(instanceConfig, []string{"catalog_owner_type"})))
	}
	if ownerType == "" {
		ownerType = "org"
	}
	if ownerType != "org" && ownerType != "user" {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("catalog owner_type %q is unsupported", ownerType)
	}

	limit := firstInt(req.Input, []string{"limit"}, 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	path := fmt.Sprintf("/orgs/%s/repos?per_page=%d&type=all", owner, limit)
	if ownerType == "user" {
		path = fmt.Sprintf("/users/%s/repos?per_page=%d&type=owner", owner, limit)
	}

	payload, _, err := doGitHubRequest(apiBaseURL, token, http.MethodGet, path, nil, http.StatusOK)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	var repositories []map[string]any
	if err := json.Unmarshal(payload, &repositories); err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("decode catalog_discover repositories: %w", err)
	}

	kinds := normalizeDiscoverKinds(req.Input["kinds"])
	query := strings.ToLower(strings.TrimSpace(firstString(req.Input, []string{"query"})))

	items := make([]map[string]any, 0, len(repositories))
	for _, repository := range repositories {
		item := catalogItemFromRepository(repository)
		if item == nil {
			continue
		}
		if len(kinds) > 0 && !slicesContainsString(kinds, firstString(item, []string{"kind"})) {
			continue
		}
		if query != "" && !catalogItemMatchesQuery(item, query) {
			continue
		}
		items = append(items, item)
	}

	return protocol.AdapterExecuteIntegrationResponse{
		Operation:  OperationCatalogDiscover,
		Capability: OperationCatalogDiscover,
		Status:     "succeeded",
		Output: map[string]any{
			"items": items,
		},
		Metadata: map[string]any{
			"provider":      Provider,
			"catalog_owner": owner,
			"owner_type":    ownerType,
			"api_base_url":  apiBaseURL,
			"count":         len(items),
		},
	}, nil
}

func genericRequestToDispatchWorkflow(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterDispatchWorkflowRequest, error) {
	dispatchReq := protocol.AdapterDispatchWorkflowRequest{
		Operation:  OperationDispatchWorkflow,
		Capability: OperationDispatchWorkflow,
		Auth: protocol.WorkflowDispatchAuth{
			Token: firstString(req.Auth, []string{"token", "github_token"}),
		},
		Integration: protocol.AdapterDispatchWorkflowIntegrationContext{
			Type:         req.Integration.Type,
			TypeSpec:     req.Integration.TypeSpec,
			Instance:     req.Integration.Instance,
			InstanceSpec: req.Integration.InstanceSpec,
		},
	}

	dispatchReq.Workflow.Repository = firstString(req.Input, []string{"repository"})
	dispatchReq.Workflow.Workflow = firstString(req.Input, []string{"workflow"})
	dispatchReq.Workflow.Ref = firstString(req.Input, []string{"ref"})
	dispatchReq.Workflow.Metadata = mapStringAny(req.Input["metadata"])
	dispatchReq.Workflow.Inputs = mapStringAny(req.Input["inputs"])

	if componentID := firstString(req.Input, []string{"component_id"}); componentID != "" {
		dispatchReq.Workflow.ComponentID = componentID
	}

	return dispatchReq, nil
}

func resolveDispatchRequest(req protocol.AdapterDispatchWorkflowRequest) (repository string, workflowName string, ref string, apiBaseURL string, token string, err error) {
	instanceConfig := req.Integration.InstanceSpec.Config
	instanceCredentials := req.Integration.InstanceSpec.Credentials

	repository = resolveRepository(strings.TrimSpace(req.Workflow.Repository), "", instanceConfig)
	if repository == "" {
		return "", "", "", "", "", fmt.Errorf("workflow repository is required")
	}

	workflowName = strings.TrimSpace(req.Workflow.Workflow)
	if workflowName == "" {
		workflowName = firstString(instanceConfig, []string{"default_workflow"})
	}
	if workflowName == "" {
		return "", "", "", "", "", fmt.Errorf("workflow name is required")
	}

	ref = strings.TrimSpace(req.Workflow.Ref)
	if ref == "" {
		ref = firstString(instanceConfig, []string{"default_ref"})
	}
	if ref == "" {
		ref = DefaultRef
	}

	apiBaseURL = firstString(instanceConfig, []string{"api_base_url"})
	if apiBaseURL == "" {
		apiBaseURL = DefaultAPIBaseURL
	}

	token = strings.TrimSpace(req.Auth.Token)
	if token == "" {
		token = firstString(instanceCredentials, []string{"token", "github_token"})
	}
	if token == "" {
		return "", "", "", "", "", fmt.Errorf("github token is required")
	}

	return repository, workflowName, ref, strings.TrimRight(apiBaseURL, "/"), token, nil
}

func resolveExecuteConfig(req protocol.AdapterExecuteIntegrationRequest) (apiBaseURL string, token string, instanceConfig map[string]any, instanceCredentials map[string]any, err error) {
	instanceConfig = req.Integration.InstanceSpec.Config
	instanceCredentials = req.Integration.InstanceSpec.Credentials

	apiBaseURL = firstString(instanceConfig, []string{"api_base_url"})
	if apiBaseURL == "" {
		apiBaseURL = DefaultAPIBaseURL
	}

	token = firstString(req.Auth, []string{"token", "github_token"})
	if token == "" {
		token = firstString(instanceCredentials, []string{"token", "github_token"})
	}
	if token == "" {
		err = fmt.Errorf("github token is required")
		return
	}

	apiBaseURL = strings.TrimRight(apiBaseURL, "/")
	return
}

func resolveRepository(repository string, owner string, instanceConfig map[string]any) string {
	defaultRepository := firstString(instanceConfig, []string{"default_repository"})
	defaultOwner := firstString(instanceConfig, []string{"default_owner"})
	repository = normalizeRepository(strings.TrimSpace(repository), defaultRepository, owner)
	if repository == "" {
		repository = normalizeRepository(strings.TrimSpace(repository), defaultRepository, defaultOwner)
	}
	return repository
}

func normalizeRepository(workflowRepository string, defaultRepository string, defaultOwner string) string {
	for _, candidate := range []string{workflowRepository, defaultRepository} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, "/") {
			return candidate
		}
		if strings.TrimSpace(defaultOwner) != "" {
			return strings.TrimSpace(defaultOwner) + "/" + candidate
		}
		return candidate
	}

	return ""
}

func normalizeInput(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	return input
}

func mapStringAny(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return typed
}

func firstInt(source map[string]any, keys []string, fallback int) int {
	for _, key := range keys {
		if source == nil {
			continue
		}
		value, ok := source[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case int:
			return typed
		case int32:
			return int(typed)
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return int(parsed)
			}
		case string:
			parsed := 0
			if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func firstString(source map[string]any, keys []string) string {
	for _, key := range keys {
		if source == nil {
			continue
		}
		value, ok := source[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func firstBool(source map[string]any, keys []string, fallback bool) bool {
	for _, key := range keys {
		if source == nil {
			continue
		}
		value, ok := source[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "1", "yes":
				return true
			case "false", "0", "no":
				return false
			}
		}
	}
	return fallback
}

func repositoryOwner(repository string) string {
	parts := strings.Split(strings.TrimSpace(repository), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

func lastRepositorySegment(repository string) string {
	parts := strings.Split(strings.TrimSpace(repository), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func normalizeDiscoverKinds(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}

	kinds := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		kind := strings.ToLower(strings.TrimSpace(fmt.Sprint(item)))
		switch kind {
		case "integration", "surface":
		default:
			continue
		}
		if _, exists := seen[kind]; exists {
			continue
		}
		seen[kind] = struct{}{}
		kinds = append(kinds, kind)
	}
	return kinds
}

func slicesContainsString(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func catalogItemMatchesQuery(item map[string]any, query string) bool {
	for _, value := range []string{
		firstString(item, []string{"kind"}),
		firstString(item, []string{"name"}),
		firstString(item, []string{"display_name"}),
		firstString(item, []string{"description"}),
		firstString(item, []string{"domain"}),
		firstString(item, []string{"section"}),
		firstString(item, []string{"entry"}),
		firstString(item, []string{"repository"}),
	} {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), query) {
			return true
		}
	}
	return false
}

func catalogItemFromRepository(repository map[string]any) map[string]any {
	customProperties := mapStringAny(repository["custom_properties"])
	topics := stringSlice(repository["topics"])

	kind := firstString(customProperties, []string{"yggdrasil_kind"})
	if kind == "" {
		switch {
		case stringSliceContains(topics, "yggdrasil-integration"):
			kind = "integration"
		case stringSliceContains(topics, "yggdrasil-surface"):
			kind = "surface"
		}
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "integration" && kind != "surface" {
		return nil
	}

	repoName := firstString(repository, []string{"name"})
	displayName := firstString(customProperties, []string{"yggdrasil_display_name"})
	if displayName == "" {
		displayName = humanizeCatalogName(repoName)
	}

	item := map[string]any{
		"kind":         kind,
		"name":         deriveCatalogName(kind, repoName),
		"display_name": displayName,
		"description":  firstString(repository, []string{"description"}),
		"repository":   firstString(repository, []string{"html_url"}),
		"metadata": map[string]any{
			"topics":            topics,
			"custom_properties": customProperties,
			"full_name":         firstString(repository, []string{"full_name"}),
			"default_branch":    firstString(repository, []string{"default_branch"}),
			"visibility":        firstString(repository, []string{"visibility"}),
			"archived":          repository["archived"],
		},
	}

	if kind == "integration" {
		domain := firstString(customProperties, []string{"yggdrasil_domain"})
		section := firstString(customProperties, []string{"yggdrasil_section"})
		entry := firstString(customProperties, []string{"yggdrasil_entry"})

		if domain == "" || section == "" || entry == "" {
			fallbackDomain, fallbackSection, fallbackEntry := deriveIntegrationCatalogPositionFromRepository(repoName)
			if domain == "" {
				domain = fallbackDomain
			}
			if section == "" {
				section = fallbackSection
			}
			if entry == "" {
				entry = fallbackEntry
			}
		}

		item["domain"] = domain
		item["section"] = section
		item["entry"] = entry
	}

	if kind == "surface" {
		namespace := firstString(customProperties, []string{"yggdrasil_namespace"})
		if namespace == "" {
			namespace = "global"
		}
		item["namespace"] = namespace
	}

	return item
}

func deriveCatalogName(kind, repoName string) string {
	repoName = strings.TrimSpace(repoName)
	if kind == "integration" {
		return strings.TrimPrefix(repoName, "integration-")
	}
	return repoName
}

func deriveIntegrationCatalogPositionFromRepository(repoName string) (string, string, string) {
	name := strings.TrimPrefix(strings.TrimSpace(repoName), "integration-")
	if strings.Contains(name, "-on-") {
		left, right, _ := strings.Cut(name, "-on-")
		return left, "installations", right
	}
	return name, "operations", "api"
}

func humanizeCatalogName(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "integration-") {
		value = strings.TrimPrefix(value, "integration-")
	}
	value = strings.ReplaceAll(value, "-", " ")
	parts := strings.Fields(value)
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text == "" || text == "<nil>" {
				continue
			}
			items = append(items, text)
		}
		return items
	default:
		return nil
	}
}

func stringSliceContains(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func doGitHubRequestHTTP(apiBaseURL, token, method, path string, body any, expectedStatus ...int) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal github payload: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	url := strings.TrimRight(apiBaseURL, "/") + path
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("build github request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "integration-github")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("call github api %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	for _, status := range expectedStatus {
		if resp.StatusCode == status {
			return payload, resp.StatusCode, nil
		}
	}

	message := strings.TrimSpace(string(payload))
	if message == "" {
		message = resp.Status
	}
	return payload, resp.StatusCode, fmt.Errorf("github api %s %s failed with status %s: %s", method, path, resp.Status, message)
}
