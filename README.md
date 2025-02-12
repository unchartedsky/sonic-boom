# Sonic Boom

![build](https://github.com/unchartedsky/sonic-boom/workflows/build/badge.svg)

```bash
./run.sh
docker-compose logs -f
```

## Config 주요 설정 예시

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
        pool-size: 10               # 기본값 10.
        max-retries: 5              # 재시도. 기본값 3. 끄고 싶을 경우, -1
        db-number: 0                # 기본값 0
        min-retry-backoff-ms: 8     # 단위는 ms
        max-retry-backoff-ms: 512   # 단위는 ms
        dial-timeout: 5             # 단위는 초(s)
        read-timeout: 3             # 단위는 초(s)
        write-timeout: 3            # 단위는 초(s)
        pool-timeout: 5             # 단위는 초(s)
        idle-timeout: 1             # 단위는 초(s)
...
```
