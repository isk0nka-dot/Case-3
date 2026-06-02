// Package reporter sends agent telemetry to the Argus backend.
package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client sends proctoring events to the Argus backend on behalf of the agent.
// It authenticates using the student's argusSessionToken.
type Client struct {
	serverURL    string
	sessionToken string
	httpClient   *http.Client
}

// NewClient creates a new reporter client.
func NewClient(serverURL, sessionToken string) *Client {
	return &Client{
		serverURL:    serverURL,
		sessionToken: sessionToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AgentEvent represents a single event pushed from the desktop agent.
type AgentEvent struct {
	EventType  string  `json:"eventType"`   // e.g. "FORBIDDEN_PROCESS_DETECTED"
	Severity   string  `json:"severity"`    // "critical" | "warning" | "info"
	Source     string  `json:"source"`      // always "KERNEL_AGENT"
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Details    string  `json:"details,omitempty"`
	Timestamp  string  `json:"timestamp"`
}

// AgentHeartbeat is the payload for the heartbeat endpoint.
type AgentHeartbeat struct {
	SessionToken  string      `json:"sessionToken"`
	AgentVersion  string      `json:"agentVersion"`
	OS            string      `json:"os"`
	Hostname      string      `json:"hostname"`
	MachineID     string      `json:"machineId"`
	MonitorCount  int         `json:"monitorCount"`
	IsVM          bool        `json:"isVm"`
	VMProduct     string      `json:"vmProduct,omitempty"`
	ProcessCount  int         `json:"processCount"`
	ForbiddenProcs []string   `json:"forbiddenProcs,omitempty"`
	Timestamp     string      `json:"timestamp"`
}

type ingestBatchRequest struct {
	Events []AgentEvent `json:"events"`
}

// SendEvents pushes a batch of agent events to the backend.
func (c *Client) SendEvents(ctx context.Context, events []AgentEvent) error {
	if len(events) == 0 {
		return nil
	}

	body, err := json.Marshal(ingestBatchRequest{Events: events})
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.serverURL+"/api/v1/external/events",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.sessionToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned %d for events", resp.StatusCode)
	}
	return nil
}

// SendHeartbeat sends a heartbeat with system status to the backend.
func (c *Client) SendHeartbeat(ctx context.Context, hb AgentHeartbeat) error {
	body, err := json.Marshal(hb)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.serverURL+"/api/v1/agent/heartbeat",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.sessionToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send heartbeat: %w", err)
	}
	defer resp.Body.Close()
	// 404 is OK — backend may not have the endpoint yet; don't fail heartbeat
	return nil
}

// CheckServer verifies the Argus backend is reachable.
func (c *Client) CheckServer(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.serverURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server health check returned %d", resp.StatusCode)
	}
	return nil
}
