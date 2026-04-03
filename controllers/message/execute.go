package message

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dakasa-yggdrasil/integration-github/internal/adapter"
	"github.com/dakasa-yggdrasil/integration-github/internal/protocol"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

func executeHandler(conn *amqp.Connection, logger *zap.Logger) ConsumerHandler {
	return func(ctx context.Context, d amqp.Delivery) error {
		var envelope struct {
			Operation  string          `json:"operation"`
			Capability string          `json:"capability,omitempty"`
			Workflow   json.RawMessage `json:"workflow,omitempty"`
			Input      json.RawMessage `json:"input,omitempty"`
		}
		if err := json.Unmarshal(d.Body, &envelope); err != nil {
			return replyFailure(ctx, conn, d, "bad_request", err, logger)
		}

		operation := adapter.NormalizeExecuteOperation(envelope.Operation, envelope.Capability)
		capability := adapter.NormalizeExecuteCapability(envelope.Capability, operation)
		if !adapter.SupportsExecuteCapability(capability) {
			return replyFailure(ctx, conn, d, "unsupported_capability", fmt.Errorf("unsupported capability %q", envelope.Capability), logger)
		}

		switch operation {
		case adapter.OperationDispatchWorkflow:
			if len(envelope.Workflow) > 0 && string(envelope.Workflow) != "null" {
				var req protocol.AdapterDispatchWorkflowRequest
				if err := json.Unmarshal(d.Body, &req); err != nil {
					return replyFailure(ctx, conn, d, "bad_request", err, logger)
				}
				response, err := adapter.DispatchWorkflow(req)
				if err != nil {
					return replyFailure(ctx, conn, d, "dispatch_failed", err, logger)
				}
				return replySuccess(ctx, conn, d, response, logger)
			}

			var req protocol.AdapterExecuteIntegrationRequest
			if err := json.Unmarshal(d.Body, &req); err != nil {
				return replyFailure(ctx, conn, d, "bad_request", err, logger)
			}
			response, err := adapter.Execute(req)
			if err != nil {
				return replyFailure(ctx, conn, d, "execute_failed", err, logger)
			}
			return replySuccess(ctx, conn, d, response, logger)
		default:
			var req protocol.AdapterExecuteIntegrationRequest
			if err := json.Unmarshal(d.Body, &req); err != nil {
				return replyFailure(ctx, conn, d, "bad_request", err, logger)
			}
			response, err := adapter.Execute(req)
			if err != nil {
				return replyFailure(ctx, conn, d, "execute_failed", err, logger)
			}
			return replySuccess(ctx, conn, d, response, logger)
		}
	}
}
