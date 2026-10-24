package internal

import (
	"encoding/hex"
	"fmt"

	"go.opentelemetry.io/otel/trace"
)

// spanContextDTO는 Access에서 Response로 trace.SpanContext를 kong.ctx.shared로
// 전달하기 위한 직렬화 형식이다. SetPluginEx는 JSON 마샬을 하므로 SpanContext를
// 그대로 저장하면 복원 시 string이 돌아와 타입 단언이 항상 실패했다(PE5).
type spanContextDTO struct {
	TraceID    string
	SpanID     string
	Flags      int
	TraceState string
	Remote     bool
}

func newSpanContextDTO(sc trace.SpanContext) spanContextDTO {
	return spanContextDTO{
		TraceID:    sc.TraceID().String(),
		SpanID:     sc.SpanID().String(),
		Flags:      int(sc.TraceFlags()),
		TraceState: sc.TraceState().String(),
		Remote:     sc.IsRemote(),
	}
}

// spanContextFromDTO는 DTO를 부모 SpanContext로 복원한다. TraceID·SpanID 파싱
// 실패는 오류와 무효 컨텍스트를 반환하고, TraceState 파싱 실패는 빈 값으로
// 무시한다 — 호출자는 부모 없이 저장 경로를 계속한다.
func spanContextFromDTO(dto spanContextDTO) (trace.SpanContext, error) {
	tid, err := parseTraceID(dto.TraceID)
	if err != nil {
		return trace.SpanContext{}, err
	}
	sid, err := parseSpanID(dto.SpanID)
	if err != nil {
		return trace.SpanContext{}, err
	}
	ts, _ := trace.ParseTraceState(dto.TraceState)

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.TraceFlags(dto.Flags),
		TraceState: ts,
		Remote:     dto.Remote,
	}), nil
}

// parseTraceID·parseSpanID는 otel trace 패키지에 공개 파서가 없어 hex 디코드로
// 같은 일을 한다(16·32자 십육진).
func parseTraceID(s string) (trace.TraceID, error) {
	var tid trace.TraceID
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(tid) {
		return trace.TraceID{}, fmt.Errorf("invalid trace id %q", s)
	}
	copy(tid[:], b)
	return tid, nil
}

func parseSpanID(s string) (trace.SpanID, error) {
	var sid trace.SpanID
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(sid) {
		return trace.SpanID{}, fmt.Errorf("invalid span id %q", s)
	}
	copy(sid[:], b)
	return sid, nil
}
