package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/te2wow/koto/internal/config"
	"github.com/te2wow/koto/internal/engine"
	"github.com/te2wow/koto/internal/provider"
	"github.com/te2wow/koto/internal/runlog"
	"github.com/te2wow/koto/internal/workflow"
)

// startRunReq is the request body for POST /api/run.
type startRunReq struct {
	Workflow  string            `json:"workflow"`
	Task      string            `json:"task"`
	Provider  string            `json:"provider"`            // overrides config
	Model     string            `json:"model"`               // overrides config
	Vars      map[string]string `json:"vars"`                // workflow var overrides
	NoIsolate bool              `json:"noIsolate,omitempty"` // mirror --no-isolate
	DryRun    bool              `json:"dryRun,omitempty"`
}

// handleStartRun spawns a workflow run in the background and returns its id.
func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 256*1024))
	var req startRunReq
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "expected JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Task) == "" {
		http.Error(w, "task is required", http.StatusBadRequest)
		return
	}
	if req.Workflow == "" {
		req.Workflow = "default"
	}

	wf, _, err := workflow.Resolve(req.Workflow)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if wf.Vars == nil {
		wf.Vars = map[string]string{}
	}
	for k, v := range req.Vars {
		wf.Vars[k] = v
	}

	cfg, _ := config.Load()
	if req.Provider != "" {
		cfg.Provider = req.Provider
	}
	if req.Model != "" {
		cfg.Model = req.Model
	}
	prov, err := provider.Get(cfg.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logger, err := runlog.New(s.rootD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runID := logger.ID

	// Spawn the run in the background; the dashboard tracks it via SSE on the run.
	go func() {
		defer func() { _ = logger.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		eng := &engine.Engine{
			WF:       wf,
			Provider: prov,
			Model:    cfg.Model,
			WorkDir:  s.rootD,
			Reporter: nil, // dashboard tails events.jsonl directly
			Log:      logger,
			DryRun:   req.DryRun,
			Isolate:  !req.NoIsolate,
		}
		_, _ = eng.Run(ctx, req.Task)
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"id": runID, "path": logger.Dir})
}

// Stats is the response shape for GET /api/stats.
type Stats struct {
	Total       int            `json:"total"`
	Completed   int            `json:"completed"`
	Aborted     int            `json:"aborted"`
	MaxSteps    int            `json:"maxSteps"`
	Running     int            `json:"running"`
	AvgSteps    float64        `json:"avgSteps"`
	GateAttempt map[string]int `json:"gateAttempts"` // step → total attempts across runs
	ByDay       []DayCount     `json:"byDay"`        // last 14 days
}

// DayCount holds per-day counts for the trend chart.
type DayCount struct {
	Date string `json:"date"`
	Runs int    `json:"runs"`
	OK   int    `json:"ok"`
}

// handleStats walks all runs and returns aggregate metrics for the dashboard.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := runlog.RunsBaseDir(s.rootD)
	entries, _ := os.ReadDir(base)

	stats := Stats{GateAttempt: map[string]int{}}
	dayMap := map[string]*DayCount{}
	stepsTotal := 0
	stepsCounted := 0

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sum := summarize(filepath.Join(base, e.Name()))
		stats.Total++
		switch sum.Outcome {
		case "complete":
			stats.Completed++
		case "abort":
			stats.Aborted++
		case "maxsteps":
			stats.MaxSteps++
		default:
			stats.Running++
		}
		if sum.Steps > 0 {
			stepsTotal += sum.Steps
			stepsCounted++
		}
		// gate attempts: read events to find gate_result entries
		evs, _ := readEvents(filepath.Join(base, e.Name(), "events.jsonl"))
		for _, ev := range evs {
			if ev["type"] == "gate_result" {
				step, _ := ev["step"].(string)
				if step != "" {
					stats.GateAttempt[step]++
				}
			}
		}
		// per-day bucket (use the ID prefix YYYYMMDD as the day)
		if len(sum.ID) >= 8 {
			day := sum.ID[:4] + "-" + sum.ID[4:6] + "-" + sum.ID[6:8]
			d, ok := dayMap[day]
			if !ok {
				d = &DayCount{Date: day}
				dayMap[day] = d
			}
			d.Runs++
			if sum.Outcome == "complete" {
				d.OK++
			}
		}
	}
	if stepsCounted > 0 {
		stats.AvgSteps = float64(stepsTotal) / float64(stepsCounted)
	}
	// Keep the latest 14 days, oldest first.
	days := make([]DayCount, 0, len(dayMap))
	for _, d := range dayMap {
		days = append(days, *d)
	}
	sortByDate(days)
	if len(days) > 14 {
		days = days[len(days)-14:]
	}
	stats.ByDay = days
	writeJSON(w, http.StatusOK, stats)
}

// sortByDate sorts day buckets ascending by Date (string sort works for ISO).
func sortByDate(d []DayCount) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j-1].Date > d[j].Date; j-- {
			d[j-1], d[j] = d[j], d[j-1]
		}
	}
}
