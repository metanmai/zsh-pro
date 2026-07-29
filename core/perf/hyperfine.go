package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type hyperfineReport struct {
	Results []hyperfineResult `json:"results"`
}

type hyperfineResult struct {
	Command string  `json:"command"`
	Mean    float64 `json:"mean"`
}

func addedStartupMilliseconds(r io.Reader, withCommand, withoutCommand string) (float64, error) {
	if withCommand == withoutCommand {
		return 0, fmt.Errorf("with and without commands must differ")
	}

	var report hyperfineReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return 0, fmt.Errorf("decode hyperfine JSON: %w", err)
	}

	means := make(map[string]float64, 2)
	for _, result := range report.Results {
		if result.Command != withCommand && result.Command != withoutCommand {
			continue
		}
		if _, exists := means[result.Command]; exists {
			return 0, fmt.Errorf("hyperfine JSON contains duplicate result for %q", result.Command)
		}
		means[result.Command] = result.Mean
	}

	withMean, exists := means[withCommand]
	if !exists {
		return 0, fmt.Errorf("hyperfine JSON is missing result for %q", withCommand)
	}
	withoutMean, exists := means[withoutCommand]
	if !exists {
		return 0, fmt.Errorf("hyperfine JSON is missing result for %q", withoutCommand)
	}
	return (withMean - withoutMean) * 1000, nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: hyperfine <result.json> <with-command> <without-command>")
		os.Exit(2)
	}

	resultFile, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open hyperfine JSON: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resultFile.Close() }()

	added, err := addedStartupMilliseconds(resultFile, os.Args[2], os.Args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%.6f\n", added)
}
