package internal

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// 캐시 판정 메트릭. 라벨은 strategy/status 두 개로 제한한다 — 요청량만큼
// 시계열이 늘지 않게(sdk/metric 기본 카디널리티 상한 2000).
//
// 계기는 매 기록 시점의 전역 프로바이더에서 해석한다(초기 1회 캐시하지
// 않는다). SDK가 동일 이름 계기를 내부 캐시해 2회째부터는 조회 비용만
// 남고, 테스트나 잦은 설정이 SetMeterProvider로 늦게 붙여도 수집된다.
// OTel 미구성(compose 기본, OTEL_SDK_DISABLED=true)이면 no-op 계기라
// Enabled()가 false — 기록 분기가 0비용 게이트가 된다(otel v1.40 안정화).
const (
	metricsScope      = "sonic-boom"
	cacheRequestsName = "sonicboom_cache_requests"
	cacheLookupName   = "sonicboom_cache_lookup_duration"
)

func recordCacheStatus(strategy, status string) {
	c, err := otel.Meter(metricsScope).Int64Counter(cacheRequestsName,
		metric.WithDescription("Cache decisions by strategy and status"))
	if err != nil || !c.Enabled(context.Background()) {
		return
	}
	c.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("strategy", strategy),
		attribute.String("status", status),
	))
}

func recordCacheLookup(strategy string, d time.Duration) {
	h, err := otel.Meter(metricsScope).Float64Histogram(cacheLookupName,
		metric.WithUnit("ms"),
		metric.WithDescription("Cache lookup duration"))
	if err != nil || !h.Enabled(context.Background()) {
		return
	}
	h.Record(context.Background(), float64(d.Nanoseconds())/1e6, metric.WithAttributes(
		attribute.String("strategy", strategy),
	))
}
