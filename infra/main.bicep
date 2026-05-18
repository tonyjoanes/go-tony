@description('Azure region for all resources')
param location string = resourceGroup().location

@description('Name of the App Service Plan')
param planName string = 'demo-asp'

@description('Name of the App Service')
param appName string = 'demo-myapi'

// ── App Service Plan ──────────────────────────────────────────────────────────
resource appServicePlan 'Microsoft.Web/serverfarms@2023-12-01' = {
  name: planName
  location: location
  sku: {
    name: 'F1'
    tier: 'Free'
  }
  properties: {}
}

// ── App Service ───────────────────────────────────────────────────────────────
resource appService 'Microsoft.Web/sites@2023-12-01' = {
  name: appName
  location: location
  properties: {
    serverFarmId: appServicePlan.id
    siteConfig: {
      netFrameworkVersion: 'v8.0'
      healthCheckPath: '/health'
    }
    httpsOnly: true
  }
}

// ── Outputs ───────────────────────────────────────────────────────────────────
output appServiceName string = appService.name
output defaultHostname string = appService.properties.defaultHostName
