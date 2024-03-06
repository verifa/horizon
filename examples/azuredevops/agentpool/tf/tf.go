package tf

type Vars struct {
	AgentPool *AgentPool `json:"agent_pool,omitempty"`
}

type AgentPool struct {
	Name                string `json:"name,omitempty"`
	ProjectID           string `json:"project_id,omitempty"`
	VMScaleSetID        string `json:"vm_scale_set_id,omitempty"`
	ServiceConnectionID string `json:"service_connection_id,omitempty"`
}
