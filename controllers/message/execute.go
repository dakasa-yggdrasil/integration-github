package message

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"go.uber.org/zap"

	"github.com/dakasa-yggdrasil/integration-github/internal/adapter"
	model "github.com/dakasa-yggdrasil/integration-github/internal/protocol"
)

// ExecuteHandler returns an SDK-shaped handler for the execute
// capability. Callers send an AdapterExecuteIntegrationRequest or an
// AdapterDispatchWorkflowRequest; for dispatch_workflow we detect the
// presence of a top-level "workflow" key and route to DispatchWorkflow
// directly, otherwise every operation is routed through Execute which
// handles the full operation switch internally.
func ExecuteHandler(logger *zap.Logger) Handler {
	return func(ctx context.Context, d rpc.Delivery) ([]byte, string, error) {
		var envelope struct {
			Operation  string          `json:"operation"`
			Capability string          `json:"capability,omitempty"`
			Workflow   json.RawMessage `json:"workflow,omitempty"`
		}
		if err := json.Unmarshal(d.Body, &envelope); err != nil {
			return failure("bad_request", err, logger)
		}

		operation := adapter.NormalizeExecuteOperation(envelope.Operation, envelope.Capability)
		capability := adapter.NormalizeExecuteCapability(envelope.Capability, operation)
		if !adapter.SupportsExecuteCapability(capability) {
			return failure("unsupported_capability", fmt.Errorf("unsupported capability %q", envelope.Capability), logger)
		}

		switch operation {
		case adapter.OperationDispatchWorkflow:
			if len(envelope.Workflow) > 0 && string(envelope.Workflow) != "null" {
				var req model.AdapterDispatchWorkflowRequest
				if err := json.Unmarshal(d.Body, &req); err != nil {
					return failure("bad_request", err, logger)
				}
				response, err := adapter.DispatchWorkflow(req)
				if err != nil {
					return failure("dispatch_failed", err, logger)
				}
				return success(response)
			}

			var req model.AdapterExecuteIntegrationRequest
			if err := json.Unmarshal(d.Body, &req); err != nil {
				return failure("bad_request", err, logger)
			}
			response, err := adapter.Execute(req)
			if err != nil {
				return failure("execute_failed", err, logger)
			}
			return success(response)
		default:
			var req model.AdapterExecuteIntegrationRequest
			if err := json.Unmarshal(d.Body, &req); err != nil {
				return failure("bad_request", err, logger)
			}
			response, err := adapter.Execute(req)
			if err != nil {
				return failure("execute_failed", err, logger)
			}
			return success(response)
		}
	}
}
