package process

import (
	"os/exec"
	"strconv"
	"strings"
)

// sampleResources returns the CPU percentage and resident memory (bytes) for a
// PID by shelling out to ps. It works on macOS and Linux. On any error it
// returns zeroes — monitoring is best-effort and must never block management.
//
// Note: this measures the tracked process only, not its child tree, and ps
// reports CPU as a lifetime average rather than an instantaneous sample.
func sampleResources(pid int) (cpuPercent float64, rssBytes int64) {
	out, err := exec.Command("ps", "-o", "%cpu=,rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, 0
	}
	cpu, _ := strconv.ParseFloat(fields[0], 64)
	rssKB, _ := strconv.ParseInt(fields[1], 10, 64)
	return cpu, rssKB * 1024
}
