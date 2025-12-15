# Sonic Boom

![CI](https://github.com/unchartedsky/sonic-boom/workflows/CI/badge.svg)

Kong Gateway용 고성능 캐시 플러그인으로, in-memory, Redis, Redis Cluster 전략을 지원합니다.

## 빠른 시작

### 로컬 개발 환경

```bash
# 서비스 시작
./run.sh
docker-compose logs -f

# 테스트 실행
./test.sh

# 스모크 테스트
./scripts/smoke_in_memory.sh
```

### 빌드 및 설치

```bash
# 빌드
go build -o sonic-boom ./cmd

# Kong Gateway에 플러그인 설치
# (Kong 설정에 따라 다름)
```

## 캐시 전략

### In-Memory 캐시
- **용도**: 단일 인스턴스, 고성능 캐시
- **특징**: Ristretto 기반, 비동기 쓰기, TTL 지원
- **설정**: `strategy: in-memory`

### Redis 캐시
- **용도**: 분산 환경, 지속성 필요
- **특징**: 네트워크 기반, 클러스터 지원
- **설정**: `strategy: redis`

### Redis Cluster 캐시
- **용도**: 대규모 분산 환경
- **특징**: 자동 샤딩, 고가용성
- **설정**: `strategy: redis-cluster`

## 설정 예시

[config_v1.yml](./config_v1.yml), [config_v2.yml](./config_v2.yml)를 참고하십시오.

```yaml
...
config:
    response_code:                  # 캐시 고려할 HTTP 응답 코드
        - 200
        - 301
        - 404
    request_method:                 # 캐시 고려할 HTTP 요청 메소드
        - GET
        - HEAD
        - POST
        - PUT
    content_type:                   # 캐시 고려할 Content-type 응답 헤더
        - "text/plain"
        - "text/html; charset=utf-8"
        - "application/json"
        - "application/json; charset=utf-8"
    vary_headers:
        - Authorization
    cache_ttl: 15                   # 캐시할 엔티티의 TTL 값
    cacheable_body_max_size: 100000 # 캐시 고려할 최대 허용 Body 길이
    strategy: redis                 # 캐시 방식
    redis:
        host: redis                 # 접근할 Redis 호스트명. 기본값 localhost
        port: 6379                  # 접근할 Redis 포트번호. 기본값 6379
        pool_size: 10               # 기본값 10.
        max_retries: 5              # 재시도. 기본값 3. 끄고 싶을 경우, -1
        db_number: 0                # 기본값 0
        min_retry_backoff_ms: 8     # 단위는 ms
        max_retry_backoff_ms: 512   # 단위는 ms
        dial_timeout: 5             # 단위는 초(s)
        read_timeout: 3             # 단위는 초(s)
        write_timeout: 3            # 단위는 초(s)
        pool_timeout: 5             # 단위는 초(s)
        idle_timeout: 1             # 단위는 초(s)
...
```

## 테스트

### 단위 테스트
```bash
# 전체 테스트 (race condition 검사)
go test ./... -race -count=1

# 특정 캐시 전략 테스트
go test ./internal -run Test_InMemory -v
go test ./internal -run Test_Redis -v
go test ./internal -run Test_RedisCluster -v
```

### 스모크 테스트
```bash
# In-Memory 스모크 테스트
./scripts/smoke_in_memory.sh

# Redis 스모크 테스트
./scripts/smoke_redis.sh

# Redis Cluster 스모크 테스트
./scripts/smoke_rediscluster.sh
```

자세한 테스트 가이드는 [docs/testing.md](./docs/testing.md)를 참고하세요.

## 개발

### 필수 요구사항
- Go 1.25+
- Docker & Docker Compose
- Redis (스모크 테스트용)

### 개발 워크플로우
1. 코드 변경
2. 테스트 실행: `go test ./... -race`
3. 스모크 테스트: `./scripts/smoke_in_memory.sh`
4. 커밋 및 푸시

## TODO

- [x] `linux/arm64` 컨테이너 이미지 지원 ✅ 2025-02-17
- [x] 바이너리 릴리즈 ✅ 2025-02-17
- [x] in-memory 스토어 지원 ✅ 2025-02-17
- [x] Redis cluster 스토어 지원 ✅ 2025-02-19
- [x] OpenTelemetry 통합 ✅ 2025-02-19
- [x] 포괄적인 테스트 커버리지 ✅ 2025-01-21
- [x] CI/CD 파이프라인 ✅ 2025-01-21
- [ ] Kubernetes 예제 추가
- [ ] Kong proxycache 의 [`ignore_uri_case`](https://github.com/Kong/kong/blob/a4c0b461345d431067a2bfb7645434212eed7e5b/kong/plugins/proxy-cache/handler.lua#L247) 지원

```yaml

```

## 참고

- [Feature request plugin instance lifecycle hooks · Issue 78 · Konggo-pdk](https://github.com/Kong/go-pdk/issues/78)

## 유용한 도구

- [A hosted REST-API ready to respond to your AJAX requests](https://reqres.in/)
- [Mockbin by Zuplo](https://mockbin.io/)
- [dnnrlywait-for Super simple tool to help with orchestration of commands on the CLI by waiting on networking resources.](https://github.com/dnnrly/wait-for)
- [A hosted REST-API ready to respond to your AJAX requests](https://reqres.in/)
