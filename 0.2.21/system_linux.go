//go:build linux

package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (c *metricCollector) collectHostMetrics() hostMetrics {
	hostname, _ := os.Hostname()
	metrics := hostMetrics{
		OS:       runtime.GOOS,
		Hostname: hostname,
		IP:       detectIP(),
		CPUCount: runtime.NumCPU(),
		PingMS:   measureServerPing(c.serverURL),
	}
	metrics.Platform = linuxPlatform()
	metrics.KernelVersion = linuxKernelVersion()
	metrics.CPUPercent, metrics.CPUCores = c.cpuPercent()
	metrics.CPUIOWaitPct = c.lastCPUIOWait
	metrics.MemoryTotalMB, metrics.MemoryUsedMB, metrics.MemoryPercent = linuxMemory()
	metrics.MemoryMB = metrics.MemoryUsedMB
	metrics.DiskTotalGB, metrics.DiskUsedGB, metrics.DiskPercent, metrics.Disks = linuxDisk()
	metrics.LoadAvg1, metrics.LoadAvg5, metrics.LoadAvg15 = linuxLoadAverage()
	metrics.UploadBPS, metrics.DownloadBPS = c.networkRate()
	metrics.DiskReadBPS, metrics.DiskWriteBPS, metrics.DiskReadIOPS, metrics.DiskWriteIOPS, metrics.DiskBusyPct = c.diskIORate()
	metrics.UptimeSec = linuxUptime()
	metrics.Services = linuxServices()
	metrics.ConfigProfiles = linuxConfigProfiles(metrics.Services)
	return metrics
}

func linuxPlatform() string {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "linux"
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	values := map[string]string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		values[parts[0]] = strings.Trim(parts[1], `"`)
	}
	if pretty := values["PRETTY_NAME"]; pretty != "" {
		return pretty
	}
	if name := values["NAME"]; name != "" {
		if version := values["VERSION"]; version != "" {
			return name + " " + version
		}
		return name
	}
	return "linux"
}

func linuxKernelVersion() string {
	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err != nil {
		return ""
	}
	buf := make([]byte, 0, len(uts.Release))
	for _, b := range uts.Release {
		if b == 0 {
			break
		}
		buf = append(buf, byte(b))
	}
	return string(buf)
}

func readCPUSnapshots() (cpuSnapshot, []cpuSnapshot, bool) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSnapshot{}, nil, false
	}
	defer file.Close()
	parseFields := func(fields []string) (cpuSnapshot, bool) {
		if len(fields) < 5 {
			return cpuSnapshot{}, false
		}
		var values []uint64
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return cpuSnapshot{}, false
			}
			values = append(values, value)
		}
		idle := values[3]
		iowait := uint64(0)
		if len(values) > 4 {
			iowait = values[4]
			idle += iowait
		}
		var total uint64
		for _, value := range values {
			total += value
		}
		return cpuSnapshot{idle: idle, iowait: iowait, total: total}, true
	}
	scanner := bufio.NewScanner(file)
	var overall cpuSnapshot
	var cores []cpuSnapshot
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		switch {
		case fields[0] == "cpu":
			parsed, ok := parseFields(fields)
			if !ok {
				return cpuSnapshot{}, nil, false
			}
			overall = parsed
		case strings.HasPrefix(fields[0], "cpu"):
			parsed, ok := parseFields(fields)
			if !ok {
				return cpuSnapshot{}, nil, false
			}
			cores = append(cores, parsed)
		}
	}
	if overall.total == 0 {
		return cpuSnapshot{}, nil, false
	}
	return overall, cores, true
}

func usageDelta(current, previous cpuSnapshot) float64 {
	totalDelta := current.total - previous.total
	idleDelta := current.idle - previous.idle
	if totalDelta == 0 {
		return 0
	}
	used := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	if used < 0 {
		return 0
	}
	return used
}

func iowaitDelta(current, previous cpuSnapshot) float64 {
	totalDelta := current.total - previous.total
	if totalDelta == 0 || current.iowait < previous.iowait {
		return 0
	}
	value := float64(current.iowait-previous.iowait) / float64(totalDelta) * 100
	if value < 0 {
		return 0
	}
	return value
}

func (c *metricCollector) cpuPercent() (float64, []float64) {
	current, currentCores, ok := readCPUSnapshots()
	if !ok {
		return 0, nil
	}
	if !c.hasCPU {
		c.prevCPU = current
		c.hasCPU = true
		c.prevCPUCores = append([]cpuSnapshot(nil), currentCores...)
		time.Sleep(200 * time.Millisecond)
		current, currentCores, ok = readCPUSnapshots()
		if !ok {
			return 0, nil
		}
	}
	overall := usageDelta(current, c.prevCPU)
	c.lastCPUIOWait = iowaitDelta(current, c.prevCPU)
	c.prevCPU = current
	coreValues := make([]float64, 0, len(currentCores))
	if len(currentCores) > 0 {
		// Для истории по ядрам агенту нужен отдельный baseline по каждому CPU.
		// Пока держим его в срезе в порядке cpu0, cpu1, ... из /proc/stat.
		if len(c.prevCPUCores) != len(currentCores) {
			c.prevCPUCores = append([]cpuSnapshot(nil), currentCores...)
			return overall, nil
		}
		for index := range currentCores {
			coreValues = append(coreValues, usageDelta(currentCores[index], c.prevCPUCores[index]))
		}
		c.prevCPUCores = append([]cpuSnapshot(nil), currentCores...)
	}
	return overall, coreValues
}

func linuxMemory() (int64, int64, float64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	defer file.Close()
	var totalKB, availableKB int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = value
		case "MemAvailable:":
			availableKB = value
		}
	}
	if totalKB == 0 {
		return 0, 0, 0
	}
	usedKB := totalKB - availableKB
	if usedKB < 0 {
		usedKB = 0
	}
	totalMB := totalKB / 1024
	usedMB := usedKB / 1024
	return totalMB, usedMB, float64(usedKB) / float64(totalKB) * 100
}

func linuxDisk() (int64, int64, float64, []agentDisk) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return 0, 0, 0, nil
	}
	defer file.Close()
	skipFS := map[string]bool{
		"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true, "devpts": true,
		"overlay": true, "squashfs": true, "cgroup": true, "cgroup2": true, "pstore": true,
		"tracefs": true, "securityfs": true, "configfs": true, "fusectl": true, "mqueue": true,
		"debugfs": true, "autofs": true, "rpc_pipefs": true,
	}
	seen := map[string]bool{}
	disks := make([]agentDisk, 0)
	scanner := bufio.NewScanner(file)
	var rootTotal, rootUsed int64
	var rootPercent float64
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		device := fields[0]
		mount := fields[1]
		fsType := fields[2]
		if skipFS[fsType] || seen[mount] {
			continue
		}
		seen[mount] = true
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			continue
		}
		total := int64(stat.Blocks) * int64(stat.Bsize)
		free := int64(stat.Bavail) * int64(stat.Bsize)
		used := total - free
		if total <= 0 {
			continue
		}
		const gb = int64(1024 * 1024 * 1024)
		entry := agentDisk{
			Name:    filepath.Base(device),
			Mount:   mount,
			TotalGB: total / gb,
			UsedGB:  used / gb,
			Percent: float64(used) / float64(total) * 100,
			FS:      fsType,
		}
		disks = append(disks, entry)
		if mount == "/" {
			rootTotal = entry.TotalGB
			rootUsed = entry.UsedGB
			rootPercent = entry.Percent
		}
	}
	return rootTotal, rootUsed, rootPercent, disks
}

func linuxLoadAverage() (float64, float64, float64) {
	body, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(body))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	la1, _ := strconv.ParseFloat(fields[0], 64)
	la5, _ := strconv.ParseFloat(fields[1], 64)
	la15, _ := strconv.ParseFloat(fields[2], 64)
	return la1, la5, la15
}

func linuxUptime() int64 {
	body, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	parts := strings.Fields(string(body))
	if len(parts) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	return int64(value)
}

func (c *metricCollector) networkRate() (int64, int64) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	var rx, tx uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		rxValue, err1 := strconv.ParseUint(fields[0], 10, 64)
		txValue, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		rx += rxValue
		tx += txValue
	}
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

func linuxDiskDeviceEligible(name string) bool {
	switch {
	case strings.HasPrefix(name, "loop"), strings.HasPrefix(name, "ram"), strings.HasPrefix(name, "fd"), strings.HasPrefix(name, "sr"), strings.HasPrefix(name, "dm-"), strings.HasPrefix(name, "md"):
		return false
	case strings.HasPrefix(name, "nvme"):
		return !strings.Contains(name, "p")
	case strings.HasPrefix(name, "mmcblk"):
		return !strings.Contains(name, "p")
	case strings.HasPrefix(name, "sd"), strings.HasPrefix(name, "vd"), strings.HasPrefix(name, "hd"):
		return len(name) == 3
	case strings.HasPrefix(name, "xvd"):
		return len(name) == 4
	default:
		return false
	}
}

func readDiskIOSnapshot() (diskIOSnapshot, bool) {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return diskIOSnapshot{}, false
	}
	defer file.Close()

	var snapshot diskIOSnapshot
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}
		name := strings.TrimSpace(fields[2])
		if !linuxDiskDeviceEligible(name) {
			continue
		}
		readOps, err1 := strconv.ParseUint(fields[3], 10, 64)
		readSectors, err2 := strconv.ParseUint(fields[5], 10, 64)
		writeOps, err3 := strconv.ParseUint(fields[7], 10, 64)
		writeSectors, err4 := strconv.ParseUint(fields[9], 10, 64)
		ioMillis, err5 := strconv.ParseUint(fields[12], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			continue
		}
		snapshot.readOps += readOps
		snapshot.readSectors += readSectors
		snapshot.writeOps += writeOps
		snapshot.writeSectors += writeSectors
		snapshot.ioMillis += ioMillis
	}
	snapshot.at = time.Now()
	return snapshot, true
}

func (c *metricCollector) diskIORate() (int64, int64, float64, float64, float64) {
	current, ok := readDiskIOSnapshot()
	if !ok {
		return 0, 0, 0, 0, 0
	}
	if !c.hasDiskIO {
		c.prevDiskIO = current
		c.hasDiskIO = true
		return 0, 0, 0, 0, 0
	}
	seconds := current.at.Sub(c.prevDiskIO.at).Seconds()
	if seconds <= 0 ||
		current.readSectors < c.prevDiskIO.readSectors ||
		current.writeSectors < c.prevDiskIO.writeSectors ||
		current.readOps < c.prevDiskIO.readOps ||
		current.writeOps < c.prevDiskIO.writeOps ||
		current.ioMillis < c.prevDiskIO.ioMillis {
		c.prevDiskIO = current
		return 0, 0, 0, 0, 0
	}
	const sectorSize = 512.0
	readBytes := float64(current.readSectors-c.prevDiskIO.readSectors) * sectorSize
	writeBytes := float64(current.writeSectors-c.prevDiskIO.writeSectors) * sectorSize
	readBPS := int64(readBytes / seconds)
	writeBPS := int64(writeBytes / seconds)
	readIOPS := float64(current.readOps-c.prevDiskIO.readOps) / seconds
	writeIOPS := float64(current.writeOps-c.prevDiskIO.writeOps) / seconds
	busy := float64(current.ioMillis-c.prevDiskIO.ioMillis) / (seconds * 1000) * 100
	if busy < 0 {
		busy = 0
	}
	if busy > 100 {
		busy = 100
	}
	c.prevDiskIO = current
	return readBPS, writeBPS, readIOPS, writeIOPS, busy
}

func normalizeLinuxServiceStatus(active, sub string) (string, string) {
	active = strings.TrimSpace(strings.ToLower(active))
	sub = strings.TrimSpace(strings.ToLower(sub))
	note := strings.Trim(strings.Join([]string{active, sub}, "/"), "/")
	switch {
	case active == "failed" || sub == "failed":
		return "failed", note
	case active == "active":
		return "running", note
	default:
		return "stopped", note
	}
}

func linuxServices() []agentService {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain").Output()
	if err != nil {
		return nil
	}
	items := make([]agentService, 0, 64)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) < 4 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if name == "" || !strings.HasSuffix(name, ".service") {
			continue
		}
		status, note := normalizeLinuxServiceStatus(fields[2], fields[3])
		items = append(items, agentService{Name: name, Status: status, StatusNote: note})
	}
	slices.SortFunc(items, func(left, right agentService) int {
		return strings.Compare(left.Name, right.Name)
	})
	return items
}

type configProfileSpec struct {
	key          string
	serviceNames []string
	filePaths    []string
	includeGlobs []string
}

func linuxConfigProfiles(services []agentService) []agentConfigProfile {
	specs := []configProfileSpec{
		{
			key:          "nginx",
			serviceNames: []string{"nginx.service"},
			filePaths:    []string{"/etc/nginx/nginx.conf"},
			includeGlobs: []string{"/etc/nginx/conf.d/*.conf", "/etc/nginx/sites-enabled/*"},
		},
		{
			key:          "apache",
			serviceNames: []string{"httpd.service", "apache2.service"},
			filePaths:    []string{"/etc/httpd/conf/httpd.conf", "/etc/apache2/apache2.conf"},
			includeGlobs: []string{"/etc/httpd/conf.d/*.conf", "/etc/apache2/conf-enabled/*", "/etc/apache2/sites-enabled/*"},
		},
		{
			key:          "mysql",
			serviceNames: []string{"mysqld.service", "mysql.service", "mariadb.service"},
			filePaths:    []string{"/etc/my.cnf", "/etc/mysql/my.cnf"},
			includeGlobs: []string{"/etc/mysql/conf.d/*", "/etc/mysql/mysql.conf.d/*"},
		},
	}
	out := make([]agentConfigProfile, 0, len(specs)+1)
	for _, spec := range specs {
		profile := collectStaticConfigProfile(spec, services)
		if profile.Key != "" {
			out = append(out, profile)
		}
	}
	if profile := collectPostgresConfigProfile(services); profile.Key != "" {
		out = append(out, profile)
	}
	slices.SortFunc(out, func(left, right agentConfigProfile) int {
		return strings.Compare(left.Key, right.Key)
	})
	return out
}

func collectStaticConfigProfile(spec configProfileSpec, services []agentService) agentConfigProfile {
	if !configProfileCandidate(spec.serviceNames, spec.filePaths, spec.includeGlobs, services) {
		return agentConfigProfile{}
	}
	entries := make([]agentConfigEntry, 0, len(spec.filePaths)+8)
	for _, path := range dedupeSortedStrings(spec.filePaths) {
		entries = append(entries, collectConfigFileEntry(path, "file"))
	}
	for _, pattern := range dedupeSortedStrings(spec.includeGlobs) {
		entries = append(entries, collectConfigGlobEntries(pattern)...)
	}
	return newAgentConfigProfile(spec.key, entries)
}

func collectPostgresConfigProfile(services []agentService) agentConfigProfile {
	mainPaths := dedupeSortedStrings(postgresMainConfigPaths())
	includeGlobs := []string{}
	for _, mainPath := range mainPaths {
		dir := filepath.Dir(mainPath)
		includeGlobs = append(includeGlobs, filepath.Join(dir, "conf.d", "*.conf"))
	}
	if !configProfileCandidate([]string{"postgresql.service"}, mainPaths, includeGlobs, services) {
		return agentConfigProfile{}
	}
	entries := make([]agentConfigEntry, 0, len(mainPaths)*3)
	for _, mainPath := range mainPaths {
		dir := filepath.Dir(mainPath)
		entries = append(entries, collectConfigFileEntry(mainPath, "file"))
		entries = append(entries, collectConfigFileEntry(filepath.Join(dir, "pg_hba.conf"), "file"))
		entries = append(entries, collectConfigFileEntry(filepath.Join(dir, "pg_ident.conf"), "file"))
		entries = append(entries, collectConfigGlobEntries(filepath.Join(dir, "conf.d", "*.conf"))...)
	}
	return newAgentConfigProfile("postgresql", entries)
}

func postgresMainConfigPaths() []string {
	paths := []string{
		"/var/lib/pgsql/data/postgresql.conf",
		"/var/lib/postgresql/data/postgresql.conf",
	}
	for _, pattern := range []string{
		"/etc/postgresql/*/*/postgresql.conf",
		"/var/lib/pgsql/*/data/postgresql.conf",
		"/var/lib/postgresql/*/main/postgresql.conf",
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		paths = append(paths, matches...)
	}
	return paths
}

func configProfileCandidate(serviceNames, filePaths, includeGlobs []string, services []agentService) bool {
	if hasMatchingService(services, serviceNames) {
		return true
	}
	for _, path := range filePaths {
		if fileExists(path) {
			return true
		}
	}
	for _, pattern := range includeGlobs {
		if len(globMatches(pattern)) > 0 {
			return true
		}
	}
	return false
}

func hasMatchingService(services []agentService, names []string) bool {
	if len(services) == 0 || len(names) == 0 {
		return false
	}
	normalized := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(strings.ToLower(name))
		if name != "" {
			normalized[name] = struct{}{}
		}
	}
	for _, service := range services {
		name := strings.TrimSpace(strings.ToLower(service.Name))
		if name == "" {
			continue
		}
		if _, ok := normalized[name]; ok {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func collectConfigFileEntry(path, kind string) agentConfigEntry {
	entry := agentConfigEntry{
		Path: strings.TrimSpace(path),
		Kind: firstNonEmpty(strings.TrimSpace(kind), "file"),
	}
	if entry.Path == "" {
		return entry
	}
	info, err := os.Stat(entry.Path)
	if err != nil {
		return entry
	}
	if !info.Mode().IsRegular() {
		return entry
	}
	entry.Exists = true
	entry.Size = info.Size()
	entry.ModifiedAt = info.ModTime().UTC()
	body, err := os.ReadFile(entry.Path)
	if err == nil {
		sum := sha256.Sum256(body)
		entry.SHA256 = hex.EncodeToString(sum[:])
	}
	return entry
}

func collectConfigGlobEntries(pattern string) []agentConfigEntry {
	matches := globMatches(pattern)
	if len(matches) == 0 {
		return nil
	}
	items := make([]agentConfigEntry, 0, len(matches))
	for _, match := range matches {
		entry := collectConfigFileEntry(match, "include-entry")
		if strings.TrimSpace(entry.Path) == "" {
			continue
		}
		items = append(items, entry)
	}
	slices.SortFunc(items, func(left, right agentConfigEntry) int {
		return strings.Compare(left.Path, right.Path)
	})
	return items
}

func globMatches(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, match)
	}
	return dedupeSortedStrings(out)
}

func newAgentConfigProfile(key string, entries []agentConfigEntry) agentConfigProfile {
	key = strings.TrimSpace(key)
	if key == "" {
		return agentConfigProfile{}
	}
	entries = dedupeConfigEntries(entries)
	return agentConfigProfile{
		Key:     key,
		Digest:  digestConfigEntries(entries),
		Entries: entries,
	}
}

func digestConfigEntries(entries []agentConfigEntry) string {
	sum := sha256.New()
	for _, entry := range dedupeConfigEntries(entries) {
		sum.Write([]byte(entry.Path))
		sum.Write([]byte{0})
		sum.Write([]byte(entry.Kind))
		sum.Write([]byte{0})
		if entry.Exists {
			sum.Write([]byte("1"))
		} else {
			sum.Write([]byte("0"))
		}
		sum.Write([]byte{0})
		sum.Write([]byte(strconv.FormatInt(entry.ModifiedAt.UTC().Unix(), 10)))
		sum.Write([]byte{0})
		sum.Write([]byte(strconv.FormatInt(entry.Size, 10)))
		sum.Write([]byte{0})
		sum.Write([]byte(entry.SHA256))
		sum.Write([]byte{'\n'})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func dedupeConfigEntries(entries []agentConfigEntry) []agentConfigEntry {
	if len(entries) == 0 {
		return []agentConfigEntry{}
	}
	seen := make(map[string]agentConfigEntry, len(entries))
	for _, entry := range entries {
		path := strings.TrimSpace(entry.Path)
		if path == "" {
			continue
		}
		entry.Path = path
		entry.Kind = firstNonEmpty(strings.TrimSpace(entry.Kind), "file")
		if entry.Exists {
			entry.ModifiedAt = entry.ModifiedAt.UTC()
		} else {
			entry.ModifiedAt = time.Time{}
			entry.Size = 0
			entry.SHA256 = ""
		}
		seen[path+"|"+entry.Kind] = entry
	}
	out := make([]agentConfigEntry, 0, len(seen))
	for _, entry := range seen {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(left, right agentConfigEntry) int {
		switch {
		case left.Path < right.Path:
			return -1
		case left.Path > right.Path:
			return 1
		}
		return strings.Compare(left.Kind, right.Kind)
	})
	return out
}

func dedupeSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
