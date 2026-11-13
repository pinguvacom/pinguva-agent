//go:build !linux && !windows

package main

import (
	"os"
	"runtime"
	"time"
)

func (c *metricCollector) collectHostMetrics() hostMetrics {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	hostname, _ := os.Hostname()
	usedMB := int64(mem.Alloc / 1024 / 1024)
	return hostMetrics{
		OS:            runtime.GOOS,
		Platform:      runtime.GOOS,
		Hostname:      hostname,
		IP:            detectIP(),
		CPUCount:      runtime.NumCPU(),
		MemoryMB:      usedMB,
		MemoryUsedMB:  usedMB,
		MemoryTotalMB: usedMB,
		UptimeSec:     int64(time.Now().Unix()),
		PingMS:        measureServerPing(c.serverURL),
	}
}
