package terra

type Backender interface {
	Backend() string
}

var _ Backender = (*BackendAzureRM)(nil)

type BackendAzureRM struct {
	ResourceGroupName  string `json:"resource_group_name,omitempty"`
	StorageAccountName string `json:"storage_account_name,omitempty"`
	ContainerName      string `json:"container_name,omitempty"`
	Key                string `json:"key,omitempty"`
}

func (b BackendAzureRM) Backend() string {
	return "azurerm"
}

var _ Backender = (*BackendLocal)(nil)

type BackendLocal struct {
	Path string `json:"path,omitempty"`
}

func (b BackendLocal) Backend() string {
	return "local"
}
