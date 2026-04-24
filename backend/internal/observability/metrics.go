package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var DefaultMetrics = NewMetrics()

type Metrics struct {
	mu sync.Mutex

	requests         map[string]int64
	requestDurations map[string]durationStats
	analysis         map[string]int64
	analysisDuration map[string]durationStats
	kimi             map[string]durationStats
	ocr              map[string]durationStats
	rateLimited      map[string]int64
	saves            map[string]int64
	deletes          map[string]int64
}

type durationStats struct {
	Count   int64
	Errors  int64
	Timeout int64
	Sum     float64
	Max     float64
}

func NewMetrics() *Metrics {
	return &Metrics{
		requests:         make(map[string]int64),
		requestDurations: make(map[string]durationStats),
		analysis:         make(map[string]int64),
		analysisDuration: make(map[string]durationStats),
		kimi:             make(map[string]durationStats),
		ocr:              make(map[string]durationStats),
		rateLimited:      make(map[string]int64),
		saves:            make(map[string]int64),
		deletes:          make(map[string]int64),
	}
}

func (m *Metrics) RecordRequest(method, path string, status int, duration time.Duration) {
	route := normalizePath(path)
	requestKey := labels("method", method, "route", route, "status", fmt.Sprintf("%d", status))
	durationKey := labels("method", method, "route", route)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[requestKey]++
	m.requestDurations[durationKey] = addDuration(m.requestDurations[durationKey], duration, false, false)
}

func (m *Metrics) RecordAnalysis(inputType string, saved bool, err error, duration time.Duration) {
	result := resultLabel(err)
	key := labels("input_type", inputType, "saved", fmt.Sprintf("%t", saved), "result", result)
	durationKey := labels("input_type", inputType)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.analysis[key]++
	m.analysisDuration[durationKey] = addDuration(m.analysisDuration[durationKey], duration, err != nil, isTimeout(err))
}

func (m *Metrics) RecordKimi(operation string, err error, duration time.Duration) {
	key := labels("operation", operation)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.kimi[key] = addDuration(m.kimi[key], duration, err != nil, isTimeout(err))
}

func (m *Metrics) RecordOCR(err error, duration time.Duration) {
	key := labels("operation", "extract_text")

	m.mu.Lock()
	defer m.mu.Unlock()
	m.ocr[key] = addDuration(m.ocr[key], duration, err != nil, isTimeout(err))
}

func (m *Metrics) RecordRateLimited() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimited[labels("scope", "user")]++
}

func (m *Metrics) RecordSave(inputType string, err error) {
	key := labels("input_type", inputType, "result", resultLabel(err))

	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves[key]++
}

func (m *Metrics) RecordDelete(hadImage bool, err error) {
	key := labels("had_image", fmt.Sprintf("%t", hadImage), "result", resultLabel(err))

	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletes[key]++
}

func (m *Metrics) Render(dbUp bool) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var b strings.Builder
	writeGauge(&b, "senti_database_up", "Database ping status from backend metrics scrape.", boolFloat(dbUp), "")
	writeCounters(&b, "senti_http_requests_total", "HTTP requests by method, route, and status.", m.requests)
	writeDurationStats(&b, "senti_http_request", "HTTP request duration stats.", m.requestDurations)
	writeCounters(&b, "senti_analysis_total", "Analysis attempts by input type, save state, and result.", m.analysis)
	writeDurationStats(&b, "senti_analysis", "Analysis pipeline duration stats.", m.analysisDuration)
	writeDurationStats(&b, "senti_kimi_request", "Kimi call duration stats.", m.kimi)
	writeDurationStats(&b, "senti_ocr_request", "OCR call duration stats.", m.ocr)
	writeCounters(&b, "senti_rate_limited_total", "Rate limited requests.", m.rateLimited)
	writeCounters(&b, "senti_analysis_saves_total", "Analysis save attempts.", m.saves)
	writeCounters(&b, "senti_analysis_deletes_total", "Analysis delete attempts.", m.deletes)
	return b.String()
}

func addDuration(stats durationStats, duration time.Duration, failed bool, timeout bool) durationStats {
	seconds := duration.Seconds()
	stats.Count++
	stats.Sum += seconds
	if seconds > stats.Max {
		stats.Max = seconds
	}
	if failed {
		stats.Errors++
	}
	if timeout {
		stats.Timeout++
	}
	return stats
}

func writeCounters(b *strings.Builder, name, help string, values map[string]int64) {
	b.WriteString("# HELP " + name + " " + help + "\n")
	b.WriteString("# TYPE " + name + " counter\n")
	for _, key := range sortedKeys(values) {
		fmt.Fprintf(b, "%s{%s} %d\n", name, key, values[key])
	}
}

func writeDurationStats(b *strings.Builder, prefix, help string, values map[string]durationStats) {
	fmt.Fprintf(b, "# HELP %s_duration_seconds_sum %s\n", prefix, help)
	fmt.Fprintf(b, "# TYPE %s_duration_seconds_sum counter\n", prefix)
	for _, key := range sortedKeys(values) {
		fmt.Fprintf(b, "%s_duration_seconds_sum{%s} %.6f\n", prefix, key, values[key].Sum)
	}
	fmt.Fprintf(b, "# HELP %s_duration_seconds_count %s\n", prefix, help)
	fmt.Fprintf(b, "# TYPE %s_duration_seconds_count counter\n", prefix)
	for _, key := range sortedKeys(values) {
		fmt.Fprintf(b, "%s_duration_seconds_count{%s} %d\n", prefix, key, values[key].Count)
	}
	fmt.Fprintf(b, "# HELP %s_errors_total %s\n", prefix, help)
	fmt.Fprintf(b, "# TYPE %s_errors_total counter\n", prefix)
	for _, key := range sortedKeys(values) {
		fmt.Fprintf(b, "%s_errors_total{%s} %d\n", prefix, key, values[key].Errors)
	}
	fmt.Fprintf(b, "# HELP %s_timeouts_total %s\n", prefix, help)
	fmt.Fprintf(b, "# TYPE %s_timeouts_total counter\n", prefix)
	for _, key := range sortedKeys(values) {
		fmt.Fprintf(b, "%s_timeouts_total{%s} %d\n", prefix, key, values[key].Timeout)
	}
	fmt.Fprintf(b, "# HELP %s_duration_seconds_max %s\n", prefix, help)
	fmt.Fprintf(b, "# TYPE %s_duration_seconds_max gauge\n", prefix)
	for _, key := range sortedKeys(values) {
		fmt.Fprintf(b, "%s_duration_seconds_max{%s} %.6f\n", prefix, key, values[key].Max)
	}
}

func writeGauge(b *strings.Builder, name, help string, value float64, labelSet string) {
	b.WriteString("# HELP " + name + " " + help + "\n")
	b.WriteString("# TYPE " + name + " gauge\n")
	if labelSet == "" {
		fmt.Fprintf(b, "%s %.0f\n", name, value)
		return
	}
	fmt.Fprintf(b, "%s{%s} %.0f\n", name, labelSet, value)
}

func labels(values ...string) string {
	pairs := make([]string, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		pairs = append(pairs, fmt.Sprintf(`%s="%s"`, values[i], escapeLabel(values[i+1])))
	}
	return strings.Join(pairs, ",")
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizePath(path string) string {
	if strings.HasPrefix(path, "/api/history/") {
		return "/api/history/:id"
	}
	return path
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func resultLabel(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "deadline exceeded") || strings.Contains(text, "Client.Timeout") || strings.Contains(text, "timeout")
}
