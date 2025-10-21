# Testing Guide

이 문서는 sonic-boom 프로젝트의 테스트 전략과 구조를 설명합니다.

## 테스트 구조

### 1. 단위 테스트 (Unit Tests)

#### In-Memory 캐시 테스트 (`internal/sonic_boom_inmemory_test.go`)
- **목적**: Ristretto 기반 in-memory 캐시의 핵심 기능 검증
- **주요 테스트**:
  - `Test_InMemory_PutGet_RoundTrip`: 기본 put/get 동작
  - `Test_InMemory_TTL_Expires`: TTL 만료 검증
  - `Test_InMemory_Concurrency_NoRace`: 동시성 안전성
  - `Test_InMemory_Eviction_MinimalGuarantee`: 캐시 제거 정책
  - `Test_InMemory_Marshal_Unmarshal_CacheValue`: 직렬화/역직렬화
  - `Test_InMemory_LoadOrStore_ReusesInstance`: 인스턴스 재사용
  - `Test_InMemory_DifferentConfig_IsolatedInstances`: 설정별 격리

#### Redis 캐시 테스트 (`internal/sonic_boom_redis_test.go`)
- **목적**: Redis 기반 캐시의 네트워크 통신 및 TTL 검증
- **주요 테스트**:
  - `Test_Redis_PutGet_RoundTrip`: Redis put/get 동작
  - `Test_Redis_TTL_Expires`: Redis TTL 만료
  - `Test_Redis_Concurrency_NoRace`: Redis 동시성
  - `Test_Redis_Marshal_Unmarshal_CacheValue`: Redis 직렬화
  - `Test_Redis_Connection_Error`: 연결 실패 처리

#### Redis Cluster 테스트 (`internal/sonic_boom_rediscluster_test.go`)
- **목적**: Redis Cluster 기반 캐시의 분산 처리 검증
- **주요 테스트**:
  - `Test_RedisCluster_PutGet_RoundTrip`: Cluster put/get 동작
  - `Test_RedisCluster_TTL_Expires`: Cluster TTL 만료
  - `Test_RedisCluster_Concurrency_NoRace`: Cluster 동시성
  - `Test_RedisCluster_Marshal_Unmarshal_CacheValue`: Cluster 직렬화
  - `Test_RedisCluster_Connection_Error`: Cluster 연결 실패 처리

### 2. 스모크 테스트 (Smoke Tests)

#### In-Memory 스모크 테스트 (`scripts/smoke_in_memory.sh`)
- **목적**: Kong Gateway + in-memory 캐시의 엔드투엔드 검증
- **검증 항목**: `X-Cache-Status` 헤더를 통한 캐시 히트/미스 확인

#### Redis 스모크 테스트 (`scripts/smoke_redis.sh`)
- **목적**: Kong Gateway + Redis 캐시의 엔드투엔드 검증
- **검증 항목**: Redis 기반 캐시 히트/미스 동작

#### Redis Cluster 스모크 테스트 (`scripts/smoke_rediscluster.sh`)
- **목적**: Kong Gateway + Redis Cluster 캐시의 엔드투엔드 검증
- **검증 항목**: Redis Cluster 기반 캐시 히트/미스 동작

## 테스트 실행 방법

### 로컬 테스트 실행

```bash
# 전체 테스트 실행 (race condition 검사 포함)
go test ./... -race -count=1

# 특정 캐시 전략 테스트
go test ./internal -run Test_InMemory -v
go test ./internal -run Test_Redis -v
go test ./internal -run Test_RedisCluster -v

# 커버리지 확인
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

### 스모크 테스트 실행

```bash
# Docker Compose로 서비스 시작
docker-compose up -d

# In-Memory 스모크 테스트
./scripts/smoke_in_memory.sh

# Redis 스모크 테스트 (Redis 서버 필요)
./scripts/smoke_redis.sh

# Redis Cluster 스모크 테스트 (Redis Cluster 필요)
./scripts/smoke_rediscluster.sh

# 서비스 정리
docker-compose down
```

## Ristretto 비동기 쓰기 특성 및 대응

### 문제점
Ristretto는 성능 최적화를 위해 비동기 쓰기를 사용합니다. 이로 인해:
- `Set()` 호출 후 즉시 `Get()` 호출 시 miss 발생 가능
- 테스트에서 flaky behavior 발생

### 해결 방법

#### 1. 리트라이 로직 적용
```go
for i := 0; i < 10; i++ {
    got, err := cm.Get(context.Background(), key)
    if err == nil {
        break
    }
    time.Sleep(20 * time.Millisecond)
}
```

#### 2. 충분한 대기 시간 확보
```go
require.NoError(t, cm.Set(context.Background(), key, val))
time.Sleep(100 * time.Millisecond) // 비동기 쓰기 완료 대기
got, err := cm.Get(context.Background(), key)
```

#### 3. TTL 테스트 시 추가 대기
```go
require.NoError(t, cm.Set(context.Background(), key, val))
time.Sleep(200 * time.Millisecond) // 쓰기 확정 대기
time.Sleep(1500 * time.Millisecond) // TTL 만료 대기
```

## CI/CD 통합

### GitHub Actions 워크플로우

#### 테스트 단계
- Go 1.25 환경 설정
- 의존성 캐싱
- Race condition 검사
- 커버리지 측정

#### 스모크 테스트 단계
- Docker Compose로 Kong + Redis 환경 구성
- In-memory 스모크 테스트 실행
- 자동 정리

### 로컬 개발 환경

#### 필수 요구사항
- Go 1.25+
- Docker & Docker Compose
- Redis (스모크 테스트용)

#### 권장 설정
```bash
# Go 모듈 캐시 설정
export GOMODCACHE=$HOME/.cache/go-mod

# Docker Compose 오버라이드 (개발용)
cp docker-compose.yaml docker-compose.override.yaml
```

## 테스트 작성 가이드

### 새로운 캐시 전략 테스트 추가

1. **테스트 파일 생성**: `internal/sonic_boom_{strategy}_test.go`
2. **기본 테스트 패턴**:
   - `Test_newCacheManager_{Strategy}_Success`
   - `Test_{Strategy}_PutGet_RoundTrip`
   - `Test_{Strategy}_TTL_Expires`
   - `Test_{Strategy}_Concurrency_NoRace`
   - `Test_{Strategy}_Marshal_Unmarshal_CacheValue`
   - `Test_{Strategy}_Connection_Error`

3. **스모크 테스트 스크립트**: `scripts/smoke_{strategy}.sh`

### 테스트 작성 시 주의사항

1. **비동기 동작 고려**: Ristretto, Redis 등 비동기 특성 반영
2. **네트워크 지연 고려**: Redis/Cluster 테스트 시 충분한 대기 시간
3. **리소스 정리**: 테스트 후 연결 정리
4. **에러 케이스**: 연결 실패, 타임아웃 등 예외 상황 테스트

## 성능 테스트

### 벤치마크 실행
```bash
# 전체 벤치마크
go test -bench=. ./internal

# 특정 벤치마크
go test -bench=Benchmark_InMemory ./internal
go test -bench=Benchmark_Redis ./internal
```

### 벤치마크 분석
- 메모리 사용량: `-benchmem` 플래그 사용
- 프로파일링: `go test -cpuprofile=cpu.prof`
- 경쟁 상태: `go test -race -bench=.`

## 문제 해결

### 자주 발생하는 문제

1. **테스트 실패 (Flaky Tests)**
   - 원인: 비동기 쓰기, 네트워크 지연
   - 해결: 리트라이 로직, 충분한 대기 시간

2. **연결 실패**
   - 원인: Redis 서버 미실행, 잘못된 설정
   - 해결: Docker Compose 확인, 설정 검증

3. **메모리 누수**
   - 원인: 연결 미정리, 캐시 미정리
   - 해결: `defer` 구문으로 정리, 테스트 후 정리

### 디버깅 팁

```bash
# 상세 로그와 함께 테스트
go test -v -race ./internal

# 특정 테스트만 실행
go test -run Test_InMemory_PutGet_RoundTrip -v ./internal

# 프로파일링
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./internal
```
