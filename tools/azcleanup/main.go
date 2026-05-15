package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var (
		subscriptionID string
		resourceGroup  string
		outputFormat   string
		metricName     string
		days           int
		dryRun         bool
		doDelete       bool
	)

	root := &cobra.Command{
		Use:   "azcleanup",
		Short: "Find and optionally delete idle Azure App Services",
		Long: `azcleanup scans a resource group and lists App Services that have had zero
metric activity in the last N days (default: 30). Use --delete to remove them.

Authentication uses DefaultAzureCredential — it will try environment variables,
workload identity, managed identity, and az CLI login in order.

Examples:
  azcleanup -g my-rg -s 00000000-0000-0000-0000-000000000000
  azcleanup -g my-rg --output json
  azcleanup -g my-rg --dry-run --delete
  azcleanup -g my-rg --metric Connections --days 7`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if subscriptionID == "" {
				subscriptionID = os.Getenv("AZURE_SUBSCRIPTION_ID")
			}
			if subscriptionID == "" {
				return fmt.Errorf("--subscription or AZURE_SUBSCRIPTION_ID env var is required")
			}
			if resourceGroup == "" {
				return fmt.Errorf("--resource-group is required")
			}
			if outputFormat != "table" && outputFormat != "json" {
				return fmt.Errorf("--output must be 'table' or 'json'")
			}
			if days < 1 {
				return fmt.Errorf("--days must be at least 1")
			}

			ctx := context.Background()

			c, err := newClients(subscriptionID)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Scanning resource group %q (metric: %s, last %d days)...\n",
				resourceGroup, metricName, days)

			apps, err := listAppServices(ctx, c, subscriptionID, resourceGroup, metricName, days)
			if err != nil {
				return err
			}

			if len(apps) == 0 {
				fmt.Fprintln(os.Stderr, "No App Services found in resource group.")
				return nil
			}

			if err := printResults(apps, outputFormat); err != nil {
				return err
			}

			idle := filterIdle(apps)
			if len(idle) == 0 {
				fmt.Fprintln(os.Stderr, "\nNo idle App Services found.")
				return nil
			}

			if !doDelete {
				fmt.Fprintf(os.Stderr, "\n%d idle App Service(s) found. Pass --delete to remove them.\n", len(idle))
				return nil
			}

			if dryRun {
				fmt.Fprintf(os.Stderr, "\n[dry-run] Would delete %d idle App Service(s):\n", len(idle))
				for _, a := range idle {
					fmt.Fprintf(os.Stderr, "  - %s (%s)\n", a.Name, a.Location)
				}
				return nil
			}

			fmt.Fprintf(os.Stderr, "\nDeleting %d idle App Service(s)...\n", len(idle))
			var deleteErr bool
			for _, a := range idle {
				fmt.Fprintf(os.Stderr, "  Deleting %s... ", a.Name)
				if err := deleteAppService(ctx, c, resourceGroup, a.Name); err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
					deleteErr = true
				} else {
					fmt.Fprintln(os.Stderr, "done.")
				}
			}
			if deleteErr {
				return fmt.Errorf("one or more deletions failed")
			}
			return nil
		},
	}

	root.Flags().StringVarP(&subscriptionID, "subscription", "s", "", "Azure subscription ID (or AZURE_SUBSCRIPTION_ID env)")
	root.Flags().StringVarP(&resourceGroup, "resource-group", "g", "", "Azure resource group name (required)")
	root.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format: table or json")
	root.Flags().StringVar(&metricName, "metric", "Requests", "Azure Monitor metric to check (e.g. Requests, Connections)")
	root.Flags().IntVar(&days, "days", 30, "Lookback window in days")
	root.Flags().BoolVar(&dryRun, "dry-run", false, "Preview deletions without making changes (requires --delete)")
	root.Flags().BoolVar(&doDelete, "delete", false, "Delete idle App Services after listing")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func filterIdle(apps []AppServiceStatus) []AppServiceStatus {
	var idle []AppServiceStatus
	for _, a := range apps {
		if a.Idle {
			idle = append(idle, a)
		}
	}
	return idle
}
