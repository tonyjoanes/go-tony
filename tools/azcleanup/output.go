package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

func printResults(apps []AppServiceStatus, format string) error {
	switch format {
	case "json":
		return printJSON(apps)
	default:
		return printTable(apps)
	}
}

func printTable(apps []AppServiceStatus) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tLOCATION\tSTATE\tREQUESTS(30d)\tIDLE")
	fmt.Fprintln(w, "----\t--------\t-----\t-------------\t----")
	for _, a := range apps {
		metric := fmt.Sprintf("%d", a.MetricTotal)
		if a.MetricTotal < 0 {
			metric = "N/A"
		}
		idle := "no"
		if a.Idle {
			idle = "YES"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			a.Name, a.Location, a.State, metric, idle)
	}
	return w.Flush()
}

func printJSON(apps []AppServiceStatus) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(apps)
}
