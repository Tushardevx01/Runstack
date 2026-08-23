package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
)

type ListJobsResponse struct {
	Jobs []job.Job `json:"jobs"`
}

func startJobPolling(ctx context.Context, nodeID string) {
	log.Println("Job polling started")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobs, err := fetchAssignedJobs(nodeID)
			if err != nil {
				// Don't spam logs if CP is temporarily down
				continue
			}

			if len(jobs) == 0 {
				continue
			}

			// Process first job (v1 executes one at a time)
			j := jobs[0]

			if err := claimJob(j.ID, nodeID); err != nil {
				log.Printf("Failed to claim job %s: %v", j.ID, err)
				continue
			}

			log.Printf("Successfully claimed job %s. Executing...", j.ID)
			res := executeJob(ctx, j)

			reportResultWithRetry(ctx, j.ID, nodeID, res)
		}
	}
}

func fetchAssignedJobs(nodeID string) ([]job.Job, error) {
	resp, err := http.Get(fmt.Sprintf("http://localhost:8080/api/v1/jobs?assignedNodeId=%s&status=%s", nodeID, job.StatusAssigned))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result ListJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Jobs, nil
}

func claimJob(jobID, nodeID string) error {
	payload, _ := json.Marshal(map[string]string{"nodeId": nodeID})

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:8080/api/v1/jobs/%s/claim", jobID),
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("claim rejected with status %d", resp.StatusCode)
	}

	return nil
}

// executeJob natively parses the execution string by splitting spaces.
// IMPORTANT V1 LIMITATION: This deliberately avoids shell injection by NOT using /bin/sh.
// As a result, quoted arguments (e.g. echo "hello world") are NOT supported in v1 and
// will be split indiscriminately into ["echo", "\"hello", "world\""].
func executeJob(ctx context.Context, j job.Job) job.JobResult {
	parts := strings.Fields(j.Command)
	if len(parts) == 0 {
		return job.JobResult{ExitCode: -1, Error: "empty command"}
	}

	// Note: context cancellation terminates the command process managed by os/exec.
	// It does not guarantee that all descendant processes spawned by this process will be terminated.
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()

	exitCode := 0
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return job.JobResult{
		ExitCode: exitCode,
		Stdout:   outBuf.String(),
		Stderr:   errBuf.String(),
		Error:    errMsg,
	}
}

func reportResultWithRetry(ctx context.Context, jobID, nodeID string, res job.JobResult) {
	payload, _ := json.Marshal(map[string]interface{}{
		"nodeId": nodeID,
		"result": res,
	})

	for i := 0; i < 5; i++ {
		resp, err := http.Post(
			fmt.Sprintf("http://localhost:8080/api/v1/jobs/%s/result", jobID),
			"application/json",
			bytes.NewBuffer(payload),
		)

		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("Successfully reported result for job %s (exit code %d)", jobID, res.ExitCode)
				return
			}
			log.Printf("Failed to report result (status %d). Retrying...", resp.StatusCode)
		} else {
			log.Printf("Failed to connect while reporting result: %v. Retrying...", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	log.Printf("Gave up reporting result for job %s after multiple attempts", jobID)
}
