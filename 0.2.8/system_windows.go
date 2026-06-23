//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

func (c *metricCollector) collectHostMetrics() hostMetrics {
	hostname, _ := os.Hostname()
	info, _ := host.InfoWithContext(context.Background())
	metrics := hostMetrics{
		OS:       runtime.GOOS,
		Hostname: hostname,
		IP:       detectIP(),
		CPUCount: runtime.NumCPU(),
		PingMS:   measureServerPing(c.serverURL),
	}
	if info != nil {
		if platform := strings.TrimSpace(info.Platform); platform != "" {
			if version := strings.TrimSpace(info.PlatformVersion); version != "" {
				metrics.Platform = platform + " " + version
			} else {
				metrics.Platform = platform
			}
		}
		metrics.KernelVersion = strings.TrimSpace(info.KernelVersion)
		metrics.UptimeSec = int64(info.Uptime)
	}
	if metrics.Platform == "" {
		metrics.Platform = "windows"
	}
	if totalMB, usedMB, percent := windowsMemory(); totalMB > 0 {
		metrics.MemoryTotalMB = totalMB
		metrics.MemoryUsedMB = usedMB
		metrics.MemoryMB = usedMB
		metrics.MemoryPercent = percent
	}
	metrics.CPUPercent, metrics.CPUCores = windowsCPUPercent()
	metrics.DiskTotalGB, metrics.DiskUsedGB, metrics.DiskPercent, metrics.Disks = windowsDisk()
	metrics.UploadBPS, metrics.DownloadBPS = c.networkRateWindows()
	return metrics
}

func windowsCPUPercent() (float64, []float64) {
	coreValues, err := cpu.PercentWithContext(context.Background(), time.Second, true)
	if err != nil {
		return 0, nil
	}
	if len(coreValues) == 0 {
		overall, err := cpu.PercentWithContext(context.Background(), 0, false)
		if err != nil || len(overall) == 0 {
			return 0, nil
		}
		return overall[0], nil
	}
	var sum float64
	for _, value := range coreValues {
		sum += value
	}
	return sum / float64(len(coreValues)), coreValues
}

func windowsMemory() (int64, int64, float64) {
	vm, err := mem.VirtualMemoryWithContext(context.Background())
	if err != nil || vm == nil || vm.Total == 0 {
		return 0, 0, 0
	}
	return int64(vm.Total / 1024 / 1024), int64(vm.Used / 1024 / 1024), vm.UsedPercent
}

func windowsDisk() (int64, int64, float64, []agentDisk) {
	partitions, err := disk.PartitionsWithContext(context.Background(), false)
	if err != nil {
		return 0, 0, 0, nil
	}
	systemDrive := strings.ToUpper(strings.TrimSpace(os.Getenv("SystemDrive")))
	if systemDrive == "" {
		systemDrive = "C:"
	}
	systemMountCandidates := map[string]bool{
		systemDrive:                        true,
		systemDrive + "\\":                 true,
		filepath.Clean(systemDrive):        true,
		filepath.Clean(systemDrive + "\\"): true,
	}
	disks := make([]agentDisk, 0, len(partitions))
	var rootTotal, rootUsed int64
	var rootPercent float64
	for _, part := range partitions {
		mount := strings.TrimSpace(part.Mountpoint)
		if mount == "" {
			continue
		}
		usage, err := disk.UsageWithContext(context.Background(), mount)
		if err != nil || usage == nil || usage.Total == 0 {
			continue
		}
		entry := agentDisk{
			Name:    filepath.Clean(mount),
			Mount:   mount,
			TotalGB: int64(usage.Total / 1024 / 1024 / 1024),
			UsedGB:  int64(usage.Used / 1024 / 1024 / 1024),
			Percent: usage.UsedPercent,
			FS:      part.Fstype,
		}
		disks = append(disks, entry)
		cleanMount := filepath.Clean(mount)
		if rootTotal == 0 && (systemMountCandidates[mount] || systemMountCandidates[cleanMount]) {
			rootTotal = entry.TotalGB
			rootUsed = entry.UsedGB
			rootPercent = entry.Percent
		}
	}
	if rootTotal == 0 && len(disks) > 0 {
		rootTotal = disks[0].TotalGB
		rootUsed = disks[0].UsedGB
		rootPercent = disks[0].Percent
	}
	return rootTotal, rootUsed, rootPercent, disks
}

func (c *metricCollector) networkRateWindows() (int64, int64) {
	counters, err := gnet.IOCountersWithContext(context.Background(), false)
	if err != nil || len(counters) == 0 {
		return 0, 0
	}
	rx := counters[0].BytesRecv
	tx := counters[0].BytesSent
	now := time.Now()
	if !c.hasNet {
		c.prevNet = netSnapshot{rx: rx, tx: tx, at: now}
		c.hasNet = true
		return 0, 0
	}
	seconds := now.Sub(c.prevNet.at).Seconds()
	if seconds <= 0 {
		return 0, 0
	}
	download := int64(float64(rx-c.prevNet.rx) / seconds)
	upload := int64(float64(tx-c.prevNet.tx) / seconds)
	c.prevNet = netSnapshot{rx: rx, tx: tx, at: now}
	if upload < 0 {
		upload = 0
	}
	if download < 0 {
		download = 0
	}
	return upload, download
}
