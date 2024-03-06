terraform {
  required_providers {
    azuredevops = {
      source  = "microsoft/azuredevops"
      version = "1.0.0"
    }
    azuread = {
      source  = "hashicorp/azuread"
      version = "2.47.0"
    }
  }
}

provider "azuredevops" {
  # use_msi         = true
  org_service_url = "https://dev.azure.com/verifa-hz"
}

provider "azuread" {
  # Configuration options
}







#
# Azure DevOps project
#
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
resource "azuredevops_project" "this" {
  name               = var.project.name
  description        = var.project.description
  visibility         = var.project.visibility
  version_control    = var.project.version_control
  work_item_template = var.project.work_item_template
}

#
# Create the Azure DevOps service connection.
#
variable "azure_subscription" {
  description = "The Azure subscription"
  type = object({
    id   = string
    name = string
  })
}
resource "azuredevops_serviceendpoint_azurerm" "this" {
  project_id                             = azuredevops_project.this.id
  service_endpoint_name                  = var.project.name
  description                            = "Managed by Horizon"
  service_endpoint_authentication_scheme = "WorkloadIdentityFederation"
  credentials {
    serviceprincipalid = var.azuread_application.client_id
  }
  azurerm_spn_tenantid      = var.azuread_application.tenant_id
  azurerm_subscription_id   = var.azure_subscription.id
  azurerm_subscription_name = var.azure_subscription.name
}


# Maybe we could use this to get the tenant id?
# data "azurerm_client_config" "current" {}

variable "azuread_application" {
  description = "The Azure AD application"
  type = object({
    client_id = string
    tenant_id = string
  })
}
data "azuread_application" "this" {
  client_id = var.azuread_application.client_id

}

#
# Federated identity credential to "trust" the Azure DevOps service connection.
#
resource "azuread_application_federated_identity_credential" "this" {
  application_id = data.azuread_application.this.id
  display_name   = format("azure-devops-project-%s", var.project.name)
  description    = format("Azure DevOps Project: %s", var.project.name)
  audiences      = ["api://AzureADTokenExchange"]
  issuer         = azuredevops_serviceendpoint_azurerm.this.workload_identity_federation_issuer
  subject        = azuredevops_serviceendpoint_azurerm.this.workload_identity_federation_subject
}


