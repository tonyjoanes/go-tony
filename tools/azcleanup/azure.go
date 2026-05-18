package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor"
)

type AppServiceStatus struct {
	Name          string `json:"name"`
	Location      string `json:"location"`
	State         string `json:"state"`
	MetricTotal   int64  `json:"metric_total_30d"` // -1 = unavailable
	MetricName    string `json:"metric_name"`
	Idle          bool   `json:"idle"`
}

type clients struct {
	web     *armappservice.WebAppsClient
	metrics *armmonitor.MetricsClient
}

func newClients(subscriptionID string) (*clients, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("creating credential: %w", err)
	}
	web, err := armappservice.NewWebAppsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating web apps client: %w", err)
	}
	metrics, err := armmonitor.NewMetricsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating metrics client: %w", err)
	}
	return &clients{web: web, metrics: metrics}, nil
}

func listAppServices(ctx context.Context, c *clients, subscriptionID, resourceGroup, metricName string, days int) ([]AppServiceStatus, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -days)
	timespan := fmt.Sprintf("%s/%s", start.Format(time.RFC3339), now.Format(time.RFC3339))

	var results []AppServiceStatus

	pager := c.web.NewListByResourceGroupPager(resourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing app services: %w", err)
		}
		for _, site := range page.Value {
			if site.Name == nil {
				continue
			}
			resourceID := fmt.Sprintf(
				"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Web/sites/%s",
				subscriptionID, resourceGroup, *site.Name,
			)

			total, err := getMetricTotal(ctx, c.metrics, resourceID, timespan, metricName)
			if err != nil {
				fmt.Printf("  warning: metrics unavailable for %s: %v\n", *site.Name, err)
				total = -1
			}

			location, state := "", ""
			if site.Location != nil {
				location = *site.Location
			}
			if site.Properties != nil && site.Properties.State != nil {
				state = *site.Properties.State
			}

			results = append(results, AppServiceStatus{
				Name:        *site.Name,
				Location:    location,
				State:       state,
				MetricTotal: total,
				MetricName:  metricName,
				Idle:        total == 0,
			})
		}
	}
	return results, nil
}

func getMetricTotal(ctx context.Context, client *armmonitor.MetricsClient, resourceID, timespan, metricName string) (int64, error) {
	resp, err := client.List(ctx, resourceID, &armmonitor.MetricsClientListOptions{
		Timespan:    to.Ptr(timespan),
		Metricnames: to.Ptr(metricName),
		Aggregation: to.Ptr("Total"),
		Interval:    to.Ptr("P1D"),
	})
	if err != nil {
		return 0, err
	}

	var total float64
	for _, m := range resp.Value {
		if m.Timeseries == nil {
			continue
		}
		for _, ts := range m.Timeseries {
			for _, dp := range ts.Data {
				if dp.Total != nil {
					total += *dp.Total
				}
			}
		}
	}
	return int64(total), nil
}

func deleteAppService(ctx context.Context, c *clients, resourceGroup, appName string) error {
	_, err := c.web.Delete(ctx, resourceGroup, appName, &armappservice.WebAppsClientDeleteOptions{
		DeleteMetrics:        to.Ptr(true),
		DeleteEmptyServerFarm: to.Ptr(false),
	})
	return err
}
