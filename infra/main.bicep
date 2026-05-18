@description('Azure region for all resources')
param location string = resourceGroup().location

@description('Name of the Container App')
param appName string = 'demo-myapi-dev'

@description('Azure Container Registry name (globally unique, alphanumeric)')
param acrName string = 'acr${uniqueString(resourceGroup().id)}'

// Azure Container Registry (Basic SKU supports ACR Tasks for remote image builds)
resource acr 'Microsoft.ContainerRegistry/registries@2023-07-01' = {
  name: acrName
  location: location
  sku: {
    name: 'Basic'
  }
  properties: {
    adminUserEnabled: true
  }
}

// Container Apps Environment - Consumption plan, no VM quota required
resource containerEnv 'Microsoft.App/managedEnvironments@2023-05-01' = {
  name: '${appName}-env'
  location: location
  properties: {}
}

output acrLoginServer string = acr.properties.loginServer
output acrName string = acr.name
output containerEnvName string = containerEnv.name
