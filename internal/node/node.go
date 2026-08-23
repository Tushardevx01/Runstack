package node

import "time"

const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

type Capabilities struct {
	TotalMemoryBytes     uint64 `json:"totalMemoryBytes"`
	AvailableMemoryBytes uint64 `json:"availableMemoryBytes"`
	HasDocker            bool   `json:"hasDocker"`
	HasPodman            bool   `json:"hasPodman"`
}

type Node struct {
	ID            string       `json:"id"`
	Hostname      string       `json:"hostname"`
	IPAddress     string       `json:"ipAddress"`
	CPUCores      int          `json:"cpuCores"`
	OS            string       `json:"os"`
	Architecture  string       `json:"architecture"`
	Status        string       `json:"status"`
	LastHeartbeat time.Time    `json:"lastHeartbeat"`
	OfflineSince  *time.Time   `json:"offlineSince,omitempty"`
	Capabilities  Capabilities `json:"capabilities"`
}
