terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "3.94.0"
    }
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

provider "azurerm" {
  features {}

  # subscription_id = var.subscription_id
}


#
# VMSS
#
variable "vm_scale_set" {
  description = "VM Scale Set"
  type = object({
    name = string

    resource_group_name = string
    location            = string
    sku                 = string

    subnet_id = string
  })

}

resource "azurerm_linux_virtual_machine_scale_set" "this" {
  name                = var.vm_scale_set.name
  resource_group_name = var.vm_scale_set.resource_group_name
  location            = var.vm_scale_set.location
  sku                 = var.vm_scale_set.sku

  instances = 0
  # TODO: don't hardcode this kinda stuff.
  admin_username = "mike"
  admin_password = "123mikeistheadmin!"
  # Needed for admin_password, remove once using SSH keys.
  disable_password_authentication = false


  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }

  os_disk {
    storage_account_type = "Premium_LRS"
    disk_size_gb         = 30
    caching              = "None"
  }

  network_interface {
    name                 = var.vm_scale_set.name
    primary              = true
    enable_ip_forwarding = true

    ip_configuration {
      name      = var.vm_scale_set.name
      primary   = true
      subnet_id = var.vm_scale_set.subnet_id
    }
  }

  # RANDOM STUFF FROM WORKING VMSS
  overprovision               = false
  platform_fault_domain_count = 0
  secure_boot_enabled         = false
  vtpm_enabled                = false

  # Set by Azure DevOps Elastic Pool
  # TODO: should we ignore these?
  lifecycle {
    ignore_changes = [
      instances,
      automatic_instance_repair,
      automatic_os_upgrade_policy,
      extension,
      scale_in,
      single_placement_group, overprovision, automatic_os_upgrade_policy,
      tags["__AzureDevOpsElasticPool"], tags["__AzureDevOpsElasticPoolTimeStamp"],
    ]
  }
}

