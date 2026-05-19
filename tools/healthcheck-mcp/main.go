package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"healthcheck-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(mcp.NewTool("check_health",
		mcp.WithDescription("Perform a single HTTP GET to a health endpoint and return the status code and response body."),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description("Full URL to check, e.g. https://myapp.azurecontainerapps.io/health"),
		),
	), checkHealthHandler)

	s.AddTool(mcp.NewTool("poll_until_healthy",
		mcp.WithDescription("Poll a health endpoint repeatedly until it returns HTTP 200 or the timeout is reached. Useful after a deployment to wait for an app to become ready."),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description("Full URL to poll, e.g. https://myapp.azurecontainerapps.io/health"),
		),
		mcp.WithString("timeout",
			mcp.Description("How long to keep trying before giving up, e.g. 2m, 30s (default: 2m)"),
		),
		mcp.WithString("interval",
			mcp.Description("Time between attempts, e.g. 5s, 10s (default: 5s)"),
		),
	), pollUntilHealthyHandler)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func checkHealthHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url, err := req.RequireString("url")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("error: %v", err)), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	result := fmt.Sprintf("status: %d %s\nbody: %s",
		resp.StatusCode,
		http.StatusText(resp.StatusCode),
		strings.TrimSpace(string(body)),
	)
	return mcp.NewToolResultText(result), nil
}

func pollUntilHealthyHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url, err := req.RequireString("url")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	timeout := parseDuration(req.GetString("timeout", ""), 2*time.Minute)
	interval := parseDuration(req.GetString("interval", ""), 5*time.Second)

	client := &http.Client{Timeout: min(interval, 10 * time.Second)}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var attempts int

	check := func() bool {
		attempts++
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}

	if check() {
		return mcp.NewToolResultText(fmt.Sprintf("healthy after 1 attempt (immediate)")), nil
	}

	for {
		select {
		case t := <-ticker.C:
			if t.After(deadline) {
				return mcp.NewToolResultText(fmt.Sprintf(
					"timed out after %s (%d attempts) — %s is not returning HTTP 200",
					timeout, attempts, url,
				)), nil
			}
			if check() {
				return mcp.NewToolResultText(fmt.Sprintf(
					"healthy after %d attempts (~%s)",
					attempts, time.Since(deadline.Add(-timeout)).Round(time.Second),
				)), nil
			}
		case <-time.After(time.Until(deadline)):
			return mcp.NewToolResultText(fmt.Sprintf(
				"timed out after %s (%d attempts) — %s is not returning HTTP 200",
				timeout, attempts, url,
			)), nil
		}
	}
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}
