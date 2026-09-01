package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Tushardevx01/runstack/internal/api"
)

func runApps(args []string) error {
	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
	}

	resp, err := getClient().Get("/api/v1/apps")
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var result struct {
		Applications []api.AppSummary `json:"applications"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if len(result.Applications) == 0 {
		fmt.Println("No applications found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tREADY\tDESIRED\tROLLOUT\tDOMAINS\tTLS")
	for _, app := range result.Applications {
		domStr := "-"
		if len(app.Domains) > 0 {
			domStr = strings.Join(app.Domains, ",")
			if len(domStr) > 30 {
				domStr = domStr[:27] + "..."
			}
		}

		tlsStr := "-"
		if app.TLSActive {
			tlsStr = "Active"
		} else if len(app.Domains) > 0 {
			tlsStr = "Pending/None"
		}

		rolloutStr := string(app.RolloutStatus)
		if rolloutStr == "" {
			rolloutStr = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t%s\t%s\n",
			app.Name, app.Status, app.ReadyReplicas, app.DesiredReplicas, rolloutStr, domStr, tlsStr)
	}
	w.Flush()
	return nil
}

func runAppStatus(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: runstack app status <name> [--json]")
	}
	name := args[0]
	jsonOutput := false
	for _, a := range args[1:] {
		if a == "--json" {
			jsonOutput = true
		}
	}

	resp, err := getClient().Get(fmt.Sprintf("/api/v1/apps/%s/status", name))
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("application '%s' not found", name)
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var detail api.AppStatusDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(detail)
	}

	fmt.Printf("Application: %s (%s)\n", detail.Application.Name, detail.Application.Status)
	if detail.ActiveDeployment != nil {
		fmt.Printf("Active Deployment: %s (Hash: %s)\n", detail.ActiveDeployment.ID, detail.ActiveDeployment.Hash)
		fmt.Println()
		fmt.Printf("Rollout State: %s\n", detail.ActiveDeployment.RolloutStatus)
		fmt.Printf("  Replicas: %d Desired | %d Ready | %d Unavailable\n",
			detail.ActiveDeployment.DesiredReplicas,
			detail.ActiveDeployment.ReadyReplicas,
			detail.ActiveDeployment.UnavailableReplicas)

		reason := detail.ActiveDeployment.BlockedReason
		if reason == "" {
			reason = "-"
		}
		fmt.Printf("  Reason: %s\n", reason)
	} else {
		fmt.Println("Active Deployment: None")
	}

	fmt.Println("\nInstances:")
	if len(detail.Instances) == 0 {
		fmt.Println("  (no instances)")
	} else {
		for _, inst := range detail.Instances {
			fmt.Printf("  %s  %s  %s  %s  (Resets: -)\n",
				inst.ID, inst.NodeID, inst.Status, inst.Health)
		}
	}

	fmt.Println("\nNetworking:")
	if len(detail.Domains) == 0 {
		fmt.Println("  Domains: (none)")
	} else {
		fmt.Println("  Domains:")
		for _, d := range detail.Domains {
			tlsStr := "None"
			if d.TLS {
				tlsStr = string(d.Status)
			}
			fmt.Printf("    - %s (TLS: %s, Path: %s)\n", d.Name, tlsStr, d.Path)
		}
	}

	return nil
}
