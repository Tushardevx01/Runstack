package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/Tushardevx01/runstack/internal/application"
)

func runSecret(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing secret command: set, ls, rm")
	}

	command := args[0]
	switch command {
	case "set":
		return runSecretSet(args[1:])
	case "ls":
		return runSecretList(args[1:])
	case "rm":
		return runSecretDelete(args[1:])
	default:
		return fmt.Errorf("unknown secret command: %s", command)
	}
}

func runSecretSet(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: runstack secret set <app_id> <name> <value>")
	}
	appID := args[0]
	name := args[1]
	value := args[2]

	payload, _ := json.Marshal(map[string]string{
		"application_id": appID,
		"name":           name,
		"value":          value,
	})

	resp, err := http.Post("http://localhost:8080/api/v1/secrets", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set secret: %s", string(bytes.TrimSpace(b)))
	}

	fmt.Printf("Secret %s set for application %s\n", name, appID)
	return nil
}

func runSecretList(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: runstack secret ls <app_id>")
	}
	appID := args[0]

	resp, err := http.Get(fmt.Sprintf("http://localhost:8080/api/v1/secrets?application_id=%s", appID))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to list secrets: %s", string(bytes.TrimSpace(b)))
	}

	var secrets []application.Secret
	if err := json.NewDecoder(resp.Body).Decode(&secrets); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tUPDATED_AT")
	for _, sec := range secrets {
		fmt.Fprintf(w, "%s\t%s\t%s\n", sec.ID, sec.Name, sec.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	w.Flush()
	return nil
}

func runSecretDelete(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: runstack secret rm <secret_id>")
	}
	id := args[0]

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("http://localhost:8080/api/v1/secrets/%s", id), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete secret: %s", string(bytes.TrimSpace(b)))
	}

	fmt.Printf("Secret %s deleted\n", id)
	return nil
}
