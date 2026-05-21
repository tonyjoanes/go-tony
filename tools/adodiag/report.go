package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func buildReport(build *adoBuild, timeline *adoTimeline, logs map[int]string, pr *adoPullRequest) string {
	var sb strings.Builder
	w := &sb

	fmt.Fprintf(w, "# ADO Pipeline Diagnostic Report\n\n")
	fmt.Fprintf(w, "_Generated: %s_\n\n", time.Now().UTC().Format(time.RFC3339))

	// ── Run summary ──────────────────────────────────────────────────────────
	fmt.Fprintf(w, "## Run Summary\n\n")
	fmt.Fprintf(w, "| | |\n|---|---|\n")
	fmt.Fprintf(w, "| **Run ID** | %d (`%s`) |\n", build.ID, build.BuildNumber)
	fmt.Fprintf(w, "| **Pipeline** | %s (def #%d) |\n", build.Definition.Name, build.Definition.ID)
	fmt.Fprintf(w, "| **Status** | %s |\n", build.Status)
	fmt.Fprintf(w, "| **Result** | %s |\n", statusMark(build.Result))
	fmt.Fprintf(w, "| **Branch** | `%s` |\n", build.SourceBranch)
	fmt.Fprintf(w, "| **Commit** | `%s` |\n", shortSHA(build.SourceVersion))
	fmt.Fprintf(w, "| **Reason** | %s |\n", build.Reason)
	fmt.Fprintf(w, "| **Triggered by** | %s |\n", build.RequestedBy.DisplayName)
	fmt.Fprintf(w, "| **Started** | %s |\n", fmtTime(build.StartTime))
	fmt.Fprintf(w, "| **Finished** | %s |\n", fmtTime(build.FinishTime))
	fmt.Fprintf(w, "| **Duration** | %s |\n", elapsed(build.StartTime, build.FinishTime))
	if href := build.Links.Web.Href; href != "" {
		fmt.Fprintf(w, "| **ADO URL** | [open](%s) |\n", href)
	}
	fmt.Fprintf(w, "\n")

	// Build lookup map for parent traversal
	byID := make(map[string]*adoRecord, len(timeline.Records))
	for i := range timeline.Records {
		r := &timeline.Records[i]
		byID[r.ID] = r
	}

	// ── Stage summary ────────────────────────────────────────────────────────
	stages := recordsOfType(timeline.Records, "Stage")
	if len(stages) > 0 {
		fmt.Fprintf(w, "## Stage Summary\n\n")
		fmt.Fprintf(w, "| Stage | Result | Duration | Errors | Warnings |\n")
		fmt.Fprintf(w, "|-------|--------|----------|--------|----------|\n")
		for _, s := range stages {
			fmt.Fprintf(w, "| %s | %s | %s | %d | %d |\n",
				s.Name, statusMark(s.Result), elapsed(s.StartTime, s.FinishTime),
				s.ErrorCount, s.WarningCount,
			)
		}
		fmt.Fprintf(w, "\n")
	}

	// ── Failed steps with logs ───────────────────────────────────────────────
	var failed []adoRecord
	for _, r := range timeline.Records {
		if r.Type == "Task" && r.Result == "failed" {
			failed = append(failed, r)
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(w, "## Failed Steps\n\n")
		for _, task := range failed {
			crumb := breadcrumb(task, byID)
			fmt.Fprintf(w, "### %s\n\n", crumb)
			fmt.Fprintf(w, "**Result:** %s  \n", statusMark(task.Result))
			fmt.Fprintf(w, "**Duration:** %s  \n", elapsed(task.StartTime, task.FinishTime))
			if task.ErrorCount > 0 {
				fmt.Fprintf(w, "**Errors:** %d  \n", task.ErrorCount)
			}
			if len(task.Issues) > 0 {
				fmt.Fprintf(w, "\n**Issues:**\n\n")
				for _, issue := range task.Issues {
					fmt.Fprintf(w, "- `%s`: %s\n", issue.Type, issue.Message)
				}
			}
			if task.Log != nil {
				if content, ok := logs[task.Log.ID]; ok {
					fmt.Fprintf(w, "\n**Log (last 100 lines):**\n\n```\n%s\n```\n\n", lastN(content, 100))
				}
			}
		}
	}

	// ── Full step timeline ───────────────────────────────────────────────────
	tasks := recordsOfType(timeline.Records, "Task")
	if len(tasks) > 0 {
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].StartTime < tasks[j].StartTime })
		fmt.Fprintf(w, "## Full Step Timeline\n\n")
		fmt.Fprintf(w, "| Step | Result | Duration |\n")
		fmt.Fprintf(w, "|------|--------|----------|\n")
		for _, t := range tasks {
			fmt.Fprintf(w, "| %s | %s | %s |\n",
				breadcrumb(t, byID), statusMark(t.Result), elapsed(t.StartTime, t.FinishTime),
			)
		}
		fmt.Fprintf(w, "\n")
	}

	// ── Pull request ─────────────────────────────────────────────────────────
	if pr != nil && pr.ID > 0 {
		fmt.Fprintf(w, "## Pull Request\n\n")
		fmt.Fprintf(w, "**PR #%d:** %s  \n", pr.ID, pr.Title)
		fmt.Fprintf(w, "**Author:** %s  \n", pr.CreatedBy.DisplayName)
		fmt.Fprintf(w, "**Status:** %s  \n", pr.Status)
		fmt.Fprintf(w, "**Branch:** `%s` → `%s`  \n", pr.SourceBranch, pr.TargetBranch)
		fmt.Fprintf(w, "**Created:** %s  \n", fmtTime(pr.CreationDate))
		if pr.Description != "" {
			desc := pr.Description
			if len(desc) > 600 {
				desc = desc[:600] + "...(truncated)"
			}
			fmt.Fprintf(w, "\n> %s\n\n", strings.ReplaceAll(strings.TrimSpace(desc), "\n", "\n> "))
		}
		fmt.Fprintf(w, "\n")
	}

	return sb.String()
}

// ── helpers ───────────────────────────────────────────────────────────────────

func statusMark(result string) string {
	switch result {
	case "succeeded":
		return "[ok] succeeded"
	case "failed":
		return "[FAIL] failed"
	case "canceled":
		return "[canceled]"
	case "skipped":
		return "[skipped]"
	case "partiallySucceeded":
		return "[partial] partiallySucceeded"
	case "":
		return "-"
	default:
		return result
	}
}

func shortSHA(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

var timeFmts = []string{
	"2006-01-02T15:04:05.9999999Z07:00",
	time.RFC3339Nano,
	time.RFC3339,
}

func parseTime(s string) time.Time {
	for _, f := range timeFmts {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func fmtTime(s string) string {
	t := parseTime(s)
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func elapsed(start, finish string) string {
	s, f := parseTime(start), parseTime(finish)
	if s.IsZero() || f.IsZero() || f.Before(s) {
		return "-"
	}
	d := f.Sub(s).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func lastN(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) <= n {
		return strings.TrimSpace(text)
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func recordsOfType(records []adoRecord, t string) []adoRecord {
	var out []adoRecord
	for _, r := range records {
		if r.Type == t {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// breadcrumb walks parent links to build "Stage > Job > Task", skipping Phase nodes.
func breadcrumb(r adoRecord, byID map[string]*adoRecord) string {
	parts := []string{r.Name}
	visited := map[string]bool{r.ID: true}
	cur := r.ParentID
	for range 5 {
		if cur == "" {
			break
		}
		p, ok := byID[cur]
		if !ok || visited[cur] {
			break
		}
		visited[cur] = true
		if p.Type != "Phase" {
			parts = append([]string{p.Name}, parts...)
		}
		cur = p.ParentID
	}
	return strings.Join(parts, " > ")
}
