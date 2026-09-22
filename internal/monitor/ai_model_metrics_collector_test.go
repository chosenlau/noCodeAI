package monitor

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestAiModelMetricsCollector_RecordsAllMetricTypes(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewAiModelMetricsCollector(registry)

	collector.RecordRequest("user-1", "app-1", "test-model", "success")
	collector.RecordError("user-1", "app-1", "test-model", "test error")
	collector.RecordTokenUsage("user-1", "app-1", "test-model", "total", 12)
	collector.RecordResponseTime("user-1", "app-1", "test-model", time.Millisecond)
	finish := collector.RecordResponseTimeStart("user-1", "app-1", "test-model")
	finish()

	families, err := registry.Gather()
	require.NoError(t, err)

	names := make(map[string]bool, len(families))
	for _, family := range families {
		names[family.GetName()] = true
	}
	require.True(t, names["ai_model_requests_total"])
	require.True(t, names["ai_model_errors_total"])
	require.True(t, names["ai_model_tokens_total"])
	require.True(t, names["ai_model_response_duration_seconds"])
}
