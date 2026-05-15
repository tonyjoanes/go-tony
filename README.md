# Azure Health Check Demo

A minimal end-to-end demo showing a production-style post-deploy validation pattern:

- A **.NET 8 Web API** with a `/health` endpoint deployed to Azure App Service (Linux)
- **Bicep IaC** to provision the App Service Plan and App Service
- A **Go CLI tool** (`azhealthcheck`) that polls the health endpoint after deployment until it returns HTTP 200
- An **Azure DevOps pipeline** wiring all three together

## What It Does

The pipeline builds the API, provisions Azure infrastructure with Bicep, deploys the app, then runs `azhealthcheck` to confirm the deployment is live and healthy before the stage reports success. If the app never becomes healthy within the timeout, the pipeline fails fast.

```
Build API ──┐
            ├──► Infra (Bicep) ──► Deploy App ──► Health Check ✅
Build Go  ──┘
```

## Repo Structure

```
├── src/MyApi/                  # C# .NET 8 Web API
├── infra/                      # Bicep IaC
│   ├── main.bicep
│   └── main.bicepparam
├── tools/azhealthcheck/        # Go health-check CLI tool
│   ├── main.go
│   └── go.mod
└── azure-pipelines.yml         # Azure DevOps pipeline
```

## Prerequisites

- Azure subscription with Contributor access
- Azure DevOps project with an Azure Resource Manager service connection
- Go 1.23+ (for building/running the tool locally)
- .NET 8 SDK (for building/running the API locally)

## Running Locally

### .NET API

```bash
cd src/MyApi
dotnet run
# GET http://localhost:5000/health
# → {"status":"healthy","timestamp":"2026-05-15T19:54:00.0000000Z"}
# GET http://localhost:5000/weatherforecast
# → JSON array of 5 forecast items
```

### Go health-check tool

```bash
cd tools/azhealthcheck
go mod tidy
go run . --app-name <your-app-service-name>

# All flags:
go run . \
  --app-name my-app \
  --resource-group my-rg \
  --health-path /health \
  --timeout 3m \
  --interval 8s
```

## Azure DevOps Pipeline Setup

1. **Import this repo** into your Azure DevOps project.

2. **Create a service connection**: Project Settings > Service connections > New > Azure Resource Manager.
   Name it `your-azure-service-connection-name` (or update the `serviceConnection` variable in `azure-pipelines.yml`).

3. **Create a pipeline** from `azure-pipelines.yml`.

4. **Set pipeline variables** (or edit the defaults in `azure-pipelines.yml`):
   | Variable | Example |
   |----------|---------|
   | `resourceGroupName` | `demo-rg` |
   | `location` | `eastus` |
   | `azureSubscriptionId` | `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` |
   | `appName` | `demo-myapi-dev` |
   | `planName` | `demo-asp-dev` |
   | `serviceConnection` | `your-azure-service-connection-name` |

5. **Create an Environment** named `azure-demo` in Azure DevOps (Pipelines > Environments).

6. **Run the pipeline** and watch it:
   - Build the .NET API and Go tool in parallel
   - Provision the App Service with Bicep
   - Deploy the API zip to the App Service
   - Poll `/health` every 10s until HTTP 200

## Extending the Demo

- **Multiple endpoints**: Run `azhealthcheck` multiple times with different `--health-path` values.
- **Slot swap validation**: Add a second check stage targeting a staging slot before swapping.
- **JSON output**: Pipe `azhealthcheck` output to `jq` or add a `--json` flag for structured pipeline logging.
- **Environment-specific params**: Create `main.prod.bicepparam` alongside `main.bicepparam` for prod overrides.
