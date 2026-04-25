package protocol

import "github.com/google/uuid"

const WorkflowDispatchOperation = "dispatch_workflow"

type ManifestSelector struct {
	ManifestID string `json:"manifest_id,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	Version    *int   `json:"version,omitempty"`
}

type ManifestReference struct {
	ID        uuid.UUID `json:"id"`
	Kind      string    `json:"kind"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
}

type IntegrationAdapterSpec struct {
	Transport      string                  `json:"transport"`
	Version        string                  `json:"version"`
	Queues         IntegrationAdapterQueue `json:"queues,omitempty"`
	Endpoints      IntegrationAdapterRoute `json:"endpoints,omitempty"`
	TimeoutSeconds int                     `json:"timeout_seconds,omitempty"`
}

// IntegrationAdapterRoute mirrors the core's http_json endpoint
// addressing: path (relative) per capability. Populated instead of
// Queues when Transport is "http_json".
type IntegrationAdapterRoute struct {
	Describe string `json:"describe,omitempty"`
	Discover string `json:"discover,omitempty"`
	Read     string `json:"read,omitempty"`
	Execute  string `json:"execute,omitempty"`
	Sync     string `json:"sync,omitempty"`
	Health   string `json:"health,omitempty"`
}

type IntegrationAdapterQueue struct {
	Describe string `json:"describe,omitempty"`
	Discover string `json:"discover,omitempty"`
	Read     string `json:"read,omitempty"`
	Execute  string `json:"execute,omitempty"`
	Sync     string `json:"sync,omitempty"`
	Health   string `json:"health,omitempty"`
}

type IntegrationSchemaSpec struct {
	Mode       string                               `json:"mode"`
	Required   []string                             `json:"required,omitempty"`
	Properties map[string]IntegrationSchemaProperty `json:"properties,omitempty"`
}

type IntegrationSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
	Default     any    `json:"default,omitempty"`
}

type IntegrationResourceType struct {
	Name             string   `json:"name"`
	CanonicalPrefix  string   `json:"canonical_prefix"`
	IdentityTemplate string   `json:"identity_template"`
	Discoverable     bool     `json:"discoverable"`
	DefaultActions   []string `json:"default_actions"`
}

type IntegrationActionDefinition struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	ResourceTypes []string `json:"resource_types,omitempty"`
	Idempotent    bool     `json:"idempotent,omitempty"`
}

type IntegrationDiscoverySpec struct {
	Mode             string `json:"mode"`
	Cursor           string `json:"cursor,omitempty"`
	SupportsWebhooks bool   `json:"supports_webhooks,omitempty"`
}

type IntegrationNormalizationSpec struct {
	ExternalIDPath         string `json:"external_id_path"`
	NamePath               string `json:"name_path,omitempty"`
	OwnerPath              string `json:"owner_path,omitempty"`
	FallbackResourcePrefix string `json:"fallback_resource_prefix"`
}

type IntegrationExecutionSpec struct {
	SupportsDryRun    bool     `json:"supports_dry_run,omitempty"`
	IdempotentActions []string `json:"idempotent_actions,omitempty"`
}

type IntegrationExtensionsSpec struct {
	AllowCustomResourceTypes bool `json:"allow_custom_resource_types,omitempty"`
	AllowCustomActions       bool `json:"allow_custom_actions,omitempty"`
	PreserveRawPayload       bool `json:"preserve_raw_payload,omitempty"`
}

type IntegrationTypeManifestSpec struct {
	Provider         string                        `json:"provider"`
	Adapter          IntegrationAdapterSpec        `json:"adapter"`
	Capabilities     []string                      `json:"capabilities"`
	CredentialSchema IntegrationSchemaSpec         `json:"credential_schema"`
	InstanceSchema   IntegrationSchemaSpec         `json:"instance_schema"`
	ResourceTypes    []IntegrationResourceType     `json:"resource_types"`
	ActionCatalog    []IntegrationActionDefinition `json:"action_catalog,omitempty"`
	Discovery        IntegrationDiscoverySpec      `json:"discovery"`
	Normalization    IntegrationNormalizationSpec  `json:"normalization"`
	Execution        IntegrationExecutionSpec      `json:"execution"`
	Extensions       IntegrationExtensionsSpec     `json:"extensions"`
}

type IntegrationInstanceDiscoverySpec struct {
	Enabled             bool   `json:"enabled"`
	Mode                string `json:"mode,omitempty"`
	SyncIntervalSeconds int    `json:"sync_interval_seconds,omitempty"`
}

type IntegrationInstanceExecutionSpec struct {
	DefaultDryRun bool `json:"default_dry_run,omitempty"`
	MaxBatchSize  int  `json:"max_batch_size,omitempty"`
}

type IntegrationInstanceManifestSpec struct {
	TypeRef     ManifestSelector                 `json:"type_ref"`
	Status      string                           `json:"status,omitempty"`
	Owners      []string                         `json:"owners,omitempty"`
	Credentials map[string]any                   `json:"credentials,omitempty"`
	Config      map[string]any                   `json:"config,omitempty"`
	Discovery   IntegrationInstanceDiscoverySpec `json:"discovery"`
	Execution   IntegrationInstanceExecutionSpec `json:"execution,omitempty"`
}

type AdapterDescribeRequest struct {
	Provider        string `json:"provider"`
	ExpectedVersion string `json:"expected_version,omitempty"`
}

type AdapterDescribeResponse struct {
	Provider         string                        `json:"provider"`
	Adapter          IntegrationAdapterSpec        `json:"adapter"`
	Capabilities     []string                      `json:"capabilities"`
	CredentialSchema IntegrationSchemaSpec         `json:"credential_schema"`
	InstanceSchema   IntegrationSchemaSpec         `json:"instance_schema"`
	ResourceTypes    []IntegrationResourceType     `json:"resource_types"`
	ActionCatalog    []IntegrationActionDefinition `json:"action_catalog,omitempty"`
	Discovery        IntegrationDiscoverySpec      `json:"discovery"`
	Normalization    IntegrationNormalizationSpec  `json:"normalization"`
	Execution        IntegrationExecutionSpec      `json:"execution"`
	Extensions       IntegrationExtensionsSpec     `json:"extensions"`
}

type AdapterExecuteIntegrationContext struct {
	Type         ManifestReference               `json:"type"`
	TypeSpec     IntegrationTypeManifestSpec     `json:"type_spec"`
	Instance     ManifestReference               `json:"instance"`
	InstanceSpec IntegrationInstanceManifestSpec `json:"instance_spec"`
}

type AdapterExecuteIntegrationRequest struct {
	Operation   string                           `json:"operation"`
	Capability  string                           `json:"capability,omitempty"`
	Input       map[string]any                   `json:"input,omitempty"`
	Auth        map[string]any                   `json:"auth,omitempty"`
	Metadata    map[string]any                   `json:"metadata,omitempty"`
	Integration AdapterExecuteIntegrationContext `json:"integration"`
}

type AdapterExecuteIntegrationResponse struct {
	Operation  string         `json:"operation,omitempty"`
	Capability string         `json:"capability,omitempty"`
	Status     string         `json:"status,omitempty"`
	Output     any            `json:"output,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type WorkflowDispatchAuth struct {
	Token string `json:"token,omitempty"`
}

type WorkflowDispatchSpec struct {
	ComponentID string         `json:"component_id,omitempty"`
	Repository  string         `json:"repository"`
	Workflow    string         `json:"workflow"`
	Ref         string         `json:"ref,omitempty"`
	Inputs      map[string]any `json:"inputs,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type AdapterDispatchWorkflowIntegrationContext struct {
	Type         ManifestReference               `json:"type"`
	TypeSpec     IntegrationTypeManifestSpec     `json:"type_spec"`
	Instance     ManifestReference               `json:"instance"`
	InstanceSpec IntegrationInstanceManifestSpec `json:"instance_spec"`
}

type AdapterDispatchWorkflowRequest struct {
	Operation   string                                    `json:"operation"`
	Capability  string                                    `json:"capability,omitempty"`
	Workflow    WorkflowDispatchSpec                      `json:"workflow"`
	Auth        WorkflowDispatchAuth                      `json:"auth,omitempty"`
	Integration AdapterDispatchWorkflowIntegrationContext `json:"integration"`
}

type AdapterDispatchWorkflowResponse struct {
	Operation string               `json:"operation,omitempty"`
	Status    string               `json:"status"`
	Workflow  WorkflowDispatchSpec `json:"workflow,omitempty"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
}

// AdapterOperationStatusResponse is a minimal response for side-effect-only
// operations that produce no domain output beyond status. Named consistently
// with AdapterDispatchWorkflowResponse and AdapterExecuteIntegrationResponse.
type AdapterOperationStatusResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}
