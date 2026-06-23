package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	agentVersion   = "0.2.5"
	reportInterval = time.Minute
	linuxEnvPath   = "/etc/pinguva-agent.env"
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

type agentBitrix24Status struct {
	Configured bool                        `json:"configured"`
	Status     string                      `json:"status,omitempty"`
	ResponseMS int64                       `json:"responseMs,omitempty"`
	CheckedAt  time.Time                   `json:"checkedAt,omitempty"`
	Error      string                      `json:"error,omitempty"`
	Summary    *agentBitrix24Summary       `json:"summary,omitempty"`
	Methods    []agentBitrix24MethodStatus `json:"methods,omitempty"`
}

type agentBitrix24Summary struct {
	MethodCount   int       `json:"methodCount,omitempty"`
	OKCount       int       `json:"okCount,omitempty"`
	ErrorCount    int       `json:"errorCount,omitempty"`
	AvgResponseMS int64     `json:"avgResponseMs,omitempty"`
	MaxResponseMS int64     `json:"maxResponseMs,omitempty"`
	SlowestMethod string    `json:"slowestMethod,omitempty"`
	RateLimited   bool      `json:"rateLimited,omitempty"`
	LastSuccessAt time.Time `json:"lastSuccessAt,omitempty"`
}

type agentBitrix24MethodStatus struct {
	Key         string    `json:"key,omitempty"`
	Profile     string    `json:"profile,omitempty"`
	Name        string    `json:"name,omitempty"`
	Method      string    `json:"method,omitempty"`
	Status      string    `json:"status,omitempty"`
	ResponseMS  int64     `json:"responseMs,omitempty"`
	ResultCount int64     `json:"resultCount,omitempty"`
	CheckedAt   time.Time `json:"checkedAt,omitempty"`
	Error       string    `json:"error,omitempty"`
	RateLimited bool      `json:"rateLimited,omitempty"`
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
	Bitrix24       *agentBitrix24Status `json:"bitrix24,omitempty"`
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
	Bitrix24Path    string
	Name            string
	Once            bool
	EnrollmentToken string
	PermanentToken  string
	ServiceName     string
}

func main() {
	logger := log.New(os.Stdout, "[agent] ", log.LstdFlags|log.Lmsgprefix)
	if len(os.Args) > 1 && os.Args[1] == "bitrix24" {
		if err := runBitrix24Command(os.Args[2:], logger); err != nil {
			logger.Fatalf("bitrix24 command failed: %v", err)
		}
		return
	}

	serverURL := flag.String("server", envOr("AGENT_SERVER", "http://127.0.0.1:8080"), "Server base URL")
	statePath := flag.String("state-path", envOr("AGENT_STATE_PATH", defaultStatePath()), "Persistent agent state path")
	bitrix24Path := flag.String("bitrix24-config-path", envOr("AGENT_BITRIX24_CONFIG_PATH", defaultBitrix24ConfigPath()), "Local Bitrix24 integration config path")
	name := flag.String("name", envOr("AGENT_NAME", ""), "Agent display name")
	enrollmentToken := flag.String("enrollment-token", envOr("AGENT_ENROLLMENT_TOKEN", ""), "Temporary enrollment token")
	permanentToken := flag.String("token", envOr("AGENT_TOKEN", ""), "Permanent agent token")
	serviceName := flag.String("service-name", envOr("AGENT_SERVICE_NAME", defaultServiceName()), "Windows service name")
	once := flag.Bool("once", false, "Send one report and exit")
	flag.Parse()

	cfg := agentConfig{
		ServerURL:       strings.TrimRight(*serverURL, "/"),
		StatePath:       *statePath,
		Bitrix24Path:    *bitrix24Path,
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

func validateAgentConfig(cfg agentConfig) error {
	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		return errors.New("AGENT_SERVER is not configured; set it in /etc/pinguva-agent.env or pass --server https://your-pinguva-host")
	}
	if serverURL == "http://127.0.0.1:8080" && runtime.GOOS == "linux" && linuxEnvFileHasEmpty("AGENT_SERVER") {
		return errors.New("AGENT_SERVER is empty in /etc/pinguva-agent.env; set the public Pinguva URL and restart pinguva-agent")
	}
	return nil
}

func runAgentLoop(ctx context.Context, cfg agentConfig, logger *log.Logger) error {
	if err := validateAgentConfig(cfg); err != nil {
		return err
	}
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
		report.Bitrix24 = collectBitrix24Status(client, cfg.Bitrix24Path)
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

func defaultBitrix24ConfigPath() string {
	if runtime.GOOS == "linux" {
		return "/etc/pinguva-agent/bitrix24.json"
	}
	if runtime.GOOS == "windows" {
		programData := strings.TrimSpace(os.Getenv("ProgramData"))
		if programData != "" {
			return filepath.Join(programData, "Pinguva", "agent", "bitrix24.json")
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".pinguva-agent", "bitrix24.json")
	}
	return "pinguva-agent-bitrix24.json"
}

type bitrix24LocalConfig struct {
	BaseURL    string    `json:"baseUrl"`
	WebhookURL string    `json:"webhookUrl"`
	Profiles   []string  `json:"profiles,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type bitrix24ProfileSpec struct {
	Key         string
	Name        string
	Method      string
	Payload     map[string]any
	Critical    bool
	ResultCount bool
}

var sensitiveBitrix24ParamPattern = regexp.MustCompile(`(?i)(auth|access_token|refresh_token|webhook|token)=([^&\s]+)`)

func defaultBitrix24ProfileKeys() []string {
	return []string{"basic", "scope", "crm_deals", "crm_leads", "crm_contacts", "crm_statuses"}
}

func bitrix24ProfileCatalog() map[string]bitrix24ProfileSpec {
	return map[string]bitrix24ProfileSpec{
		"basic": {
			Key:      "basic",
			Name:     "REST доступность",
			Method:   "user.current",
			Payload:  map[string]any{},
			Critical: true,
		},
		"scope": {
			Key:     "scope",
			Name:    "Проверка выданных REST-прав",
			Method:  "scope",
			Payload: map[string]any{},
		},
		"method_discovery": {
			Key:     "method_discovery",
			Name:    "Проверка доступности user.get",
			Method:  "method.get",
			Payload: map[string]any{"name": "user.get"},
		},
		"crm_deals": {
			Key:         "crm_deals",
			Name:        "CRM сделки",
			Method:      "crm.item.list",
			Payload:     map[string]any{"entityTypeId": 2, "select": []string{"id"}, "start": 0},
			ResultCount: true,
		},
		"crm_leads": {
			Key:         "crm_leads",
			Name:        "CRM лиды",
			Method:      "crm.item.list",
			Payload:     map[string]any{"entityTypeId": 1, "select": []string{"id"}, "start": 0},
			ResultCount: true,
		},
		"crm_contacts": {
			Key:         "crm_contacts",
			Name:        "CRM контакты",
			Method:      "crm.item.list",
			Payload:     map[string]any{"entityTypeId": 3, "select": []string{"id"}, "start": 0},
			ResultCount: true,
		},
		"crm_statuses": {
			Key:         "crm_statuses",
			Name:        "CRM стадии",
			Method:      "crm.status.list",
			Payload:     map[string]any{"filter": map[string]any{"ENTITY_ID": "DEAL_STAGE"}},
			ResultCount: true,
		},
	}
}

func runBitrix24Command(args []string, logger *log.Logger) error {
	if len(args) == 0 {
		return errors.New("usage: pinguva-agent bitrix24 connect --base-url https://portal.example.kz")
	}
	switch args[0] {
	case "configure", "connect", "wizard":
		return runBitrix24Configure(args[1:], logger)
	case "status":
		return runBitrix24Status(args[1:], logger)
	default:
		return fmt.Errorf("unknown bitrix24 command %q", args[0])
	}
}

func runBitrix24Configure(args []string, logger *log.Logger) error {
	fs := flag.NewFlagSet("bitrix24 connect", flag.ContinueOnError)
	baseURLFlag := fs.String("base-url", envOr("BITRIX24_BASE_URL", ""), "Bitrix24 portal base URL")
	profilesFlag := fs.String("profiles", envOr("BITRIX24_PROFILES", strings.Join(defaultBitrix24ProfileKeys(), ",")), "Comma-separated safe Bitrix24 profiles")
	configPathFlag := fs.String("config-path", envOr("AGENT_BITRIX24_CONFIG_PATH", defaultBitrix24ConfigPath()), "Local Bitrix24 config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	baseURL, err := normalizeBitrix24BaseURL(*baseURLFlag)
	if err != nil {
		return err
	}
	profiles, err := normalizeBitrix24Profiles(*profilesFlag)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "Bitrix24 local REST monitoring setup")
	fmt.Fprintf(os.Stdout, "Portal / Портал: %s\n", baseURL)
	fmt.Fprintln(os.Stdout, "Create an incoming webhook in Bitrix24 and paste it here.")
	fmt.Fprintln(os.Stdout, "Создайте входящий webhook в Bitrix24 и вставьте его сюда.")
	fmt.Fprint(os.Stdout, "Bitrix24 webhook URL / Входящий webhook Bitrix24 (hidden input, not the portal URL): ")
	webhookRaw, err := readHiddenTerminalLine()
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return err
	}
	webhookURL, err := normalizeBitrix24WebhookURL(string(webhookRaw))
	if err != nil {
		return err
	}
	if err := validateBitrix24WebhookHost(baseURL, webhookURL); err != nil {
		return err
	}

	now := time.Now().UTC()
	config := bitrix24LocalConfig{
		BaseURL:    baseURL,
		WebhookURL: webhookURL,
		Profiles:   profiles,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if existing, err := loadBitrix24Config(*configPathFlag); err == nil && existing != nil && !existing.CreatedAt.IsZero() {
		config.CreatedAt = existing.CreatedAt
	}
	if err := saveBitrix24Config(*configPathFlag, config); err != nil {
		return err
	}
	logger.Printf("Bitrix24 integration saved locally: %s", *configPathFlag)
	logger.Printf("Bitrix24 profiles: %s", strings.Join(config.Profiles, ", "))
	logger.Printf("Webhook secret stays on this server and is not sent to Pinguva.")
	status := checkBitrix24Webhook(&http.Client{Timeout: 15 * time.Second}, config)
	if status != nil && status.Status == "ok" {
		logger.Printf("Bitrix24 check ok: %dms", status.ResponseMS)
	} else if status != nil && strings.TrimSpace(status.Error) != "" {
		logger.Printf("Bitrix24 check failed: %s", status.Error)
	}
	return nil
}

func readHiddenTerminalLine() ([]byte, error) {
	input := os.Stdin
	closeInput := false
	if !term.IsTerminal(int(input.Fd())) {
		if runtime.GOOS == "windows" {
			return nil, errors.New("interactive terminal is required for hidden Bitrix24 webhook input")
		}
		tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
		if err != nil {
			return nil, errors.New("interactive terminal is required for hidden Bitrix24 webhook input; run the command from a terminal")
		}
		input = tty
		closeInput = true
	}
	if closeInput {
		defer input.Close()
	}
	return term.ReadPassword(int(input.Fd()))
}

func runBitrix24Status(args []string, logger *log.Logger) error {
	fs := flag.NewFlagSet("bitrix24 status", flag.ContinueOnError)
	configPathFlag := fs.String("config-path", envOr("AGENT_BITRIX24_CONFIG_PATH", defaultBitrix24ConfigPath()), "Local Bitrix24 config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	status := collectBitrix24Status(&http.Client{Timeout: 15 * time.Second}, *configPathFlag)
	if status == nil || !status.Configured {
		logger.Printf("Bitrix24 integration is not configured.")
		return nil
	}
	if status.Status == "ok" {
		logger.Printf("Bitrix24 check ok: %dms", status.ResponseMS)
		return nil
	}
	logger.Printf("Bitrix24 check failed: %s", status.Error)
	return nil
}

func loadBitrix24Config(path string) (*bitrix24LocalConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var config bitrix24LocalConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, err
	}
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.WebhookURL = strings.TrimSpace(config.WebhookURL)
	if config.WebhookURL == "" {
		return nil, errors.New("bitrix24 webhook URL is empty")
	}
	config.Profiles, err = normalizeBitrix24Profiles(strings.Join(config.Profiles, ","))
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func saveBitrix24Config(path string, config bitrix24LocalConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return err
	}
	return restrictBitrix24ConfigPermissions(path)
}

func restrictBitrix24ConfigPermissions(path string) error {
	if runtime.GOOS == "linux" {
		if gid, ok := lookupUnixGroupID("pinguva-agent"); ok {
			dir := filepath.Dir(path)
			_ = os.Chown(dir, -1, gid)
			_ = os.Chmod(dir, 0o750)
			_ = os.Chown(path, -1, gid)
			return os.Chmod(path, 0o640)
		}
	}
	return os.Chmod(path, 0o600)
}

func lookupUnixGroupID(name string) (int, bool) {
	body, err := os.ReadFile("/etc/group")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 3 || parts[0] != name {
			continue
		}
		gid, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, false
		}
		return gid, true
	}
	return 0, false
}

func collectBitrix24Status(client *http.Client, path string) *agentBitrix24Status {
	config, err := loadBitrix24Config(path)
	if err != nil {
		return &agentBitrix24Status{Configured: true, Status: "error", CheckedAt: time.Now().UTC(), Error: safeBitrix24Error(err.Error(), nil)}
	}
	if config == nil {
		return &agentBitrix24Status{Configured: false, Status: "not_configured"}
	}
	return checkBitrix24Webhook(client, *config)
}

func checkBitrix24Webhook(client *http.Client, config bitrix24LocalConfig) *agentBitrix24Status {
	status := &agentBitrix24Status{
		Configured: true,
		Status:     "error",
	}
	profiles := bitrix24SpecsForProfiles(config.Profiles)
	if len(profiles) == 0 {
		profiles = bitrix24SpecsForProfiles(defaultBitrix24ProfileKeys())
	}

	methods := make([]agentBitrix24MethodStatus, 0, len(profiles))
	for _, spec := range profiles {
		methodStatus := checkBitrix24Profile(client, config, spec)
		methods = append(methods, methodStatus)
		if spec.Critical {
			status.ResponseMS = methodStatus.ResponseMS
			status.CheckedAt = methodStatus.CheckedAt
			if methodStatus.Status != "ok" {
				status.Status = "error"
				status.Error = methodStatus.Error
			}
		}
	}
	status.Methods = methods
	status.Summary = summarizeBitrix24Methods(methods)
	if status.CheckedAt.IsZero() && len(methods) > 0 {
		status.CheckedAt = methods[0].CheckedAt
	}
	if status.ResponseMS <= 0 && status.Summary != nil {
		status.ResponseMS = status.Summary.AvgResponseMS
	}
	if status.Error == "" {
		status.Status = "ok"
	}
	return status
}

func checkBitrix24Profile(client *http.Client, config bitrix24LocalConfig, spec bitrix24ProfileSpec) agentBitrix24MethodStatus {
	started := time.Now()
	out := agentBitrix24MethodStatus{
		Key:       spec.Key,
		Profile:   spec.Key,
		Name:      spec.Name,
		Method:    spec.Method,
		Status:    "error",
		CheckedAt: started.UTC(),
	}
	payload := spec.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	requestBody, err := json.Marshal(payload)
	if err != nil {
		out.Error = safeBitrix24Error(err.Error(), &config)
		return out
	}
	req, err := http.NewRequest(http.MethodPost, bitrix24MethodURL(config.WebhookURL, spec.Method), bytes.NewReader(requestBody))
	if err != nil {
		out.Error = safeBitrix24Error(err.Error(), &config)
		return out
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	out.ResponseMS = time.Since(started).Milliseconds()
	if err != nil {
		out.Error = safeBitrix24Error(err.Error(), &config)
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		out.Status = bitrix24MethodStatusFromError(fmt.Sprintf("http_%d", resp.StatusCode))
		out.RateLimited = resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests
		out.Error = safeBitrix24Error(fmt.Sprintf("http %d", resp.StatusCode), &config)
		return out
	}
	var body struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Result           any    `json:"result"`
		Total            any    `json:"total"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		out.Error = safeBitrix24Error("invalid json response", &config)
		return out
	}
	if strings.TrimSpace(body.Error) != "" {
		message := body.Error
		if desc := strings.TrimSpace(body.ErrorDescription); desc != "" {
			message += ": " + desc
		}
		out.Status = bitrix24MethodStatusFromError(body.Error)
		out.RateLimited = out.Status == "rate_limited"
		out.Error = safeBitrix24Error(message, &config)
		return out
	}
	out.Status = "ok"
	if spec.ResultCount {
		out.ResultCount = bitrix24ResultCount(body.Total, body.Result)
	}
	return out
}

func summarizeBitrix24Methods(methods []agentBitrix24MethodStatus) *agentBitrix24Summary {
	if len(methods) == 0 {
		return nil
	}
	summary := &agentBitrix24Summary{MethodCount: len(methods)}
	var responseSum int64
	var responseCount int64
	for _, item := range methods {
		if item.Status == "ok" {
			summary.OKCount++
			if item.CheckedAt.After(summary.LastSuccessAt) {
				summary.LastSuccessAt = item.CheckedAt
			}
		} else {
			summary.ErrorCount++
		}
		if item.RateLimited || item.Status == "rate_limited" {
			summary.RateLimited = true
		}
		if item.ResponseMS > 0 {
			responseSum += item.ResponseMS
			responseCount++
			if item.ResponseMS > summary.MaxResponseMS {
				summary.MaxResponseMS = item.ResponseMS
				summary.SlowestMethod = item.Name
			}
		}
	}
	if responseCount > 0 {
		summary.AvgResponseMS = responseSum / responseCount
	}
	return summary
}

func bitrix24ResultCount(total any, result any) int64 {
	if value, ok := numericAnyToInt64(total); ok && value >= 0 {
		return value
	}
	switch typed := result.(type) {
	case []any:
		return int64(len(typed))
	case map[string]any:
		if value, ok := numericAnyToInt64(typed["total"]); ok && value >= 0 {
			return value
		}
		if items, ok := typed["items"].([]any); ok {
			return int64(len(items))
		}
	}
	return 0
}

func numericAnyToInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func bitrix24MethodStatusFromError(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(normalized, "query_limit") || strings.Contains(normalized, "too_many") || strings.Contains(normalized, "http_429") || strings.Contains(normalized, "http_503"):
		return "rate_limited"
	case strings.Contains(normalized, "insufficient") || strings.Contains(normalized, "scope") || strings.Contains(normalized, "access_denied") || strings.Contains(normalized, "forbidden"):
		return "no_rights"
	case strings.Contains(normalized, "method") && (strings.Contains(normalized, "not") || strings.Contains(normalized, "unknown")):
		return "unavailable"
	default:
		return "error"
	}
}

func normalizeBitrix24Profiles(raw string) ([]string, error) {
	catalog := bitrix24ProfileCatalog()
	values := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	if len(values) == 0 {
		values = defaultBitrix24ProfileKeys()
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		key = strings.ReplaceAll(key, "-", "_")
		if key == "" {
			continue
		}
		if _, ok := catalog[key]; !ok {
			return nil, fmt.Errorf("unknown Bitrix24 profile %q", value)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return defaultBitrix24ProfileKeys(), nil
	}
	return out, nil
}

func bitrix24SpecsForProfiles(keys []string) []bitrix24ProfileSpec {
	catalog := bitrix24ProfileCatalog()
	out := make([]bitrix24ProfileSpec, 0, len(keys))
	for _, key := range keys {
		if spec, ok := catalog[strings.TrimSpace(key)]; ok {
			out = append(out, spec)
		}
	}
	return out
}

func normalizeBitrix24BaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("base-url must be a full HTTPS URL, for example https://crm.example.kz")
	}
	if parsed.Scheme != "https" {
		return "", errors.New("base-url must use HTTPS")
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeBitrix24WebhookURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("webhook URL must be a full HTTPS URL")
	}
	if parsed.Scheme != "https" {
		return "", errors.New("webhook URL must use HTTPS")
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 3 || segments[0] != "rest" {
		return "", errors.New("webhook URL must look like https://portal/rest/{user_id}/{webhook}/")
	}
	if strings.Contains(segments[len(segments)-1], ".") && len(segments) >= 4 {
		segments = segments[:len(segments)-1]
	}
	if len(segments) < 3 || strings.TrimSpace(segments[1]) == "" || strings.TrimSpace(segments[2]) == "" {
		return "", errors.New("webhook URL must include user id and webhook code")
	}
	parsed.Path = "/" + strings.Join(segments[:3], "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateBitrix24WebhookHost(baseURL, webhookURL string) error {
	base, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	webhook, err := url.Parse(webhookURL)
	if err != nil {
		return err
	}
	if !strings.EqualFold(base.Hostname(), webhook.Hostname()) {
		return errors.New("base-url and webhook URL must belong to the same Bitrix24 host")
	}
	return nil
}

func bitrix24MethodURL(webhookURL, method string) string {
	return strings.TrimRight(webhookURL, "/") + "/" + strings.TrimLeft(method, "/")
}

func safeBitrix24Error(message string, config *bitrix24LocalConfig) string {
	value := strings.TrimSpace(message)
	if config != nil {
		value = strings.ReplaceAll(value, strings.TrimSpace(config.WebhookURL), "[redacted webhook]")
		if parsed, err := url.Parse(config.WebhookURL); err == nil {
			segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(segments) >= 3 {
				value = strings.ReplaceAll(value, segments[2], "[redacted]")
			}
		}
	}
	value = sensitiveBitrix24ParamPattern.ReplaceAllString(value, "$1=[redacted]")
	if len(value) > 180 {
		value = value[:180] + "..."
	}
	return value
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
	if value, ok := linuxEnvFileValue(key); ok && value != "" {
		return value
	}
	return fallback
}

func linuxEnvFileHasEmpty(key string) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	value, ok := linuxEnvFileValue(key)
	return ok && strings.TrimSpace(value) == ""
}

func linuxEnvFileValue(key string) (string, bool) {
	if runtime.GOOS != "linux" || strings.TrimSpace(key) == "" {
		return "", false
	}
	body, err := os.ReadFile(linuxEnvPath)
	if err != nil {
		return "", false
	}
	prefix := key + "="
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		return value, true
	}
	return "", false
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
