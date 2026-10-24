package internal

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// PE5: span context DTO 라운드트립과 복원 실패 시 계속 진행 시맨틱을 검증한다.

func Test_SpanContextDTO_RoundTrip(t *testing.T) {
	ts, err := trace.ParseTraceState("vendor=1")
	require.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03},
		SpanID:     trace.SpanID{0x04, 0x05},
		TraceFlags: trace.FlagsSampled,
		TraceState: ts,
	})

	payload, err := json.Marshal(newSpanContextDTO(sc))
	require.NoError(t, err)

	var dto spanContextDTO
	require.NoError(t, json.Unmarshal(payload, &dto))

	got, err := spanContextFromDTO(dto)
	require.NoError(t, err)
	require.True(t, got.IsValid())
	require.Equal(t, sc.TraceID(), got.TraceID())
	require.Equal(t, sc.SpanID(), got.SpanID())
	require.Equal(t, sc.TraceFlags(), got.TraceFlags())
	require.Equal(t, sc.TraceState().String(), got.TraceState().String())
}

func Test_SpanContextDTO_RestoreFailureDoesNotAbort(t *testing.T) {
	// 깨진 페이로드는 디코드 단계에서 오류가 나고(호출자가 로그 후 계속),
	// 파싱 불가 ID는 무효 컨텍스트+오류를 반환해 부모 없이 계속 가능하다.
	var dto spanContextDTO
	require.Error(t, json.Unmarshal([]byte("not-json"), &dto))

	got, err := spanContextFromDTO(spanContextDTO{TraceID: "zz", SpanID: "yy"})
	require.Error(t, err)
	require.False(t, got.IsValid())

	empty, err := spanContextFromDTO(spanContextDTO{})
	require.Error(t, err)
	require.False(t, empty.IsValid())
}
