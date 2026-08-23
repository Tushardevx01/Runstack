package node

import "time"

const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

type Node struct {
	ID            string    `json:"id"`
	Hostname      string    `json:"hostname"`
	CPUCores      int       `json:"cpuCores"`
	OS            string    `json:"os"`
	Architecture  string    `json:"architecture"`
	Status        string    `json:"status"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
}
