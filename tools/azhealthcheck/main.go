package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func main() {
	var (
		appName       string
		resourceGroup string
		healthPath    string
		timeout       time.Duration
		interval      time.Duration
	)

	root := &cobra.Command{
		Use:   "azhealthcheck",
		Short: "Poll an Azure App Service health endpoint until it returns HTTP 200",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appName == "" {
				return fmt.Errorf("--app-name is required")
			}
			url := fmt.Sprintf("https://%s.azurewebsites.net%s", appName, healthPath)
			return poll(url, timeout, interval)
		},
	}

	root.Flags().StringVar(&appName, "app-name", "", "Azure App Service name (required)")
	root.Flags().StringVar(&resourceGroup, "resource-group", "", "Azure resource group (informational)")
	root.Flags().StringVar(&healthPath, "health-path", "/health", "Path to poll on the App Service")
	root.Flags().DurationVar(&timeout, "timeout", 3*time.Minute, "Total time before giving up")
	root.Flags().DurationVar(&interval, "interval", 8*time.Second, "Time between poll attempts")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func poll(url string, timeout, interval time.Duration) error {
	requestTimeout := interval
	if requestTimeout > 10*time.Second {
		requestTimeout = 10 * time.Second
	}
	client := &http.Client{Timeout: requestTimeout}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Printf("Polling %s (timeout: %s, interval: %s)\n", url, timeout, interval)

	if check(client, url) {
		return nil
	}

	for {
		select {
		case t := <-ticker.C:
			if t.After(deadline) {
				return fmt.Errorf("timed out after %s waiting for %s to return HTTP 200", timeout, url)
			}
			if check(client, url) {
				return nil
			}
		case <-time.After(time.Until(deadline)):
			return fmt.Errorf("timed out after %s waiting for %s to return HTTP 200", timeout, url)
		}
	}
}

func check(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("  [%s] error: %v\n", time.Now().Format(time.RFC3339), err)
		return false
	}
	defer resp.Body.Close()
	ok := resp.StatusCode == http.StatusOK
	fmt.Printf("  [%s] status: %d", time.Now().Format(time.RFC3339), resp.StatusCode)
	if ok {
		fmt.Println(" - healthy!")
	} else {
		fmt.Println()
	}
	return ok
}
