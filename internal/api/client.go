package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Tushardevx01/runstack/internal/instance"
)

type Client struct {
	BaseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) ListInstances(nodeID, status string) ([]instance.Instance, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/v1/instances?node_id="+nodeID+"&status="+status, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var instances []instance.Instance
	if err := json.NewDecoder(resp.Body).Decode(&instances); err != nil {
		return nil, err
	}
	return instances, nil
}

func (c *Client) ClaimInstance(id string, nodeID string) (ClaimInstanceResponse, error) {
	body, _ := json.Marshal(ClaimInstanceRequest{NodeID: nodeID})
	req, err := http.NewRequest("POST", c.BaseURL+"/api/v1/instances/"+id+"/claim", bytes.NewBuffer(body))
	if err != nil {
		return ClaimInstanceResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ClaimInstanceResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ClaimInstanceResponse{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var claimResp ClaimInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&claimResp); err != nil {
		return ClaimInstanceResponse{}, err
	}
	return claimResp, nil
}

func (c *Client) ReportInstanceStatus(id string, nodeID, executionID string, status instance.InstanceStatus, health instance.InstanceHealth, containerID string, ports []instance.PortMapping) error {
	body, _ := json.Marshal(InstanceStatusRequest{
		NodeID:      nodeID,
		ExecutionID: executionID,
		Status:      status,
		Health:      health,
		ContainerID: containerID,
		Ports:       ports,
	})
	req, err := http.NewRequest("POST", c.BaseURL+"/api/v1/instances/"+id+"/status", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
