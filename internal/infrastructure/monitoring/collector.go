// =============================================================================
// Argus AI — Infrastructure Metrics Collector
// =============================================================================
//
// Collects real-time metrics from Docker containers and the host system.
// Uses the Docker SDK to query per-container CPU/RAM/Network stats,
// and gopsutil for host-level disk metrics.
//
// Designed to run inside a Docker container with /var/run/docker.sock mounted.
// =============================================================================

package monitoring

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerClient "github.com/docker/docker/client"
	"github.com/shirou/gopsutil/v4/disk"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Types — match the frontend's expected JSON structure
// ---------------------------------------------------------------------------

// ContainerStats holds per-container resource metrics.
type ContainerStats struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Region      string  `json:"region"`
	CPULoad     float64 `json:"cpuLoad"`
	MemoryUsage float64 `json:"memoryUsage"`
	DiskUsage   float64 `json:"diskUsage"`
	Status      string  `json:"status"` // "healthy" | "warning" | "critical"
	LatencyMs   float64 `json:"latencyMs"`
	Uptime      float64 `json:"uptime"`
	Connections int     `json:"connections"`
	MemUsedMB   float64 `json:"memUsedMB"`
	MemLimitMB  float64 `json:"memLimitMB"`
}

// NetworkMetric holds a single network / performance metric row.
type NetworkMetric struct {
	Label  string  `json:"label"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	Status string  `json:"status"` // "healthy" | "warning" | "critical"
	Trend  string  `json:"trend"`  // "up" | "down" | "stable"
}

// LatencyPoint holds a time-series latency measurement.
type LatencyPoint struct {
	Time string  `json:"time"`
	Avg  float64 `json:"avg"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
}

// InfrastructureStats is the top-level response for the endpoint.
type InfrastructureStats struct {
	ServerNodes    []ContainerStats `json:"serverNodes"`
	NetworkMetrics []NetworkMetric  `json:"networkMetrics"`
	LatencyHistory []LatencyPoint   `json:"latencyHistory"`
	TotalCapacity  int              `json:"totalCapacity"`
	CurrentLoad    int              `json:"currentLoad"`
	LoadPercent    float64          `json:"loadPercent"`
	AvgLatencyMs   float64          `json:"avgLatencyMs"`
	P99LatencyMs   float64          `json:"p99LatencyMs"`
	CollectedAt    string           `json:"collectedAt"`
}

// ---------------------------------------------------------------------------
// Collector
// ---------------------------------------------------------------------------

// Collector gathers real-time infrastructure metrics.
type Collector struct {
	logger     *zap.Logger
	docker     *dockerClient.Client
	hasDocker  bool
	containers []string // container name prefixes to monitor

	// Latency tracking (populated from gRPC interceptor stats if available).
	mu             sync.RWMutex
	latencyHistory []LatencyPoint
	prevCPU        map[string]prevCPUStats
}

type prevCPUStats struct {
	cpuUsage    uint64
	systemUsage uint64
}

// containerNameMap maps Docker container names to human-readable names.
var containerNameMap = map[string]string{
	"argus-event-collector": "Event Collector",
	"argus-kafka":           "Kafka Broker",
	"argus-clickhouse":      "ClickHouse",
	"argus-postgres":        "PostgreSQL",
}

// containerRegionMap maps container names to region labels.
var containerRegionMap = map[string]string{
	"argus-event-collector": "Docker Host",
	"argus-kafka":           "Docker Host",
	"argus-clickhouse":      "Docker Host",
	"argus-postgres":        "Docker Host",
}

// NewCollector creates a new infrastructure metrics collector.
func NewCollector(logger *zap.Logger) *Collector {
	c := &Collector{
		logger:     logger,
		containers: []string{"argus-"},
		prevCPU:    make(map[string]prevCPUStats),
	}

	// Try to connect to Docker.
	cli, err := dockerClient.NewClientWithOpts(
		dockerClient.FromEnv,
		dockerClient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		logger.Warn("Docker SDK unavailable — will use synthetic metrics", zap.Error(err))
		c.hasDocker = false
	} else {
		// Verify connectivity with a ping.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err = cli.Ping(ctx)
		if err != nil {
			logger.Warn("Docker daemon unreachable — will use synthetic metrics", zap.Error(err))
			c.hasDocker = false
		} else {
			c.docker = cli
			c.hasDocker = true
			logger.Info("Docker SDK connected — real container metrics enabled")
		}
	}

	return c
}

// Collect gathers a full snapshot of infrastructure stats.
func (c *Collector) Collect(ctx context.Context) (*InfrastructureStats, error) {
	stats := &InfrastructureStats{
		CollectedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if c.hasDocker {
		nodes, err := c.collectDockerStats(ctx)
		if err != nil {
			c.logger.Error("Failed to collect Docker stats", zap.Error(err))
			// Fall back to empty nodes rather than failing the entire endpoint.
			nodes = []ContainerStats{}
		}
		stats.ServerNodes = nodes
	} else {
		stats.ServerNodes = []ContainerStats{}
	}

	// Collect disk usage for ClickHouse volume (or root partition).
	c.enrichDiskUsage(stats)

	// Compute aggregate metrics.
	c.computeAggregates(stats)

	// Build network metrics.
	stats.NetworkMetrics = c.buildNetworkMetrics(stats)

	// Add latency history.
	c.mu.RLock()
	stats.LatencyHistory = c.latencyHistory
	c.mu.RUnlock()
	if stats.LatencyHistory == nil {
		stats.LatencyHistory = []LatencyPoint{}
	}

	return stats, nil
}

// collectDockerStats queries Docker for per-container resource usage.
func (c *Collector) collectDockerStats(ctx context.Context) ([]ContainerStats, error) {
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("docker container list: %w", err)
	}

	var result []ContainerStats

	for _, ctr := range containers {
		// Filter to our argus- containers.
		name := strings.TrimPrefix(ctr.Names[0], "/")
		isOurs := false
		for _, prefix := range c.containers {
			if strings.HasPrefix(name, prefix) {
				isOurs = true
				break
			}
		}
		if !isOurs {
			continue
		}

		// Get container stats (one-shot, not streaming).
		statsResp, err := c.docker.ContainerStatsOneShot(ctx, ctr.ID)
		if err != nil {
			c.logger.Warn("Failed to get stats for container",
				zap.String("container", name), zap.Error(err))
			continue
		}

		parsed, err := parseContainerStats(statsResp.Body)
		statsResp.Body.Close()
		if err != nil {
			c.logger.Warn("Failed to parse stats for container",
				zap.String("container", name), zap.Error(err))
			continue
		}

		// Calculate CPU percentage.
		cpuPercent := calculateCPUPercent(parsed)

		// Calculate memory percentage.
		memPercent := 0.0
		memUsedMB := 0.0
		memLimitMB := 0.0
		if parsed.MemoryStats.Limit > 0 {
			memUsage := parsed.MemoryStats.Usage - parsed.MemoryStats.Stats["cache"]
			memPercent = float64(memUsage) / float64(parsed.MemoryStats.Limit) * 100
			memUsedMB = float64(memUsage) / (1024 * 1024)
			memLimitMB = float64(parsed.MemoryStats.Limit) / (1024 * 1024)
		}

		// Determine health status.
		status := "healthy"
		if cpuPercent > 90 || memPercent > 90 {
			status = "critical"
		} else if cpuPercent > 75 || memPercent > 75 {
			status = "warning"
		}

		// Determine uptime from container state.
		uptime := 99.99 // Default high uptime.
		if ctr.State != "running" {
			uptime = 0
			status = "critical"
		}

		// Friendly name.
		friendlyName := name
		if mapped, ok := containerNameMap[name]; ok {
			friendlyName = mapped
		}

		region := "Docker Host"
		if mapped, ok := containerRegionMap[name]; ok {
			region = mapped
		}

		node := ContainerStats{
			ID:          ctr.ID[:12],
			Name:        friendlyName,
			Region:      region,
			CPULoad:     round2(cpuPercent),
			MemoryUsage: round2(memPercent),
			DiskUsage:   0, // Filled later.
			Status:      status,
			LatencyMs:   0, // Filled from gRPC stats if available.
			Uptime:      uptime,
			Connections:  0,
			MemUsedMB:   round2(memUsedMB),
			MemLimitMB:  round2(memLimitMB),
		}

		result = append(result, node)
	}

	return result, nil
}

// enrichDiskUsage adds disk usage info (primarily for ClickHouse data volume).
func (c *Collector) enrichDiskUsage(stats *InfrastructureStats) {
	// Get disk usage of / (root) — inside the container, this is the overlay FS.
	usage, err := disk.Usage("/")
	if err != nil {
		c.logger.Warn("Failed to get disk usage", zap.Error(err))
		return
	}

	rootDiskPercent := round2(usage.UsedPercent)

	// Apply to all nodes (since they share the same Docker host).
	for i := range stats.ServerNodes {
		stats.ServerNodes[i].DiskUsage = rootDiskPercent
	}

	// Also try ClickHouse-specific path if mounted.
	chUsage, err := disk.Usage("/var/lib/clickhouse")
	if err == nil {
		// Apply specifically to ClickHouse node.
		for i := range stats.ServerNodes {
			if stats.ServerNodes[i].Name == "ClickHouse" {
				stats.ServerNodes[i].DiskUsage = round2(chUsage.UsedPercent)
			}
		}
	}
}

// computeAggregates calculates summary KPIs from per-node data.
func (c *Collector) computeAggregates(stats *InfrastructureStats) {
	stats.TotalCapacity = 10000 // Design capacity.

	if len(stats.ServerNodes) == 0 {
		stats.CurrentLoad = 0
		stats.LoadPercent = 0
		stats.AvgLatencyMs = 0
		stats.P99LatencyMs = 0
		return
	}

	// Aggregate CPU as the "load percent" (weighted average).
	totalCPU := 0.0
	for _, n := range stats.ServerNodes {
		totalCPU += n.CPULoad
	}
	avgCPU := totalCPU / float64(len(stats.ServerNodes))
	stats.LoadPercent = round2(avgCPU)

	// Current load as a proportion of capacity based on CPU.
	stats.CurrentLoad = int(avgCPU / 100 * float64(stats.TotalCapacity))
}

// buildNetworkMetrics constructs the network metrics array.
func (c *Collector) buildNetworkMetrics(stats *InfrastructureStats) []NetworkMetric {
	healthyNodes := 0
	warningNodes := 0
	criticalNodes := 0
	for _, n := range stats.ServerNodes {
		switch n.Status {
		case "healthy":
			healthyNodes++
		case "warning":
			warningNodes++
		case "critical":
			criticalNodes++
		}
	}

	totalNodes := len(stats.ServerNodes)

	// Compute average memory usage.
	avgMem := 0.0
	if totalNodes > 0 {
		totalMem := 0.0
		for _, n := range stats.ServerNodes {
			totalMem += n.MemoryUsage
		}
		avgMem = totalMem / float64(totalNodes)
	}

	metrics := []NetworkMetric{
		{
			Label:  "Docker контейнеры",
			Value:  float64(totalNodes),
			Unit:   "",
			Status: statusFromCount(criticalNodes, warningNodes),
			Trend:  "stable",
		},
		{
			Label:  "Средняя CPU нагрузка",
			Value:  round2(stats.LoadPercent),
			Unit:   "%",
			Status: statusFromPercent(stats.LoadPercent),
			Trend:  "stable",
		},
		{
			Label:  "Средняя RAM нагрузка",
			Value:  round2(avgMem),
			Unit:   "%",
			Status: statusFromPercent(avgMem),
			Trend:  "stable",
		},
		{
			Label:  "Здоровые ноды",
			Value:  float64(healthyNodes),
			Unit:   fmt.Sprintf("из %d", totalNodes),
			Status: statusFromCount(criticalNodes, warningNodes),
			Trend:  "stable",
		},
	}

	// Add per-container memory details.
	for _, n := range stats.ServerNodes {
		metrics = append(metrics, NetworkMetric{
			Label:  fmt.Sprintf("RAM: %s", n.Name),
			Value:  round2(n.MemUsedMB),
			Unit:   "MB",
			Status: statusFromPercent(n.MemoryUsage),
			Trend:  "stable",
		})
	}

	return metrics
}

// RecordLatencyPoint records a latency measurement (called periodically).
func (c *Collector) RecordLatencyPoint(avg, p95, p99 float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	point := LatencyPoint{
		Time: time.Now().Format("15:04"),
		Avg:  round2(avg),
		P95:  round2(p95),
		P99:  round2(p99),
	}

	c.latencyHistory = append(c.latencyHistory, point)

	// Keep only the last 24 points (24 hours at hourly intervals, or 4 hours at 10min).
	if len(c.latencyHistory) > 24 {
		c.latencyHistory = c.latencyHistory[len(c.latencyHistory)-24:]
	}
}

// Close cleans up resources.
func (c *Collector) Close() {
	if c.docker != nil {
		c.docker.Close()
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func statusFromPercent(pct float64) string {
	if pct > 90 {
		return "critical"
	}
	if pct > 75 {
		return "warning"
	}
	return "healthy"
}

func statusFromCount(critical, warning int) string {
	if critical > 0 {
		return "critical"
	}
	if warning > 0 {
		return "warning"
	}
	return "healthy"
}
