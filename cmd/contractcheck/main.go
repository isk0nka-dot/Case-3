// +build ignore

// Command contractcheck verifies that TypeScript types in argus-frontend stay
// in sync with the canonical proto definitions. It parses the proto file for
// enum values and cross-references them against the TypeScript types file.
//
// This is the Go-native equivalent of proto-sync.sh, designed to run in CI:
//
//	go run cmd/contractcheck/main.go
//
// Exit codes:
//
//	0 — All contracts aligned
//	1 — Desync detected (enum values missing or mismatched)
//	2 — File not found or parse error
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type enumEntry struct {
	Name  string
	Value int
}

func main() {
	protoFile := "../argus-backend/api/proto/v1/event_collector.proto"
	tsFile := "../argus-frontend/app/lib/proto/types.ts"

	// Allow override via args
	if len(os.Args) > 1 {
		protoFile = os.Args[1]
	}
	if len(os.Args) > 2 {
		tsFile = os.Args[2]
	}

	fmt.Println("=================================================================")
	fmt.Println("  Argus AI — Contract Alignment Check")
	fmt.Printf("  Proto:      %s\n", protoFile)
	fmt.Printf("  TypeScript: %s\n", tsFile)
	fmt.Println("=================================================================")

	enums := []string{"EventType", "Severity", "EventSource", "TelemetryMode"}
	totalIssues := 0

	for i, enumName := range enums {
		fmt.Printf("\n[%d/%d] Checking %s...\n", i+1, len(enums), enumName)

		protoEntries, err := parseProtoEnum(protoFile, enumName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR parsing proto: %v\n", err)
			os.Exit(2)
		}

		tsEntries, err := parseTSEnum(tsFile, enumName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR parsing TypeScript: %v\n", err)
			os.Exit(2)
		}

		// Build TS lookup map
		tsMap := make(map[string]int, len(tsEntries))
		for _, e := range tsEntries {
			tsMap[e.Name] = e.Value
		}

		issues := 0
		for _, pe := range protoEntries {
			tsVal, exists := tsMap[pe.Name]
			if !exists {
				fmt.Printf("  MISSING:  %s.%s = %d (not in TypeScript)\n", enumName, pe.Name, pe.Value)
				issues++
			} else if tsVal != pe.Value {
				fmt.Printf("  MISMATCH: %s.%s = %d (proto) vs %d (TypeScript)\n", enumName, pe.Name, pe.Value, tsVal)
				issues++
			}
		}

		if issues == 0 {
			fmt.Printf("  OK: %d values aligned\n", len(protoEntries))
		}
		totalIssues += issues
	}

	fmt.Printf("\n=================================================================\n")
	fmt.Printf("  Issues: %d\n", totalIssues)
	fmt.Println("=================================================================")

	if totalIssues > 0 {
		fmt.Println("\nDESYNC DETECTED. Update TypeScript types to match proto.")
		os.Exit(1)
	}

	fmt.Println("\nALIGNED: All proto contracts match TypeScript types.")
}

// parseProtoEnum extracts enum values from a .proto file.
func parseProtoEnum(filePath, enumName string) ([]enumEntry, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	re := regexp.MustCompile(`^\s*([A-Z_][A-Z0-9_]*)\s*=\s*(\d+)\s*;`)
	enumStart := regexp.MustCompile(`^\s*enum\s+` + enumName + `\s*\{`)

	var entries []enumEntry
	inEnum := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if !inEnum {
			if enumStart.MatchString(line) {
				inEnum = true
			}
			continue
		}

		if strings.Contains(line, "}") {
			break
		}

		if m := re.FindStringSubmatch(line); m != nil {
			val, _ := strconv.Atoi(m[2])
			entries = append(entries, enumEntry{Name: m[1], Value: val})
		}
	}

	return entries, scanner.Err()
}

// parseTSEnum extracts enum values from a TypeScript file.
func parseTSEnum(filePath, enumName string) ([]enumEntry, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	re := regexp.MustCompile(`^\s*([A-Z_][A-Z0-9_]*)\s*=\s*(\d+)`)
	enumStart := regexp.MustCompile(`^\s*export\s+enum\s+` + enumName + `\s*\{`)

	var entries []enumEntry
	inEnum := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if !inEnum {
			if enumStart.MatchString(line) {
				inEnum = true
			}
			continue
		}

		if strings.TrimSpace(line) == "}" {
			break
		}

		if m := re.FindStringSubmatch(line); m != nil {
			val, _ := strconv.Atoi(m[2])
			entries = append(entries, enumEntry{Name: m[1], Value: val})
		}
	}

	return entries, scanner.Err()
}
