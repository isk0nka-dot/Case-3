package monitoring

import (
	"encoding/json"
	"io"
)

// ---------------------------------------------------------------------------
// Docker Stats JSON Parsing
// ---------------------------------------------------------------------------
//
// Docker container stats are returned as a JSON stream. We parse a single
// snapshot from the one-shot API call.
// See: https://docs.docker.com/engine/api/v1.41/#operation/ContainerStats

// DockerStatsJSON matches the Docker stats JSON structure.
type DockerStatsJSON struct {
	CPUStats    CPUStatsJSON    `json:"cpu_stats"`
	PreCPUStats CPUStatsJSON    `json:"precpu_stats"`
	MemoryStats MemoryStatsJSON `json:"memory_stats"`
	Networks    map[string]NetworkStatsJSON `json:"networks"`
}

// CPUStatsJSON holds CPU timing data.
type CPUStatsJSON struct {
	CPUUsage    CPUUsageJSON `json:"cpu_usage"`
	SystemUsage uint64       `json:"system_cpu_usage"`
	OnlineCPUs  int          `json:"online_cpus"`
}

// CPUUsageJSON holds total CPU usage.
type CPUUsageJSON struct {
	TotalUsage uint64 `json:"total_usage"`
}

// MemoryStatsJSON holds memory metrics.
type MemoryStatsJSON struct {
	Usage uint64            `json:"usage"`
	Limit uint64            `json:"limit"`
	Stats map[string]uint64 `json:"stats"`
}

// NetworkStatsJSON holds per-interface network stats.
type NetworkStatsJSON struct {
	RxBytes   uint64 `json:"rx_bytes"`
	TxBytes   uint64 `json:"tx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	TxPackets uint64 `json:"tx_packets"`
	RxErrors  uint64 `json:"rx_errors"`
	TxErrors  uint64 `json:"tx_errors"`
}

// parseContainerStats decodes a Docker stats JSON response.
func parseContainerStats(body io.Reader) (*DockerStatsJSON, error) {
	var stats DockerStatsJSON
	if err := json.NewDecoder(body).Decode(&stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// calculateCPUPercent computes the CPU usage percentage from Docker stats.
// This uses the same algorithm as the Docker CLI.
func calculateCPUPercent(stats *DockerStatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)

	if systemDelta <= 0 || cpuDelta < 0 {
		return 0.0
	}

	numCPUs := stats.CPUStats.OnlineCPUs
	if numCPUs == 0 {
		numCPUs = 1
	}

	return (cpuDelta / systemDelta) * float64(numCPUs) * 100.0
}
