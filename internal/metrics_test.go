package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// 캐시 메트릭 단언: 판정 카운터(strategy,status 라벨)와 조회 지연 히스토그램이
// OTel SDK에 기록된다. ManualReader로 in-process 수집 — 외부 수출 경로 불필요.
func TestCacheMetrics_RecordStatusAndLookup(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	t.Cleanup(func() {
		otel.SetMeterProvider(prev)
		_ = reader.Shutdown(context.Background())
	})

	recordCacheStatus("redis", "Miss")
	recordCacheStatus("redis", "Hit")
	recordCacheStatus("redis", "Hit")
	recordCacheStatus("in-memory", "Bypass")
	recordCacheLookup("redis", 3*time.Millisecond)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	counts := map[string]int64{}       // "카운터이름|strategy|status" -> 합
	histSamples := map[string]uint64{} // "히스토그램이름|strategy" -> 샘플 수
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range data.DataPoints {
					counts[m.Name+"|"+labelValue(dp.Attributes, "strategy")+"|"+labelValue(dp.Attributes, "status")] = dp.Value
				}
			case metricdata.Histogram[float64]:
				for _, dp := range data.DataPoints {
					histSamples[m.Name+"|"+labelValue(dp.Attributes, "strategy")] = dp.Count
				}
			}
		}
	}

	require.EqualValues(t, 1, counts["sonicboom_cache_requests|redis|Miss"], "Miss 1회")
	require.EqualValues(t, 2, counts["sonicboom_cache_requests|redis|Hit"], "Hit 2회")
	require.EqualValues(t, 1, counts["sonicboom_cache_requests|in-memory|Bypass"], "Bypass 1회")
	require.EqualValues(t, 1, histSamples["sonicboom_cache_lookup_duration|redis"], "lookup 샘플 1회")
}

func labelValue(attrs attribute.Set, key string) string {
	for _, kv := range attrs.ToSlice() {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return "<missing>"
}

// OTel 미구성(기본 no-op)에서는 기록 경로가 조용히 무시된다 — 패닉 없이.
func TestCacheMetrics_NoProviderIsNoop(t *testing.T) {
	recordCacheStatus("redis", "Miss")
	recordCacheLookup("redis", time.Millisecond)
}
