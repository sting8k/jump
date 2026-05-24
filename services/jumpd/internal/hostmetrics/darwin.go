//go:build darwin

package hostmetrics

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var topIdleRe = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)% idle`)
var pmsetBatteryRe = regexp.MustCompile(`(?m)([0-9]+(?:\.[0-9]+)?)%;\s*([^;]+);`)

func cpuPercent(ctx context.Context) (float64, error) {
	out, err := run(ctx, "top", "-l", "1", "-n", "0", "-s", "0")
	if err != nil {
		return 0, err
	}
	m := topIdleRe.FindStringSubmatch(string(out))
	if len(m) != 2 {
		return 0, fmt.Errorf("top output missing idle CPU percentage")
	}
	idle, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, err
	}
	if idle < 0 {
		idle = 0
	}
	if idle > 100 {
		idle = 100
	}
	return 100 - idle, nil
}

func memoryUsage(ctx context.Context) (MemoryUsage, error) {
	totalOut, err := run(ctx, "sysctl", "-n", "hw.memsize")
	if err != nil {
		return MemoryUsage{}, err
	}
	total, err := strconv.ParseUint(strings.TrimSpace(string(totalOut)), 10, 64)
	if err != nil {
		return MemoryUsage{}, err
	}

	vmOut, err := run(ctx, "vm_stat")
	if err != nil {
		return MemoryUsage{}, err
	}
	pageSize, availablePages, err := parseVMStat(string(vmOut))
	if err != nil {
		return MemoryUsage{}, err
	}
	available := availablePages * pageSize
	used := total
	if available < total {
		used = total - available
	}
	return MemoryUsage{UsedBytes: used, TotalBytes: total}, nil
}

func batteryStatus(ctx context.Context) (*BatteryStatus, error) {
	out, err := run(ctx, "pmset", "-g", "batt")
	if err != nil {
		return nil, err
	}
	return parsePMSetBattery(string(out))
}

func parsePMSetBattery(out string) (*BatteryStatus, error) {
	lower := strings.ToLower(out)
	if strings.Contains(lower, "no batteries") || strings.Contains(lower, "present: false") {
		return nil, nil
	}
	m := pmsetBatteryRe.FindStringSubmatch(out)
	if len(m) != 3 {
		return nil, nil
	}
	percent, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil, err
	}
	return &BatteryStatus{Percent: percent, State: m[2]}, nil
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func parseVMStat(out string) (pageSize, availablePages uint64, err error) {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "page size of") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "of" && i+1 < len(fields) {
					pageSize, _ = strconv.ParseUint(fields[i+1], 10, 64)
				}
			}
			continue
		}
		label, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		label = strings.TrimSpace(label)
		if label != "Pages free" && label != "Pages speculative" && label != "Pages inactive" {
			continue
		}
		v := strings.Trim(strings.TrimSpace(value), ".")
		pages, parseErr := strconv.ParseUint(v, 10, 64)
		if parseErr != nil {
			return 0, 0, parseErr
		}
		availablePages += pages
	}
	if pageSize == 0 {
		return 0, 0, fmt.Errorf("vm_stat page size missing")
	}
	return pageSize, availablePages, nil
}
