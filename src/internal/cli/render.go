package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/lfaoliveira/harpy/src/internal/model"
	"github.com/lfaoliveira/harpy/src/internal/platform"
)

func (a *Application) writeJSON(command string, data any) error {
	envelope := struct {
		Schema  int    `json:"schema"`
		Command string `json:"command"`
		Version string `json:"version"`
		Data    any    `json:"data"`
	}{
		Schema:  schemaVersion,
		Command: command,
		Version: Version,
		Data:    data,
	}
	encoder := json.NewEncoder(a.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(envelope)
}

// planPayload normalizes the serialized log path without touching the physical
// path used to read and write logs.
func planPayload(plan *model.Plan) *model.Plan {
	if plan == nil || plan.LogDir == "" {
		return plan
	}
	out := *plan
	out.LogDir = platform.ToSlash(out.LogDir)
	return &out
}

func printPlan(w io.Writer, plan *model.Plan) {
	header := "harpy plan — " + platform.HostOS()
	if plan.Machine != "" {
		header += " · machine: " + plan.Machine
	}
	fmt.Fprintln(w, header)
	if len(plan.Profiles) > 0 {
		fmt.Fprintf(w, "profiles: %s\n", strings.Join(plan.Profiles, ", "))
	}
	fmt.Fprintln(w)

	if len(plan.Items) == 0 {
		fmt.Fprintln(w, "No apps selected.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tMANAGER\tID\tAPP\tDETAIL")
	for _, item := range plan.Items {
		managerName := item.Manager.String()
		if managerName == "" {
			managerName = "-"
		}
		id := item.ID.String()
		if id == "" {
			id = "-"
		}
		detail := item.Note
		if item.Error != "" {
			detail = item.Error
		}
		if detail == "" {
			detail = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.Status, managerName, id, item.App, detail)
	}
	tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintln(w, summaryLine(plan.Summary))
}

func summaryLine(s model.Summary) string {
	parts := []string{
		fmt.Sprintf("%d total", s.Total),
		fmt.Sprintf("%d to install", s.ToInstall),
		fmt.Sprintf("%d installed", s.Installed),
	}
	if s.Blocked > 0 {
		parts = append(parts, fmt.Sprintf("%d blocked", s.Blocked))
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", s.Failed))
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
	}
	return "Summary: " + strings.Join(parts, " · ")
}

func printManagers(w io.Writer, infos []model.ManagerInfo, elevated bool) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tOS\tHOST\tAVAILABLE\tELEVATION")
	for _, info := range infos {
		host := "no"
		if info.Host {
			host = "yes"
		}
		available := "no"
		if info.Available {
			available = "yes"
		}
		elevation := "not required"
		if info.ElevationRequired {
			elevation = "required"
			if info.Host && elevated {
				elevation = "required (active)"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", info.Name, info.OS, host, available, elevation)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nhost: %s · elevated: %t\n", platform.HostOS(), elevated)
}
