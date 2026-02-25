package observability

// 文件： internal/observability/metrics.go
// 用途：运行期监控指标与可观测性基础组件。

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Registry struct {
	httpRequests sync.Map
	httpLatency  sync.Map

	aiRequestsTotal int64
	aiFailuresTotal int64
}

type latencyCounter struct {
	sumMillis int64
	count     int64
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) ObserveHTTP(method, route string, status int, latencyMillis int64) {
	key := normalizeLabel(method) + "|" + normalizeLabel(route) + "|" + fmt.Sprintf("%d", status)
	var reqCounter atomic.Int64
	actualReq, _ := r.httpRequests.LoadOrStore(key, &reqCounter)
	actualReq.(*atomic.Int64).Add(1)

	var lat latencyCounter
	actualLat, _ := r.httpLatency.LoadOrStore(key, &lat)
	entry := actualLat.(*latencyCounter)
	atomic.AddInt64(&entry.sumMillis, latencyMillis)
	atomic.AddInt64(&entry.count, 1)
}

func (r *Registry) IncAIRequest() {
	atomic.AddInt64(&r.aiRequestsTotal, 1)
}

func (r *Registry) IncAIFailure() {
	atomic.AddInt64(&r.aiFailuresTotal, 1)
}

func (r *Registry) PrometheusText() string {
	var b strings.Builder

	b.WriteString("# TYPE app_http_requests_total counter\n")
	for _, k := range sortedKeys(&r.httpRequests) {
		counter := mustLoadAtomic(&r.httpRequests, k)
		method, route, status := splitLabels(k)
		fmt.Fprintf(&b, "app_http_requests_total{method=%q,route=%q,status=%q} %d\n", method, route, status, counter.Load())
	}

	b.WriteString("# TYPE app_http_request_duration_millis_sum counter\n")
	for _, k := range sortedKeys(&r.httpLatency) {
		lat := mustLoadLatency(&r.httpLatency, k)
		method, route, status := splitLabels(k)
		fmt.Fprintf(&b, "app_http_request_duration_millis_sum{method=%q,route=%q,status=%q} %d\n", method, route, status, atomic.LoadInt64(&lat.sumMillis))
	}

	b.WriteString("# TYPE app_http_request_duration_millis_count counter\n")
	for _, k := range sortedKeys(&r.httpLatency) {
		lat := mustLoadLatency(&r.httpLatency, k)
		method, route, status := splitLabels(k)
		fmt.Fprintf(&b, "app_http_request_duration_millis_count{method=%q,route=%q,status=%q} %d\n", method, route, status, atomic.LoadInt64(&lat.count))
	}

	b.WriteString("# TYPE app_ai_requests_total counter\n")
	fmt.Fprintf(&b, "app_ai_requests_total %d\n", atomic.LoadInt64(&r.aiRequestsTotal))
	b.WriteString("# TYPE app_ai_failures_total counter\n")
	fmt.Fprintf(&b, "app_ai_failures_total %d\n", atomic.LoadInt64(&r.aiFailuresTotal))

	return b.String()
}

func normalizeLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}

func sortedKeys(m *sync.Map) []string {
	keys := make([]string, 0, 32)
	m.Range(func(key, _ any) bool {
		if k, ok := key.(string); ok {
			keys = append(keys, k)
		}
		return true
	})
	sort.Strings(keys)
	return keys
}

func splitLabels(key string) (string, string, string) {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 {
		return "unknown", "unknown", "0"
	}
	return parts[0], parts[1], parts[2]
}

func mustLoadAtomic(m *sync.Map, key string) *atomic.Int64 {
	if v, ok := m.Load(key); ok {
		return v.(*atomic.Int64)
	}
	return &atomic.Int64{}
}

func mustLoadLatency(m *sync.Map, key string) *latencyCounter {
	if v, ok := m.Load(key); ok {
		return v.(*latencyCounter)
	}
	return &latencyCounter{}
}
