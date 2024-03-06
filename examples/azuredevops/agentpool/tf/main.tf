terraform {
  required_providers {
    azuredevops = {
      source  = "microsoft/azuredevops"
      version = "1.0.0"
    }
  }
}

provider "azuredevops" {
  # use_msi         = true
  org_service_url = "https://dev.azure.com/verifa-hz"
}

variable "agent_pool" {
  description = "Agent Pool"
  type = object({
    name = string

    project_id            = string
    vm_scale_set_id       = string
    service_connection_id = string
  })
}

data "azuredevops_project" "this" {
  project_id = var.agent_pool.project_id
}

#
# Agent Pool
#
resource "azuredevops_elastic_pool" "this" {
  name                   = format("%s-%s", data.azuredevops_project.this.name, var.agent_pool.name)
  service_endpoint_id    = var.agent_pool.service_connection_id
  service_endpoint_scope = var.agent_pool.project_id
  # project_id             = var.agent_pool.project_id
  desired_idle           = 0
  max_capacity           = 3
  agent_interactive_ui   = false
  recycle_after_each_use = false
  time_to_live_minutes   = 30
  azure_resource_id      = var.agent_pool.vm_scale_set_id
  auto_provision         = true
  auto_update            = true
}

resource "azuredevops_agent_queue" "this" {
  project_id    = var.agent_pool.project_id
  agent_pool_id = azuredevops_elastic_pool.this.id
}

resource "azuredevops_pipeline_authorization" "this" {
  project_id  = var.agent_pool.project_id
  resource_id = azuredevops_agent_queue.this.id
  type        = "queue"
}
