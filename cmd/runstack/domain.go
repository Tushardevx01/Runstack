package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/Tushardevx01/runstack/internal/ingress"
)

func runDomain(args []string) error {
	if len(args) < 1 {
		fmt.Println("Usage: runstack domain <command>")
		fmt.Println("Commands:")
		fmt.Println("  add <app-name|app-id> <domain>")
		fmt.Println("  tls <enable|status> <domain>")
		fmt.Println("  ls [app-name|app-id]")
		fmt.Println("  rm <domain-id>")
		os.Exit(1)
	}

	command := args[0]
	switch command {
	case "tls":
		if len(args) < 3 {
			return fmt.Errorf("usage: runstack domain tls <enable|status> <domain>")
		}
		action := args[1]
		domain := args[2]

		switch action {
		case "enable":
			resp, err := getClient().Post(fmt.Sprintf("/api/v1/domains/%s/tls", domain), "application/json", nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return fmt.Errorf("failed: %s", resp.Status)
			}
			fmt.Printf("TLS enabled for %s (ACME process started)\n", domain)
		case "status":
			resp, err := getClient().Get(fmt.Sprintf("/api/v1/domains/%s/tls", domain))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			var status map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&status)
			fmt.Printf("Domain: %s\nStatus: %s\n", status["domain"], status["status"])
			if errStr, ok := status["error"].(string); ok && errStr != "" {
				fmt.Printf("Error: %s\n", errStr)
			}
		default:
			return fmt.Errorf("unknown action: %s", action)
		}
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: runstack domain add <app-name|app-id> <domain>")
		}
		appID := args[1]
		domain := args[2]

		payload, _ := json.Marshal(map[string]interface{}{
			"name":           domain,
			"application_id": appID,
			"tls":            false,
		})

		resp, err := getClient().Post("/api/v1/domains", "application/json", bytes.NewBuffer(payload))
		if err != nil {
			return fmt.Errorf("failed to connect to Control Plane: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusConflict {
			return fmt.Errorf("domain %s is already registered", domain)
		}
		if resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("domain %s is claimed by another application", domain)
		}
		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("unexpected status: %s", resp.Status)
		}

		fmt.Printf("Domain %s added successfully to application %s\n", domain, appID)

	case "ls":
		url := "/api/v1/domains"
		if len(args) == 2 {
			url = fmt.Sprintf("/api/v1/domains?application_id=%s", args[1])
		}

		resp, err := getClient().Get(url)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer resp.Body.Close()

		var domains []ingress.Domain
		if err := json.NewDecoder(resp.Body).Decode(&domains); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tDOMAIN\tAPP_ID\tSTATUS\tTLS")
		for _, d := range domains {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", d.ID, d.Name, d.ApplicationID, d.Status, d.TLS)
		}
		w.Flush()

	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: runstack domain rm <domain-id>")
		}

		resp, err := getClient().Delete(fmt.Sprintf("/api/v1/domains/%s", args[1]))

		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("domain not found")
		}
		if resp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("unexpected status: %s", resp.Status)
		}
		fmt.Println("Domain deleted")
	}

	return nil
}
