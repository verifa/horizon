
variable "project" {
  description = "The name of the project"
  type = object({
    name               = string
    description        = string
    visibility         = string
    version_control    = string
    work_item_template = string
  })
}


variable "azure_subscription" {
  description = "The Azure subscription"
  type = object({
    id   = string
    name = string
  })
}

variable "azuread_application" {
  description = "The Azure AD application"
  type = object({
    client_id = string
    tenant_id = string
  })
}
