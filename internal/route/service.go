package route

import (
	"time"
)

type Protocol string

const (
	ProtocolHTTP Protocol = "http"
)

type Service struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Domain        string    `json:"domain"`
	PathPrefix    string    `json:"path_prefix"`
	TargetPort    int       `json:"target_port"`
	Protocol      Protocol  `json:"protocol"`
	CreatedAt     time.Time `json:"created_at"`
}
