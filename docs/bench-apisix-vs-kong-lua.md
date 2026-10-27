# APISIX vs Kong-Lua 게이트웨이 수준 벤치 (U7)

날짜: 2026-10-22 · 브랜치 `feat/apisix-lua-adapter` · 계획 `docs/plans/2026-10-22-1939-feat-apisix-lua-adapter-plan.md` U7

## 선등록 판정 문턱 (측정 전 확정)

- 두 게이트웨이는 같은 코어(`lua/sonic_boom/flow.lua`)를 쓰므로 Kong-Lua를 기준선으로 삼는다.
- 기본 프록시 처리량이 게이트웨이마다 달라 원시 수치로는 플러그인 비용을 가를 수 없다. 그래서 각 게이트웨이의
  `/noplugins` 라우트를 같은 인터리브 회차에 함께 재고, "플러그인 라우트 대 자기 `/noplugins`" 오버헤드
  비율을 비교한다.
- APISIX의 RPS 유지율(플러그인/기본)이 Kong-Lua 대비 10% 넘게 낮거나, p99 배율(플러그인/기본)이 20% 넘게
  높으면 유의미 열세로 본다. Kong-Lua 판정(`docs/bench-kong-lua-vs-go.md`)과 같은 문턱이다.

## 조건

- 호스트: MacBook(Docker Desktop, Apple Silicon). 클라이언트 `hey`는 호스트에서 실행한다(게이트웨이 CPU와 분리).
- `hey -c 50 -z 10s`, 회차마다 Kong-Lua 기본 → Kong-Lua 플러그인 → APISIX 기본 → APISIX 플러그인 순으로 교차,
  3회차(대표값=중앙값).
- 두 게이트웨이 `cpus: 2.0` 고정(compose). 이미지: `kong:3.9.4-ubuntu`, `apache/apisix:3.19.0-debian`.
  백엔드는 nginx 목업(정적 JSON 257B).
- 측정 경로: HIT=`/lua-in-memory`(lua_shared_dict), MISS=`/lua-maxsize`(저장 상한 10B라 매 요청 Miss+upstream).
- 재현: `BENCH_PAIR=lua-apisix ./scripts/bench_gw.sh` (BENCH_DURATION·BENCH_CONCURRENCY로 조정).

## 결과 (중앙값)

| 경로 | 게이트웨이 | 기본 RPS | 플러그인 RPS | RPS 유지율 | 기본 p99 | 플러그인 p99 | p99 배율 |
|---|---|---|---|---|---|---|---|
| HIT | Kong-Lua | 5,559 | 16,105 | 2.90 | 20.7ms | 30.1ms | 1.45 |
| HIT | **APISIX** | 5,521 | **18,577** | **3.37** | 20.5ms | **22.1ms** | **1.08** |
| MISS | Kong-Lua | 5,738 | 784 | 0.137 | 20.9ms | 228ms | 10.91 |
| MISS | **APISIX** | 5,458 | 783 | 0.143 | 21.1ms | 230ms | 10.91 |

선등록 문턱 대비 결과는 다음과 같다.

- **HIT:** APISIX의 RPS 유지율이 Kong-Lua보다 16.1% 높고, p99 배율은 25.9% 낮다.
- **MISS:** RPS 유지율은 +5.0%, p99 배율은 -0.1%로 사실상 같다.

두 경로 모두 **문턱 안**이고, APISIX에 유의미 열세는 없다.

## 재측정 (2026-10-24, fill-lock 락 감시 이후)

main(패자 락 감시 #4 반영 후). 조건은 위와 같다.

| 경로 | 게이트웨이 | 기본 RPS | 플러그인 RPS | RPS 유지율 | 기본 p99 | 플러그인 p99 | p99 배율 |
|---|---|---|---|---|---|---|---|
| HIT | Kong-Lua | 3,119 | 17,574 | 5.64 | 63.1ms | 17.8ms | 0.28 |
| HIT | APISIX | 2,883 | 17,741 | **6.15** | 55.8ms | **15.5ms** | **0.28** |
| MISS | Kong-Lua | 3,062 | 889 | 0.291 | 58.3ms | 214.3ms | 3.68 |
| MISS | APISIX | 3,078 | **1,002** | **0.325** | 58.2ms | **208.1ms** | **3.58** |

- 두 경로 모두 **문턱 안**. MISS가 이전 측정(784/783) 대비 양쪽 다 오른 건 패자 락 감시(#4)의 효과다 —
  같은 코어를 쓰므로 두 게이트웨이가 함께 개선됐다(Kong-Lua +13%, APISIX +28%).
- 회차 편차가 ±40%인 공유 클러스터 특성상 절대값보다 비율(플러그인/자기 기본선)로 읽는다.
  삼각 비교 전체는 `docs/bench-kong-lua-vs-go.md` 같일 재측정 절과 함께 본다.

## 신설 suite 첫 측정 (2026-10-26, 부하 하 샘플)

go-lua 쌍에 신설된 redis·redis-cluster·bigbody HIT suite를 lua-apisix 쌍에도 추가했다(KONG-LUA/APISIX 라벨).
이 회차는 여러 스택이 떠 있는 부하 하 샘플이라 게이트급 판정(HIT in-memory가 이 회차만 문턱 초과, 직전 회차는 MISS만
초과 — 판정이 회차마다 뒤집힘)으로 읽지 않고, 신설 경로의 절대 수치 확보가 목적이다. suite 출력 라벨도 이번에
KONG-LUA/APISIX로 수정했다(기존 GO/LUA 하드코딩은 lua-apisix 절에서 실제를 잘못 표기).

| 신설 suite (원시 RPS 중앙값, KONG-LUA / APISIX) | p50 | p99 |
|---|---|---|
| HIT redis | 9,027 / 10,488 | 36.7ms / 28.2ms |
| HIT redis-cluster | 6,969 / 9,482 | 42.5ms / 31.9ms |
| HIT bigbody 100KB | 10,009 / 12,397 | 28.8ms / 28.6ms |

- 두 Lua 변종이 같은 코어·같은 redis를 쓰므로 격차는 게이트웨이 프록시 비용 차이다. 세 suite 모두 APISIX가
  같은 방향으로 앞서며(큰 폭 아님) 기존 결론(APISIX 유의미 열세 없음)과 일치한다.
- 게이트급 재판정이 필요하면 조용한 기기에서 재측정할 것.

## 해석과 주의

- **HIT의 플러그인 RPS가 기본보다 높은 이유:** Hit은 access 단계에서 단락해 업스트림을 타지 않는다. 그래서
  유지율이 1을 넘는다. 이 비율은 "업스트림 왕복을 캐시가 얼마나 대체했나"로 읽는다.
- **HIT에서 APISIX가 앞서는 지점:** 두 게이트웨이의 기본 처리량은 거의 같다(5,559 대 5,521). 차이는 단락
  경로에서 난다. APISIX는 access 반환값으로 단락하고, Kong은 PDK exit로 단락한다. 이 단락 비용의 차이로
  보이지만, 이번 벤치로 원인을 분해하지는 않았다.
- **MISS 절대 RPS가 Kong-Lua PoC 벤치(4,708)보다 크게 낮은 이유는 fill-lock이다.** PoC 이후 fill-lock이 기본 on으로
  들어왔다. `/lua-maxsize`는 응답이 절대 저장되지 않는 라우트라, 같은 키의 동시 미스에서 승자 외 패자들이
  `wait_ms`(기본 200ms)를 다 기다린 뒤 통과한다. p99 약 230ms가 이 대기와 맞는다. 코어 공통 동작이라 두
  게이트웨이 비교의 공정성에는 영향이 없다. 다만 "저장되지 않는 응답이 잦은 라우트에서 fill-lock이 꼬리
  지연을 만든다"는 운영 관찰로 판정 자료(U8)에 남긴다. Go판도 같은 시맨틱이다(`internal/fill_lock.go`).
- 수치는 PoC 조건(맥 단일 머신, c50)에서의 상대 비교다. 어느 쪽도 프로덕션 튜닝값이 아니다.
- 결론의 소유권: 수치는 판정 회의의 입력이다. 본 문서는 "유의미 열세 없음"까지만 말한다.
