package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/sysinfo"
)

type StatusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type ListNodesResponse struct {
	Nodes []node.Node `json:"nodes"`
}

func getStatus() error {
	resp, err := http.Get("http://localhost:8080/api/v1/status")
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var status StatusResponse

	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Println("RunStack Control Plane")
	fmt.Println("----------------------")
	fmt.Printf("Status:  %s\n", status.Status)
	fmt.Printf("Version: %s\n", status.Version)

	return nil
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func getNodes() error {
	resp, err := http.Get("http://localhost:8080/api/v1/nodes")
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var result ListNodesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Nodes) == 0 {
		fmt.Println("No nodes registered.")
		return nil
	}

	fmt.Println("RunStack Nodes")
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tHOSTNAME\tCPU\tMEM\tOS\tARCH\tCONTAINER\tSTATUS")
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")
	for _, n := range result.Nodes {
		container := ""
		if n.Capabilities.HasDocker {
			container += "docker "
		}
		if n.Capabilities.HasPodman {
			container += "podman"
		}
		if container == "" {
			container = "none"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n", n.ID, n.Hostname, n.CPUCores, formatBytes(n.Capabilities.TotalMemoryBytes), n.OS, n.Architecture, container, n.Status)
	}
	w.Flush()

	return nil
}

func getNode(id string) error {
	resp, err := http.Get("http://localhost:8080/api/v1/nodes/" + id)
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("node not found: %s", id)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var n node.Node
	if err := json.NewDecoder(resp.Body).Decode(&n); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Println("Node")
	fmt.Println("-------------------------")
	fmt.Printf("ID:             %s\n", n.ID)
	fmt.Printf("Hostname:       %s\n", n.Hostname)
	fmt.Printf("CPU Cores:      %d\n", n.CPUCores)
	fmt.Printf("OS:             %s\n", n.OS)
	fmt.Printf("Architecture:   %s\n", n.Architecture)
	fmt.Printf("Status:         %s\n", n.Status)
	fmt.Printf("Last Heartbeat: %s\n", n.LastHeartbeat.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("Capabilities:")
	fmt.Printf("  Total Memory:     %s\n", formatBytes(n.Capabilities.TotalMemoryBytes))
	fmt.Printf("  Available Memory: %s\n", formatBytes(n.Capabilities.AvailableMemoryBytes))
	fmt.Printf("  Docker:           %v\n", n.Capabilities.HasDocker)
	fmt.Printf("  Podman:           %v\n", n.Capabilities.HasPodman)

	return nil
}

type ListJobsResponse struct {
	Jobs []job.Job `json:"jobs"`
}

func getJobs(args []string) error {
	statusFilter := ""
	nodeFilter := ""

	for i := 0; i < len(args); i++ {
		if args[i] == "--status" && i+1 < len(args) {
			statusFilter = args[i+1]
			i++
		} else if args[i] == "--node" && i+1 < len(args) {
			nodeFilter = args[i+1]
			i++
		}
	}

	urlStr := "http://localhost:8080/api/v1/jobs"
	query := ""
	if statusFilter != "" {
		query += "status=" + statusFilter
	}
	if nodeFilter != "" {
		if query != "" {
			query += "&"
		}
		query += "assignedNodeId=" + nodeFilter
	}
	if query != "" {
		urlStr += "?" + query
	}

	resp, err := http.Get(urlStr)
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var result ListJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Jobs) == 0 {
		fmt.Println("No jobs found.")
		return nil
	}

	fmt.Println("RunStack Jobs")
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tNODE")
	fmt.Fprintln(w, "------------------------------------------")
	for _, j := range result.Jobs {
		nodeStr := j.AssignedNodeID
		if nodeStr == "" {
			nodeStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", j.ID, j.Name, j.Status, nodeStr)
	}
	w.Flush()

	return nil
}

func getJob(id string) error {
	resp, err := http.Get("http://localhost:8080/api/v1/jobs/" + id)
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("job not found: %s", id)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var j job.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Println("Job")
	fmt.Println("-------------------------")
	fmt.Printf("ID:            %s\n", j.ID)
	fmt.Printf("Name:          %s\n", j.Name)
	fmt.Printf("Command:       %s\n", j.Command)
	fmt.Printf("Status:        %s\n", j.Status)
	fmt.Printf("Created:       %s\n", j.CreatedAt.Format(time.RFC3339))

	started := "-"
	if j.StartedAt != nil {
		started = j.StartedAt.Format(time.RFC3339)
	}
	fmt.Printf("Started:       %s\n", started)

	completed := "-"
	if j.CompletedAt != nil {
		completed = j.CompletedAt.Format(time.RFC3339)
	}
	fmt.Printf("Completed:     %s\n", completed)

	nodeStr := j.AssignedNodeID
	if nodeStr == "" {
		nodeStr = "-"
	}
	fmt.Printf("Assigned Node: %s\n", nodeStr)

	resStr := "-"
	if j.Result != nil {
		resStr = fmt.Sprintf("ExitCode: %d", j.Result.ExitCode)
		if j.Result.Error != "" {
			resStr += fmt.Sprintf(", Error: %s", j.Result.Error)
		}
	}
	fmt.Printf("Result:        %s\n", resStr)

	if j.Result != nil {
		fmt.Println("-------------------------")
		fmt.Printf("Stdout:\n%s\n", j.Result.Stdout)
		fmt.Printf("Stderr:\n%s\n", j.Result.Stderr)
	}

	return nil
}

type JobHistoryResponse struct {
	Events []job.JobEvent `json:"events"`
}

func getJobHistory(id string) error {
	resp, err := http.Get(fmt.Sprintf("http://localhost:8080/api/v1/jobs/%s/events", id))
	if err != nil {
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("job not found: %s", id)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var hist JobHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&hist); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Println("RunStack Job History")
	fmt.Println("--------------------")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TIME\tEVENT\tFROM\tTO\tNODE")
	for _, e := range hist.Events {
		nodeStr := e.NodeID
		if nodeStr == "" {
			nodeStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Timestamp.Format(time.RFC3339), e.Type, e.From, e.To, nodeStr)
	}
	w.Flush()

	return nil
}

func runDoctor() {
	fmt.Println("RunStack Doctor")
	fmt.Println("────────────────────────────")

	healthy := true

	// Control Plane
	resp, err := http.Get("http://localhost:8080/health")
	cpReachable := err == nil && resp.StatusCode == http.StatusOK
	if resp != nil {
		resp.Body.Close()
	}

	if cpReachable {
		fmt.Println("Control Plane       ✓ reachable")
	} else {
		fmt.Println("Control Plane       ✗ unreachable")
		healthy = false
	}

	// API
	statusResp, err := http.Get("http://localhost:8080/api/v1/status")
	apiHealthy := err == nil && statusResp.StatusCode == http.StatusOK
	if statusResp != nil {
		statusResp.Body.Close()
	}

	if apiHealthy {
		fmt.Println("API                 ✓ healthy")
	} else {
		fmt.Println("API                 ✗ unreachable")
		healthy = false
	}

	// Node Registry
	nodesResp, err := http.Get("http://localhost:8080/api/v1/nodes")
	nodeReg := err == nil && nodesResp.StatusCode == http.StatusOK
	var nodes []node.Node
	if nodeReg {
		var listResp struct {
			Nodes []node.Node `json:"nodes"`
		}
		if err := json.NewDecoder(nodesResp.Body).Decode(&listResp); err == nil {
			nodes = listResp.Nodes
		}
		nodesResp.Body.Close()
	}

	if nodeReg {
		fmt.Println("Node Registry       ✓ accessible")
	} else {
		fmt.Println("Node Registry       ✗ inaccessible")
		healthy = false
	}

	// Job Registry
	jobsResp, err := http.Get("http://localhost:8080/api/v1/jobs")
	jobReg := err == nil && jobsResp.StatusCode == http.StatusOK
	if jobsResp != nil {
		jobsResp.Body.Close()
	}

	if jobReg {
		fmt.Println("Job Registry        ✓ accessible")
	} else {
		fmt.Println("Job Registry        ✗ inaccessible")
		healthy = false
	}

	fmt.Println("\nNodes")
	if len(nodes) == 0 {
		fmt.Println("  (no nodes registered)")
	}
	for _, n := range nodes {
		statusIcon := "✓"
		if n.Status != node.StatusOnline {
			statusIcon = "✗"
		}
		fmt.Printf("  %-17s %s %s\n", n.ID, statusIcon, n.Status)
	}

	fmt.Println("\nLocal Agent Capabilities")
	mem := sysinfo.GetMemoryInfo()
	fmt.Printf("  CPU               %d cores\n", runtime.NumCPU())
	fmt.Printf("  Memory            %.1f GiB\n", float64(mem.TotalBytes)/(1024*1024*1024))

	if sysinfo.HasDocker() {
		fmt.Println("  Docker            ✓")
	} else {
		fmt.Println("  Docker            ✗")
	}
	if sysinfo.HasPodman() {
		fmt.Println("  Podman            ✓")
	} else {
		fmt.Println("  Podman            ✗")
	}

	fmt.Println("\nScheduler           ✓ configured")

	fmt.Println("\nOverall")
	if healthy {
		fmt.Println("                    ✓ healthy")
	} else {
		fmt.Println("                    ✗ degraded")
	}
}

func printError(err error) {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "failed to connect") {
		fmt.Println("Error: unable to connect to RunStack Control Plane at localhost:8080")
		fmt.Println("Hint: start the Control Plane with `make control-plane`")
	} else if strings.Contains(msg, "status 404") {
		fmt.Println("Error: resource not found")
	} else {
		fmt.Printf("Error: %v\n", err)
	}
}

func runCLI(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: runstack <command>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  cp        Start Control Plane")
		fmt.Println("  agent     Start Agent")
		fmt.Println("  status    Show control plane status")
		fmt.Println("  version   Show CLI version")
		fmt.Println("  doctor    Diagnose system health")
		fmt.Println("  nodes     List all registered nodes")
		fmt.Println("  node <id> Show details of a specific node")
		fmt.Println("  jobs      List all jobs (flags: --status, --node)")
		fmt.Println("  job <id>  Show details of a specific job")
		os.Exit(1)
	}

	command := args[0]
	switch command {
	case "status":
		if err := getStatus(); err != nil {
			printError(err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("RunStack v0.1.0")
	case "doctor":
		runDoctor()
	case "nodes":
		if err := getNodes(); err != nil {
			printError(err)
			os.Exit(1)
		}
	case "node":
		if len(args) < 2 {
			fmt.Println("Usage: runstack node <id>")
			os.Exit(1)
		}
		if err := getNode(args[1]); err != nil {
			printError(err)
			os.Exit(1)
		}
	case "jobs":
		if err := getJobs(args[1:]); err != nil {
			printError(err)
			os.Exit(1)
		}
	case "job":
		if len(args) < 2 {
			fmt.Println("Usage: runstack job <id> [--history]")
			os.Exit(1)
		}
		if len(args) == 3 && args[2] == "--history" {
			if err := getJobHistory(args[1]); err != nil {
				printError(err)
				os.Exit(1)
			}
		} else {
			if err := getJob(args[1]); err != nil {
				printError(err)
				os.Exit(1)
			}
		}
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}
