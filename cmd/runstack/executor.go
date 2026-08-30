package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
)

func startJobPolling(ctx context.Context, nodeID, cpURL, agentToken string) {
	slog.Info("Job polling started")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobs, err := fetchAssignedJobs(nodeID, cpURL, agentToken)
			if err != nil {
				// Don't spam logs if CP is temporarily down
				continue
			}

			if len(jobs) == 0 {
				continue
			}

			// Process first job (v1 executes one at a time)
			j := jobs[0]

			execID, err := claimJob(j.ID, nodeID, cpURL, agentToken)
			if err != nil {
				slog.Error("Failed to claim job", "job_id", j.ID, "error", err)
				continue
			}

			slog.Info("job execution started", "job_id", j.ID, "execution_id", execID, "command", j.Command)
			res := executeJob(ctx, j)

			reportResultWithRetry(ctx, j.ID, nodeID, execID, cpURL, agentToken, res)
		}
	}
}

func fetchAssignedJobs(nodeID, cpURL, agentToken string) ([]job.Job, error) {
	resp, err := agentDoReq("GET", fmt.Sprintf("%s/api/v1/jobs?assignedNodeId=%s&status=assigned", cpURL, nodeID), nil, agentToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var parsed struct {
		Jobs []job.Job `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	return parsed.Jobs, nil
}

func claimJob(jobID, nodeID, cpURL, agentToken string) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"nodeId": nodeID,
	})

	resp, err := agentDoReq("POST", fmt.Sprintf("%s/api/v1/jobs/%s/claim", cpURL, jobID), payload, agentToken)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status claiming job: %d", resp.StatusCode)
	}

	var parsed struct {
		ExecutionID string `json:"executionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}

	return parsed.ExecutionID, nil
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

func reportResultWithRetry(ctx context.Context, jobID, nodeID, execID, cpURL, agentToken string, res job.JobResult) {
	payload, _ := json.Marshal(map[string]interface{}{
		"nodeId":      nodeID,
		"executionId": execID,
		"result":      res,
	})

	for i := 0; i < 5; i++ {
		resp, err := agentDoReq("POST", fmt.Sprintf("%s/api/v1/jobs/%s/result", cpURL, jobID), payload, agentToken)

		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				slog.Info("job completed", "job_id", jobID, "execution_id", execID, "exit_code", res.ExitCode)
				return
			}
			if resp.StatusCode == http.StatusConflict {
				slog.Warn("Stale execution result rejected by Control Plane", "job_id", jobID, "execution_id", execID)
				return
			}
			slog.Warn("Failed to report result. Retrying...", "status", resp.StatusCode)
		} else {
			slog.Warn("Failed to connect while reporting result. Retrying...", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	slog.Error("Gave up reporting result for job after multiple attempts", "job_id", jobID)
}
