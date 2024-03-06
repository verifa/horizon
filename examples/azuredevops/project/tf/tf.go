package tf

import "github.com/verifa/horizon/examples/azuredevops/terra"

var DefaultTFVars = Vars{
	Project: &Project{
		Name:             "test-project",
		Description:      "Test project",
		Visibility:       "private",
		VersionControl:   "Git",
		WorkItemTemplate: "Basic",
	},

	Subscription: &AzureSubscription{
		ID:   "12749df0-9a8e-44cd-889e-4740be851c13",
		Name: "verifa-main",
	},

	Application: &AzureADApplication{
		ClientID: "2a57f5af-ba13-481d-a17a-425c16dec0a6",
		TenantID: "449f7bd6-b339-4333-8b85-cd0b8fc37aa6",
	},
}

type Vars struct {
	Project      *Project            `json:"project,omitempty"`
	Subscription *AzureSubscription  `json:"azure_subscription,omitempty"`
	Application  *AzureADApplication `json:"azuread_application,omitempty"`
}

type Project struct {
	Name             string `json:"name,omitempty"`
	Description      string `json:"description,omitempty"`
	Visibility       string `json:"visibility,omitempty"`
	VersionControl   string `json:"version_control,omitempty"`
	WorkItemTemplate string `json:"work_item_template,omitempty"`
}

type AzureSubscription struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type AzureADApplication struct {
	ClientID string `json:"client_id,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
}

//
// Outputs
//

var _ terra.Outputer = (*OutputProject)(nil)

type OutputProject struct {
	ID string `json:"id"`
}

func (p *OutputProject) Name() string {
	return "project"
}

func (p *OutputProject) Resource() string {
	return "azuredevops_project.this"
}

var _ terra.Outputer = (*OutputServiceConnection)(nil)

type OutputServiceConnection struct {
	ID string `json:"id"`
}

func (p *OutputServiceConnection) Name() string {
	return "serviceconnection"
}

func (p *OutputServiceConnection) Resource() string {
	return "azuredevops_serviceendpoint_azurerm.this"
}
