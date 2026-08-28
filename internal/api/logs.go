package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
)

type LogsHandler struct {
	AppRegistry      *application.Registry
	InstanceRegistry *instance.Registry
	NodeRegistry     *node.Registry
}

func (h *LogsHandler) GetAppLogs(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	if appID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	app, err := h.AppRegistry.Get(appID)
	if err != nil && err == application.ErrNotFound {
		app, err = h.AppRegistry.GetByName(appID)
	}
	if err == nil {
		appID = app.ID
	}
	if err != nil {
		http.Error(w, "application not found", http.StatusNotFound)
		return
	}

	instanceID := r.URL.Query().Get("instance")

	// Find the instance(s)
	instances := h.InstanceRegistry.List()

	var targetInst *instance.Instance
	for _, inst := range instances {
		if inst.ApplicationID == appID {
			if instanceID != "" {
				if inst.ID == instanceID {
					targetInst = &inst
					break
				}
			} else {
				// Pick the first running instance for now if no specific one is requested
				if inst.Status == instance.StatusRunning {
					targetInst = &inst
					break
				}
			}
		}
	}

	if targetInst == nil {
		http.Error(w, "no suitable running instance found for application", http.StatusNotFound)
		return
	}

	// Validate ownership explicitly just in case
	if targetInst.ApplicationID != appID {
		http.Error(w, "instance does not belong to application", http.StatusForbidden)
		return
	}

	if targetInst.NodeID == "" || targetInst.ContainerID == "" {
		http.Error(w, "instance is not fully scheduled/running yet", http.StatusPreconditionFailed)
		return
	}

	// Find the Node IP
	n, err := h.NodeRegistry.Get(targetInst.NodeID)
	if err != nil {
		http.Error(w, "assigned node is not registered", http.StatusNotFound)
		return
	}

	if n.IPAddress == "" {
		http.Error(w, "assigned node does not have an IP address", http.StatusPreconditionFailed)
		return
	}

	ip := n.IPAddress
	if ip == "::1" || strings.HasPrefix(ip, "127.") {
		ip = "127.0.0.1"
	}

	// Dial Agent API
	agentURL := fmt.Sprintf("http://%s:8081/api/v1/logs?container=%s", ip, targetInst.ContainerID)

	client := http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), "GET", agentURL, nil)
	if err != nil {
		http.Error(w, "failed to create log request", http.StatusInternalServerError)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to contact agent: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("agent returned status %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}
