package models

type ExecutionPlan struct {
	WorkflowName string               `json:"workflow_name,omitempty"`
	Entries      []ExecutionPlanEntry `json:"entries,omitempty"`
}

func (p *ExecutionPlan) IsValid() bool {
	return p != nil && len(p.Entries) > 0
}

type ExecutionPlanEntry struct {
	EntryID          string                `json:"entry_id,omitempty"`
	ProviderName     string                `json:"provider_name,omitempty"`
	DeviceID         string                `json:"device_id,omitempty"`
	AuthorizeRequest *AuthorizeRoleRequest `json:"authorize_request,omitempty"`
}

type ExecutionPlanRequest struct {
	WorkflowID     string                  `json:"workflow_id,omitempty"`
	ElevateRequest *ElevateRequestInternal `json:"elevate_request,omitempty"`
}

func CloneWorkflowRoleRequest(req *WorkflowRoleRequest) *WorkflowRoleRequest {
	if req == nil {
		return nil
	}

	clone := *req
	if req.Role != nil {
		clone.Role = CloneRole(req.Role)
	}
	if req.Duration != nil {
		duration := *req.Duration
		clone.Duration = &duration
	}
	if req.Metadata != nil {
		clone.Metadata = make(map[string]any, len(req.Metadata))
		for key, value := range req.Metadata {
			clone.Metadata[key] = value
		}
	}

	return &clone
}

func CloneExecutionPlan(plan *ExecutionPlan) *ExecutionPlan {
	if plan == nil {
		return nil
	}

	clone := &ExecutionPlan{
		WorkflowName: plan.WorkflowName,
		Entries:      make([]ExecutionPlanEntry, 0, len(plan.Entries)),
	}

	for _, entry := range plan.Entries {
		clone.Entries = append(clone.Entries, ExecutionPlanEntry{
			EntryID:          entry.EntryID,
			ProviderName:     entry.ProviderName,
			DeviceID:         entry.DeviceID,
			AuthorizeRequest: CloneAuthorizeRoleRequest(entry.AuthorizeRequest),
		})
	}

	return clone
}
