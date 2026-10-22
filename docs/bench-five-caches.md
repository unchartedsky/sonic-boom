# 캐시 플러그인 5종 벤치 — 우리 구현 3종 vs 게이트웨이 내장 캐시 2종

날짜: 2026-10-22 · 브랜치 `feat/apisix-lua-adapter`

## 무엇을 비교했나

| 라벨 | 게이트웨이 | 플러그인 | 저장소 |
|---|---|---|---|
| `kong:go` | Kong 3.9.4 (`:8000`) | sonic-boom Go(go-pdk external plugin) | ristretto(프로세스 메모리) |
| `kong:lua` | Kong 3.9.4 (`:8010`) | sonic-boom-lua | `lua_shared_dict` |
| `kong:proxy-cache` | Kong 3.9.4 (`:8010`) | Kong OSS 내장 `proxy-cache` | memory(`kong_db_cache` shdict) |
| `apisix:lua` | APISIX 3.19 (`:8020`) | sonic-boom-lua | `lua_shared_dict` |
| `apisix:proxy-cache` | APISIX 3.19 (`:8020`) | APISIX 내장 `proxy-cache` | memory(`memory_cache` 존) |

- 모두 **노드 로컬 메모리 저장**이다. 내장 캐시 둘은 redis 같은 공유 저장소가 없어서 이 조건으로 맞췄다.
- **HIT**: 캐시에서 바로 응답하는 경로.
- **MISS**: 저장되지 않는 응답이라 매 요청 업스트림으로 가는 경로. 우리 플러그인은 본문 상한 10B로, 내장 캐시는
  저장 대상 상태 코드를 404로만 둬서 만들었다. **우리 플러그인의 fill-lock은 껐다.** 내장 캐시에는 락이 없어서,
  같은 조건에서 플러그인 자체 비용만 비교하려는 것이다.
- 기준선 `base-*`는 각 게이트웨이의 플러그인 없는 `/noplugins` 라우트다.

## 조건

- 호스트: MacBook(Docker Desktop, Apple Silicon). 클라이언트 `hey`는 호스트에서 실행한다.
- `hey -c 50 -z 10s`. 회차마다 8개 URL을 번갈아 재는 것을 3회 반복했고, 대표값은 중앙값이다.
- 게이트웨이마다 `cpus: 2.0` 고정(compose). 백엔드는 nginx 목업(정적 JSON 257B).
- 재현: `BENCH_PAIR=all5 ./scripts/bench_gw.sh`

## 결과 (중앙값, 괄호는 3회 범위)

### HIT

| 라벨 | RPS | p50 | p99 | 기준선 대비 RPS |
|---|---|---|---|---|
| `base-kong-go` | 4,709 (3,591–5,724) | 9.2ms | 45ms | — |
| `kong:go` | 2,479 (2,438–2,700) | 11.5ms | 72ms | 0.53 |
| `base-kong` | 4,615 (4,310–5,171) | 9.5ms | 40ms | — |
| `kong:lua` | 13,794 (12,762–14,815) | 2.4ms | 33ms | 2.99 |
| `kong:proxy-cache` | 14,167 (10,117–17,090) | 2.5ms | 22ms | 3.07 |
| `base-apisix` | 4,525 (4,151–4,756) | 10.5ms | 35ms | — |
| `apisix:lua` | 12,254 (10,256–14,948) | 2.9ms | 20ms | 2.71 |
| `apisix:proxy-cache` | 14,803 (9,771–19,856) | 2.3ms | 23ms | 3.27 |

### MISS (fill-lock 끔)

| 라벨 | RPS | p50 | p99 | 기준선 대비 RPS |
|---|---|---|---|---|
| `base-kong-go` | 3,270 (2,732–3,942) | 11.3ms | 52ms | — |
| `kong:go` | 195 (131–414) | 246ms | 642ms | 0.06 |
| `base-kong` | 4,048 (3,147–4,954) | 10.9ms | 42ms | — |
| `kong:lua` | 4,613 (3,644–4,968) | 9.6ms | 34ms | 1.14 |
| `kong:proxy-cache` | 4,897 (4,872–4,939) | 9.2ms | 29ms | 1.21 |
| `base-apisix` | 4,586 (4,013–4,800) | 9.6ms | 45ms | — |
| `apisix:lua` | 4,415 (4,052–4,562) | 9.9ms | 50ms | 0.96 |
| `apisix:proxy-cache` | 4,121 (3,502–4,933) | 10.2ms | 48ms | 0.90 |

## 읽는 법

- **Lua판은 같은 게이트웨이의 내장 캐시와 같은 급이다.** HIT에서 두 게이트웨이 모두 Lua판과 내장 캐시의 RPS 범위가
  크게 겹친다. 중앙값으로는 내장 캐시가 3~20% 높지만, 회차 편차(최대 2배)가 그보다 커서 이 벤치로는 순위를 가를 수 없다.
  MISS에서는 넷 다 기준선과 같은 자리라, 업스트림 왕복이 지배하고 플러그인 비용은 보이지 않는다.
- **Go판은 다른 급이다.** HIT은 플러그인 없는 기준선의 절반(0.53)이고 Lua판의 약 1/5이다. MISS는 기준선의 6%로
  무너지고 회차마다 더 나빠졌다(414→195→131). 요청마다 go-pdk RPC로 pluginserver를 왕복하고, 저장 시도를 위해
  응답 본문을 버퍼링해 넘기는 구조 비용으로 본다(`docs/bench-kong-lua-vs-go.md` 해석과 같다). 원인 분해는 하지 않았다.
- **HIT이 기준선보다 빠른 이유**: Hit은 업스트림을 타지 않는다. 그래서 비율이 1을 넘는다.
- **fill-lock을 켜면 MISS가 달라진다.** 이 표는 락을 끈 수치다. 기본값(락 켬)으로 저장되지 않는 라우트를 재면
  패자들이 `wait_ms`(200ms)를 기다려 p99가 약 230ms가 된다(`docs/bench-apisix-vs-kong-lua.md`). 락이 필요 없는
  비저장 라우트에는 `fill_lock.enabled: false`를 쓴다.

## 이 결과가 말하는 것과 말하지 않는 것

- 노드 로컬 메모리 캐시만 필요하다면 **성능은 내장 캐시를 쓰지 않을 이유가 되지 않는다.** 우리 Lua판을 고를 근거는
  성능이 아니라 기능이다. redis·redis-cluster로 노드 간 공유, 노드 간 fill-lock, 정규식 필터별 TTL, Kong과 APISIX에서
  같은 동작이 그것이다. 내장 캐시에는 이 넷이 없다.
- redis 전략의 성능은 이 벤치 범위 밖이다. 내장 캐시에 대응물이 없기 때문이다.
- 수치는 맥 단일 머신·c50의 상대 비교다. 회차 편차가 커서 10~20% 차이는 잡음으로 읽는다.
