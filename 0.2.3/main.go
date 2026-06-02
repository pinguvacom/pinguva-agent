package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	agentVersion   = "0.2.3"
	reportInterval = time.Minute
)

type agentService struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	StatusNote string `json:"statusNote,omitempty"`
}

type agentConfigEntry struct {
	Path       string    `json:"path"`
	Kind       string    `json:"kind"`
	Exists     bool      `json:"exists"`
	ModifiedAt time.Time `json:"modifiedAt,omitempty"`
	Size       int64     `json:"size,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
}

type agentConfigProfile struct {
	Key     string             `json:"key"`
	Digest  string             `json:"digest,omitempty"`
	Entries []agentConfigEntry `json:"entries,omitempty"`
}

type agentReport struct {
	AgentID        string               `json:"agentId"`
	InstallID      string               `json:"installId,omitempty"`
	Name           string               `json:"name"`
	Hostname       string               `json:"hostname"`
	OS             string               `json:"os"`
	Platform       string               `json:"platform,omitempty"`
	KernelVersion  string               `json:"kernelVersion,omitempty"`
	Arch           string               `json:"arch"`
	IP             string               `json:"ip"`
	Version        string               `json:"version"`
	CPUCount       int                  `json:"cpuCount"`
	CPUPercent     float64              `json:"cpuPercent"`
	GoRoutines     int                  `json:"goRoutines"`
	MemoryMB       int64                `json:"memoryMb"`
	MemoryTotalMB  int64                `json:"memoryTotalMb"`
	MemoryUsedMB   int64                `json:"memoryUsedMb"`
	MemoryPercent  float64              `json:"memoryPercent"`
	DiskTotalGB    int64                `json:"diskTotalGb"`
	DiskUsedGB     int64                `json:"diskUsedGb"`
	DiskPercent    float64              `json:"diskPercent"`
	LoadAvg1       float64              `json:"loadAvg1"`
	LoadAvg5       float64              `json:"loadAvg5"`
	LoadAvg15      float64              `json:"loadAvg15"`
	CPUCores       []float64            `json:"cpuCores,omitempty"`
	Disks          []agentDisk          `json:"disks,omitempty"`
	UploadBPS      int64                `json:"uploadBps"`
	DownloadBPS    int64                `json:"downloadBps"`
	DiskReadBPS    int64                `json:"diskReadBps"`
	DiskWriteBPS   int64                `json:"diskWriteBps"`
	DiskReadIOPS   float64              `json:"diskReadIops"`
	DiskWriteIOPS  float64              `json:"diskWriteIops"`
	DiskBusyPct    float64              `json:"diskBusyPercent"`
	CPUIOWaitPct   float64              `json:"cpuIowaitPercent"`
	UptimeSec      int64                `json:"uptimeSec"`
	PingMS         int64                `json:"pingMs"`
	LastError      string               `json:"lastError,omitempty"`
	Services       []agentService       `json:"services,omitempty"`
	ConfigProfiles []agentConfigProfile `json:"configProfiles,omitempty"`
	ReportedAt     time.Time            `json:"reportedAt"`
}

type agentState struct {
	AgentID   string `json:"agentId"`
	Token     string `json:"token"`
	InstallID string `json:"installId"`
}

type hostMetrics struct {
	OS             string
	Platform       string
	KernelVersion  string
	Hostname       string
	IP             string
	CPUCount       int
	CPUPercent     float64
	MemoryMB       int64
	MemoryTotalMB  int64
	MemoryUsedMB   int64
	MemoryPercent  float64
	DiskTotalGB    int64
	DiskUsedGB     int64
	DiskPercent    float64
	LoadAvg1       float64
	LoadAvg5       float64
	LoadAvg15      float64
	CPUCores       []float64
	Disks          []agentDisk
	UploadBPS      int64
	DownloadBPS    int64
	DiskReadBPS    int64
	DiskWriteBPS   int64
	DiskReadIOPS   float64
	DiskWriteIOPS  float64
	DiskBusyPct    float64
	CPUIOWaitPct   float64
	UptimeSec      int64
	PingMS         int64
	Services       []agentService
	ConfigProfiles []agentConfigProfile
}

type agentDisk struct {
	Name    string  `json:"name"`
	Mount   string  `json:"mount"`
	TotalGB int64   `json:"totalGb"`
	UsedGB  int64   `json:"usedGb"`
	Percent float64 `json:"percent"`
	FS      string  `json:"fs,omitempty"`
}

type cpuSnapshot struct {
	idle   uint64
	iowait uint64
	total  uint64
}

type netSnapshot struct {
	rx uint64
	tx uint64
	at time.Time
}

type diskIOSnapshot struct {
	readOps      uint64
	writeOps     uint64
	readSectors  uint64
	writeSectors uint64
	ioMillis     uint64
	at           time.Time
}

type metricCollector struct {
	serverURL     string
	prevCPU       cpuSnapshot
	prevCPUCores  []cpuSnapshot
	hasCPU        bool
	lastCPUIOWait float64
	prevNet       netSnapshot
	hasNet        bool
	prevDiskIO    diskIOSnapshot
	hasDiskIO     bool
}

type agentConfig struct {
	ServerURL       string
	StatePath       string
	Name            string
	Once            bool
	EnrollmentToken string
	PermanentToken  string
	ServiceName     string
}

func main() {
	serverURL := flag.String("server", envOr("AGENT_SERVER", "http://127.0.0.1:8080"), "Server base URL")
	statePath := flag.String("state-path", envOr("AGENT_STATE_PATH", defaultStatePath()), "Persistent agent state path")
	name := flag.String("name", envOr("AGENT_NAME", ""), "Agent display name")
	enrollmentToken := flag.String("enrollment-token", envOr("AGENT_ENROLLMENT_TOKEN", ""), "Temporary enrollment token")
	permanentToken := flag.String("token", envOr("AGENT_TOKEN", ""), "Permanent agent token")
	serviceName := flag.String("service-name", envOr("AGENT_SERVICE_NAME", defaultServiceName()), "Windows service name")
	once := flag.Bool("once", false, "Send one report and exit")
	flag.Parse()

	logger := log.New(os.Stdout, "[agent] ", log.LstdFlags|log.Lmsgprefix)
	cfg := agentConfig{
		ServerURL:       strings.TrimRight(*serverURL, "/"),
		StatePath:       *statePath,
		Name:            *name,
		Once:            *once,
		EnrollmentToken: strings.TrimSpace(*enrollmentToken),
		PermanentToken:  strings.TrimSpace(*permanentToken),
		ServiceName:     strings.TrimSpace(*serviceName),
	}
	if err := runAgent(cfg, logger); err != nil {
		logger.Fatalf("agent failed: %v", err)
	}
}

func runAgentLoop(ctx context.Context, cfg agentConfig, logger *log.Logger) error {
	client := &http.Client{Timeout: 15 * time.Second}
	collector := &metricCollector{serverURL: cfg.ServerURL}
	state, err := loadAgentState(cfg.StatePath)
	if err != nil {
		return err
	}
	if state == nil {
		state = &agentState{}
	}
	if strings.TrimSpace(state.InstallID) == "" {
		state.InstallID = newInstallID()
		if err := saveAgentState(cfg.StatePath, state); err != nil {
			logger.Printf("state save failed: %v", err)
		}
	}
	if state.Token == "" && cfg.PermanentToken != "" {
		state.Token = cfg.PermanentToken
	}

	send := func() {
		report := collectReport(state.AgentID, state.InstallID, cfg.Name, collector)
		if state.Token == "" {
			if cfg.EnrollmentToken == "" {
				logger.Printf("report skipped: no AGENT_TOKEN and no AGENT_ENROLLMENT_TOKEN")
				return
			}
			enrolled, err := enrollAgent(client, collector.serverURL+"/api/agent/enroll", cfg.EnrollmentToken, report)
			if err != nil {
				logger.Printf("enrollment failed: %v", err)
				return
			}
			state = enrolled
			if err := saveAgentState(cfg.StatePath, state); err != nil {
				logger.Printf("state save failed: %v", err)
			}
			report.AgentID = state.AgentID
		}
		if state.AgentID != "" {
			report.AgentID = state.AgentID
		}
		if err := postReport(client, collector.serverURL+"/api/agent/report", state.Token, report); err != nil {
			if cfg.EnrollmentToken != "" && isAuthFailure(err) {
				// Если токен агента уже недействителен, но в окружении остался
				// enrollment token, значит это переустановка или повторная привязка.
				// В этом случае агент сам сбрасывает локальный state и проходит
				// регистрацию заново без ручного удаления state.json.
				logger.Printf("report auth failed, re-enrolling agent")
				state = &agentState{}
				enrolled, enrollErr := enrollAgent(client, collector.serverURL+"/api/agent/enroll", cfg.EnrollmentToken, report)
				if enrollErr != nil {
					logger.Printf("re-enrollment failed: %v", enrollErr)
					return
				}
				state = enrolled
				if err := saveAgentState(cfg.StatePath, state); err != nil {
					logger.Printf("state save failed: %v", err)
				}
				report.AgentID = state.AgentID
				if err := postReport(client, collector.serverURL+"/api/agent/report", state.Token, report); err != nil {
					logger.Printf("report failed after re-enrollment: %v", err)
					return
				}
				logger.Printf("report sent: agent=%s cpu=%.1f%% mem=%.1f%% disk=%.1f%% up=%dB/s down=%dB/s ping=%dms", report.AgentID, report.CPUPercent, report.MemoryPercent, report.DiskPercent, report.UploadBPS, report.DownloadBPS, report.PingMS)
				return
			}
			logger.Printf("report failed: %v", err)
			return
		}
		logger.Printf("report sent: agent=%s cpu=%.1f%% mem=%.1f%% disk=%.1f%% up=%dB/s down=%dB/s ping=%dms", report.AgentID, report.CPUPercent, report.MemoryPercent, report.DiskPercent, report.UploadBPS, report.DownloadBPS, report.PingMS)
	}

	send()
	if cfg.Once {
		return nil
	}

	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			send()
		}
	}
}

func collectReport(agentID, installID, configuredName string, collector *metricCollector) agentReport {
	metrics := collector.collectHostMetrics()
	name := strings.TrimSpace(configuredName)
	if name == "" {
		name = metrics.Hostname
	}
	if name == "" {
		name = "Pinguva Agent"
	}
	return agentReport{
		AgentID:        agentID,
		InstallID:      installID,
		Name:           name,
		Hostname:       metrics.Hostname,
		OS:             metrics.OS,
		Platform:       metrics.Platform,
		KernelVersion:  metrics.KernelVersion,
		Arch:           runtime.GOARCH,
		IP:             metrics.IP,
		Version:        agentVersion,
		CPUCount:       metrics.CPUCount,
		CPUPercent:     metrics.CPUPercent,
		GoRoutines:     runtime.NumGoroutine(),
		MemoryMB:       metrics.MemoryMB,
		MemoryTotalMB:  metrics.MemoryTotalMB,
		MemoryUsedMB:   metrics.MemoryUsedMB,
		MemoryPercent:  metrics.MemoryPercent,
		DiskTotalGB:    metrics.DiskTotalGB,
		DiskUsedGB:     metrics.DiskUsedGB,
		DiskPercent:    metrics.DiskPercent,
		LoadAvg1:       metrics.LoadAvg1,
		LoadAvg5:       metrics.LoadAvg5,
		LoadAvg15:      metrics.LoadAvg15,
		CPUCores:       metrics.CPUCores,
		Disks:          metrics.Disks,
		UploadBPS:      metrics.UploadBPS,
		DownloadBPS:    metrics.DownloadBPS,
		DiskReadBPS:    metrics.DiskReadBPS,
		DiskWriteBPS:   metrics.DiskWriteBPS,
		DiskReadIOPS:   metrics.DiskReadIOPS,
		DiskWriteIOPS:  metrics.DiskWriteIOPS,
		DiskBusyPct:    metrics.DiskBusyPct,
		CPUIOWaitPct:   metrics.CPUIOWaitPct,
		UptimeSec:      metrics.UptimeSec,
		PingMS:         metrics.PingMS,
		Services:       append([]agentService(nil), metrics.Services...),
		ConfigProfiles: append([]agentConfigProfile(nil), metrics.ConfigProfiles...),
		ReportedAt:     time.Now().UTC(),
	}
}

func enrollAgent(client *http.Client, endpoint, token string, report agentReport) (*agentState, error) {
	body, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &httpError{Status: resp.Status}
	}
	var payload struct {
		AgentID string `json:"agentId"`
		Token   string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.AgentID) == "" || strings.TrimSpace(payload.Token) == "" {
		return nil, &httpError{Status: "invalid enrollment response"}
	}
	return &agentState{AgentID: payload.AgentID, Token: payload.Token}, nil
}

func loadAgentState(path string) (*agentState, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state agentState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveAgentState(path string, state *agentState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func newInstallID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "ins_fallback"
	}
	return "ins_" + hex.EncodeToString(raw[:])
}

func defaultStatePath() string {
	if runtime.GOOS == "linux" {
		return "/var/lib/pinguva-agent/state.json"
	}
	if runtime.GOOS == "windows" {
		programData := strings.TrimSpace(os.Getenv("ProgramData"))
		if programData != "" {
			return filepath.Join(programData, "Pinguva", "agent", "state.json")
		}
	}
	return "pinguva-agent-state.json"
}

func defaultServiceName() string {
	if runtime.GOOS == "windows" {
		return "PinguvaAgent"
	}
	return ""
}

func postReport(client *http.Client, endpoint, token string, report agentReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return &httpError{Status: resp.Status}
	}
	return nil
}

func detectIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip == nil || ip.IsLoopback() {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type httpError struct {
	Status string
}

func (e *httpError) Error() string {
	return e.Status
}

func isAuthFailure(err error) bool {
	var httpErr *httpError
	if !errors.As(err, &httpErr) {
		return false
	}
	return strings.HasPrefix(httpErr.Status, "401") || strings.HasPrefix(httpErr.Status, "403")
}

func measureServerPing(serverURL string) int64 {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return 0
	}
	host := parsed.Host
	if !strings.Contains(host, ":") {
		switch parsed.Scheme {
		case "https":
			host += ":443"
		default:
			host += ":80"
		}
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err != nil {
		return 0
	}
	_ = conn.Close()
	return time.Since(start).Milliseconds()
}
