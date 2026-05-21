package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ── ADO types ─────────────────────────────────────────────────────────────────

type adoBuild struct {
	ID            int    `json:"id"`
	BuildNumber   string `json:"buildNumber"`
	Status        string `json:"status"`
	Result        string `json:"result"`
	QueueTime     string `json:"queueTime"`
	StartTime     string `json:"startTime"`
	FinishTime    string `json:"finishTime"`
	SourceBranch  string `json:"sourceBranch"`
	SourceVersion string `json:"sourceVersion"`
	Reason        string `json:"reason"`
	RequestedBy   struct {
		DisplayName string `json:"displayName"`
		UniqueName  string `json:"uniqueName"`
	} `json:"requestedBy"`
	TriggerInfo map[string]string `json:"triggerInfo"`
	Definition  struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"definition"`
	Repository struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"repository"`
	Links struct {
		Web struct {
			Href string `json:"href"`
		} `json:"web"`
	} `json:"_links"`
}

type adoTimeline struct {
	Records []adoRecord `json:"records"`
}

type adoRecord struct {
	ID           string  `json:"id"`
	ParentID     string  `json:"parentId"`
	Type         string  `json:"type"`
	Name         string  `json:"name"`
	State        string  `json:"state"`
	Result       string  `json:"result"`
	StartTime    string  `json:"startTime"`
	FinishTime   string  `json:"finishTime"`
	Order        int     `json:"order"`
	ErrorCount   int     `json:"errorCount"`
	WarningCount int     `json:"warningCount"`
	Log          *adoLog `json:"log"`
	Issues       []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"issues"`
}

type adoLog struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

type adoPullRequest struct {
	ID           int    `json:"pullRequestId"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	SourceBranch string `json:"sourceRefName"`
	TargetBranch string `json:"targetRefName"`
	CreatedBy    struct {
		DisplayName string `json:"displayName"`
	} `json:"createdBy"`
	CreationDate string `json:"creationDate"`
}

// ── ADO HTTP client ───────────────────────────────────────────────────────────

type client struct {
	baseURL string
	pat     string
	http    *http.Client
}

func newClient(org, project, pat string) *client {
	return &client{
		baseURL: fmt.Sprintf("https://dev.azure.com/%s/%s/_apis", org, project),
		pat:     pat,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *client) auth() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+c.pat))
}

func (c *client) getJSON(path string, v any) error {
	req, err := http.NewRequest("GET", c.baseURL+"/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.auth())

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *client) getLogText(buildID, logID int) (string, error) {
	path := fmt.Sprintf("%s/build/builds/%d/logs/%d?api-version=7.1", c.baseURL, buildID, logID)
	req, _ := http.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", c.auth())
	req.Header.Set("Accept", "text/plain")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func (c *client) getBuild(id int) (*adoBuild, error) {
	var b adoBuild
	return &b, c.getJSON(fmt.Sprintf("build/builds/%d?api-version=7.1", id), &b)
}

func (c *client) getTimeline(buildID int) (*adoTimeline, error) {
	var t adoTimeline
	return &t, c.getJSON(fmt.Sprintf("build/builds/%d/timeline?api-version=7.1", buildID), &t)
}

func (c *client) getPR(repoID string, prID int) (*adoPullRequest, error) {
	var pr adoPullRequest
	err := c.getJSON(fmt.Sprintf("git/repositories/%s/pullRequests/%d?api-version=7.1", repoID, prID), &pr)
	return &pr, err
}

// ── CLI ───────────────────────────────────────────────────────────────────────

func main() {
	var (
		org        string
		project    string
		pat        string
		runID      int
		logFilter  string
		outputFile string
		includePR  bool
	)

	root := &cobra.Command{
		Use:   "adodiag",
		Short: "Collect Azure DevOps pipeline run diagnostics into a markdown report",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pat == "" {
				pat = os.Getenv("AZURE_DEVOPS_PAT")
			}
			if org == "" {
				org = os.Getenv("AZURE_DEVOPS_ORG")
			}
			if project == "" {
				project = os.Getenv("AZURE_DEVOPS_PROJECT")
			}
			switch {
			case pat == "":
				return fmt.Errorf("--pat or AZURE_DEVOPS_PAT required")
			case org == "":
				return fmt.Errorf("--org or AZURE_DEVOPS_ORG required")
			case project == "":
				return fmt.Errorf("--project or AZURE_DEVOPS_PROJECT required")
			case runID == 0:
				return fmt.Errorf("--run-id required")
			}

			c := newClient(org, project, pat)

			logf("Fetching build %d...", runID)
			build, err := c.getBuild(runID)
			if err != nil {
				return fmt.Errorf("get build: %w", err)
			}

			logf("Fetching timeline...")
			timeline, err := c.getTimeline(runID)
			if err != nil {
				return fmt.Errorf("get timeline: %w", err)
			}

			// Fetch logs for selected tasks
			logs := make(map[int]string)
			for _, r := range timeline.Records {
				if r.Type != "Task" || r.Log == nil {
					continue
				}
				if logFilter == "none" {
					continue
				}
				if logFilter == "failed" && r.Result != "failed" {
					continue
				}
				logf("Fetching log: %s...", r.Name)
				content, err := c.getLogText(runID, r.Log.ID)
				if err == nil {
					logs[r.Log.ID] = content
				}
			}

			// PR lookup
			var pr *adoPullRequest
			if includePR {
				prNum := extractPRNumber(build)
				if prNum > 0 && build.Repository.ID != "" {
					logf("Fetching PR #%d...", prNum)
					pr, _ = c.getPR(build.Repository.ID, prNum)
				}
			}

			report := buildReport(build, timeline, logs, pr)

			if outputFile != "" {
				if err := os.WriteFile(outputFile, []byte(report), 0644); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				logf("Report written to %s", outputFile)
			} else {
				fmt.Print(report)
			}
			return nil
		},
	}

	root.Flags().StringVar(&org, "org", "", "ADO organization (or AZURE_DEVOPS_ORG)")
	root.Flags().StringVar(&project, "project", "", "ADO project (or AZURE_DEVOPS_PROJECT)")
	root.Flags().StringVar(&pat, "pat", "", "Personal access token (or AZURE_DEVOPS_PAT)")
	root.Flags().IntVar(&runID, "run-id", 0, "Pipeline run / build ID")
	root.Flags().StringVar(&logFilter, "logs", "failed", "Logs to include: failed | all | none")
	root.Flags().StringVar(&outputFile, "output", "", "Write report to file (default: stdout)")
	root.Flags().BoolVar(&includePR, "pr", true, "Include linked pull request")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func extractPRNumber(build *adoBuild) int {
	// PR-triggered builds set triggerInfo["pr.number"]
	if v, ok := build.TriggerInfo["pr.number"]; ok {
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	// Branch refs/pull/N/merge (GitHub-backed repos)
	parts := strings.Split(build.SourceBranch, "/")
	for i, p := range parts {
		if p == "pull" && i+1 < len(parts) {
			var n int
			fmt.Sscanf(parts[i+1], "%d", &n)
			return n
		}
	}
	return 0
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
