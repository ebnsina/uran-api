package api

import (
	"net/http"

	"github.com/ebnsina/uran-api/internal/store"
)

const usageSampleLimit = 200

// usageWindowSeconds is the assumed seconds each sample represents (matches the
// controller's sampling interval) for the cpu-core-seconds rollup.
const usageWindowSeconds = 60

type usageResponse struct {
	Samples       []store.UsageSample `json:"samples"`
	SampleCount   int                 `json:"sample_count"`
	WindowSeconds int                 `json:"window_seconds"`
	CPUCoreSec    float64             `json:"cpu_core_seconds"`
	AvgMemoryMB   int64               `json:"avg_memory_mb"`
}

// handleServiceUsage returns recent usage samples and a rollup for billing.
func (s *Server) handleServiceUsage(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	samples, err := s.store.ListUsageSamples(r.Context(), svc.ID, usageSampleLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load usage")
		return
	}
	resp := usageResponse{Samples: samples, SampleCount: len(samples), WindowSeconds: len(samples) * usageWindowSeconds}
	var totalMem int64
	for _, u := range samples {
		resp.CPUCoreSec += float64(u.CPUMillicores) / 1000 * usageWindowSeconds
		totalMem += u.MemoryBytes
	}
	if len(samples) > 0 {
		resp.AvgMemoryMB = totalMem / int64(len(samples)) / (1024 * 1024)
	}
	writeJSON(w, http.StatusOK, resp)
}
