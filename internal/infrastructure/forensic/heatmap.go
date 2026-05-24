// Package forensic — Gaze Heatmap Generator
//
// Generates an SVG-based gaze heatmap from normalized gaze telemetry points.
// The heatmap uses a 2D Gaussian kernel density estimation to produce
// a continuous density surface rendered as overlapping radial gradients.
//
// Output: standalone SVG string (embeddable in PDF or HTML).
package forensic

import (
	"fmt"
	"math"
	"strings"
)

// HeatmapConfig configures the SVG heatmap generator.
type HeatmapConfig struct {
	Width      int     // SVG width in pixels (default: 640)
	Height     int     // SVG height in pixels (default: 480)
	CellSize   int     // Grid cell size for density binning (default: 16)
	Radius     int     // Gaussian blob radius (default: 30)
	MaxOpacity float64 // Maximum opacity for the hottest cell (default: 0.7)
}

var defaultHeatmapConfig = HeatmapConfig{
	Width:      640,
	Height:     480,
	CellSize:   16,
	Radius:     30,
	MaxOpacity: 0.7,
}

// GenerateGazeHeatmapSVG produces an SVG string from gaze data points.
// Points must be normalized to [0,1] for both X and Y axes.
func GenerateGazeHeatmapSVG(points []GazePoint, cfg *HeatmapConfig) string {
	if cfg == nil {
		c := defaultHeatmapConfig
		cfg = &c
	}
	if cfg.Width == 0 {
		cfg.Width = 640
	}
	if cfg.Height == 0 {
		cfg.Height = 480
	}
	if cfg.CellSize == 0 {
		cfg.CellSize = 16
	}
	if cfg.Radius == 0 {
		cfg.Radius = 30
	}
	if cfg.MaxOpacity == 0 {
		cfg.MaxOpacity = 0.7
	}

	cols := cfg.Width / cfg.CellSize
	rows := cfg.Height / cfg.CellSize

	// Build density grid
	grid := make([][]float64, rows)
	for r := range grid {
		grid[r] = make([]float64, cols)
	}

	// Accumulate density from each gaze point
	kernelCells := cfg.Radius / cfg.CellSize
	if kernelCells < 1 {
		kernelCells = 1
	}

	for _, p := range points {
		cx := int(p.X * float64(cols-1))
		cy := int(p.Y * float64(rows-1))

		for dy := -kernelCells; dy <= kernelCells; dy++ {
			for dx := -kernelCells; dx <= kernelCells; dx++ {
				gx := cx + dx
				gy := cy + dy
				if gx < 0 || gx >= cols || gy < 0 || gy >= rows {
					continue
				}
				dist := math.Sqrt(float64(dx*dx + dy*dy))
				if dist > float64(kernelCells) {
					continue
				}
				weight := math.Exp(-(dist * dist) / (2.0 * float64(kernelCells) * 0.5))
				grid[gy][gx] += weight
			}
		}
	}

	// Find max density for normalization
	maxDensity := 0.0
	for _, row := range grid {
		for _, val := range row {
			if val > maxDensity {
				maxDensity = val
			}
		}
	}
	if maxDensity == 0 {
		maxDensity = 1
	}

	// Build SVG
	var svg strings.Builder
	fmt.Fprintf(&svg,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		cfg.Width, cfg.Height, cfg.Width, cfg.Height,
	)
	svg.WriteString("\n")

	// Background
	fmt.Fprintf(&svg,
		`<rect width="%d" height="%d" fill="#1a1a2e"/>`,
		cfg.Width, cfg.Height,
	)
	svg.WriteString("\n")

	// Radial gradient definition
	svg.WriteString(`<defs>`)
	svg.WriteString(
		`<radialGradient id="hg"><stop offset="0%%" stop-color="#ff0000" stop-opacity="1"/>` +
			`<stop offset="30%%" stop-color="#ffaa00" stop-opacity="0.8"/>` +
			`<stop offset="60%%" stop-color="#ffff00" stop-opacity="0.4"/>` +
			`<stop offset="100%%" stop-color="#00ff00" stop-opacity="0"/></radialGradient>`,
	)
	svg.WriteString(`</defs>`)
	svg.WriteString("\n")

	// Render density blobs
	// Only render cells above 5% density to keep SVG compact
	threshold := maxDensity * 0.05
	for gy, row := range grid {
		for gx, val := range row {
			if val < threshold {
				continue
			}
			norm := val / maxDensity
			opacity := norm * cfg.MaxOpacity
			cx := gx*cfg.CellSize + cfg.CellSize/2
			cy := gy*cfg.CellSize + cfg.CellSize/2
			r := int(float64(cfg.Radius) * (0.5 + norm*0.5))

			fmt.Fprintf(&svg,
				`<circle cx="%d" cy="%d" r="%d" fill="url(#hg)" opacity="%.2f"/>`,
				cx, cy, r, opacity,
			)
			svg.WriteString("\n")
		}
	}

	// Grid labels: "Центр", "Лево", "Право"
	labelStyle := `font-family="Arial,sans-serif" font-size="11" fill="#ffffff" opacity="0.4"`
	fmt.Fprintf(&svg,
		`<text x="%d" y="%d" text-anchor="middle" %s>Центр</text>`,
		cfg.Width/2, cfg.Height-8, labelStyle,
	)
	fmt.Fprintf(&svg,
		`<text x="8" y="%d" %s>Лево</text>`,
		cfg.Height/2, labelStyle,
	)
	fmt.Fprintf(&svg,
		`<text x="%d" y="%d" text-anchor="end" %s>Право</text>`,
		cfg.Width-8, cfg.Height/2, labelStyle,
	)

	// Border
	fmt.Fprintf(&svg,
		`<rect width="%d" height="%d" fill="none" stroke="#333" stroke-width="1"/>`,
		cfg.Width, cfg.Height,
	)

	svg.WriteString("\n</svg>")
	return svg.String()
}
