package application

import (
	"os"
	"testing"
)

func c3PerfSizes(t *testing.T) []string {
	t.Helper()
	switch value := os.Getenv("NW_C3_PERF_SIZE"); value {
	case "":
		return []string{"small", "large"}
	case "small", "large":
		return []string{value}
	default:
		t.Fatal("invalid NW_C3_PERF_SIZE")
		return nil
	}
}
