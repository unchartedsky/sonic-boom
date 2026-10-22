# Kong Lua vs Go 게이트웨이 수준 벤치 (U8)

날짜: 2026-10-21 · 워크트리 `andromedarabbit/kong-lua` · 계획 U8

## 선등록 판정 문턱 (측정 전 확정)

- Lua hit 경로 p99 회귀 >20% 이거나 RPS 저하 >10%면 유의미 열세로 본다(판정은 사람, U9).

## 조건

- 호스트: MacBook(Docker Desktop, Apple Silicon). 클라이언트 `hey`는 **호스트에서** 실행 —
  Docker VM과 자원이 분리돼 게이트웨이 CPU를 잡지 않는다(별도 컨테이너 지시의 목적 충족,
  맥 단일 머신 PoC 조건임을 명시).
- `hey -c 50 -z 10s`, GO→LUA 교차 3회(대표값=중앙값, 편차=최댓값).
- 양 게이트웨이 `cpus: 2.0` 고정(compose). 백엔드: nginx 목업(정적 JSON 257B).
- 측정 경로(같은 저장소 전략 짝): HIT=노드 내 캐시(Go ristretto 프로세스 공유 / Lua lua_shared_dict),
  MISS=저장 상한 10B 라우트(매 요청 Miss+upstream, 저장 시도 후 스킵).
- 재현: `./scripts/bench_gw.sh` (BENCH_DURATION·BENCH_CONCURRENCY로 조정).

## 결과 (중앙값)

| 경로 | | RPS | p50 | p99 |
|---|---|---|---|---|
| HIT | Go | 1,524 (1,156–1,730) | 18.4ms | 150ms |
| HIT | **Lua** | **15,175** (13,328–16,092) | **2.3ms** | **24ms** |
| MISS | Go | 134 (121–219) | 292ms | 837ms |
| MISS | **Lua** | **4,708** (4,584–5,299) | **9.5ms** | **31ms** |

선등록 문턱 대비: Lua hit p99 회귀 없음(오히려 ~6배 개선), RPS 저하 없음(~10배 개선) — **문턱 안**.

## 재측정 (2026-10-22, 코어 이전·fill-lock 기본 on 이후)

브랜치 `feat/apisix-lua-adapter`. 페이즈 로직이 코어 `flow.lua`로 옮겨졌고 fill-lock이 기본 on이다. 조건은 위와 같다.

| 경로 | | RPS | p50 | p99 |
|---|---|---|---|---|
| HIT | Go | 2,822 (2,595–2,898) | 8.3ms | 74ms |
| HIT | **Lua** | **16,030** (8,745–16,533) | **2.0ms** | **31ms** |
| MISS | Go | 263 (256–279) | 226ms | 289ms |
| MISS | **Lua** | **742** (720–785) | **2.2ms** | **226ms** |

- 선등록 문턱 대비 여전히 **문턱 안**이다. Lua가 HIT RPS 약 5.7배, MISS RPS 약 2.8배 앞선다.
- **Lua MISS가 PoC 때(4,708 RPS·p99 31ms)보다 크게 낮아진 원인은 fill-lock이다.** `/lua-maxsize`는 응답이 절대
  저장되지 않아, 같은 키의 동시 미스에서 패자들이 `wait_ms`(기본 200ms)를 다 기다린다. p99 226ms가 그 대기와
  맞는다. Go판도 같은 시맨틱이라 MISS p99가 비슷한 자리(289ms)에 있다. 저장되지 않는 라우트에는
  `fill_lock.enabled: false`가 맞다(`docs/apisix-verdict.md` 운영 관찰).
- Lua HIT 1회차(8,745)는 첫 회차 워밍업 편차로 보이며 중앙값에 영향이 없다.

## 해석과 주의

- 격차의 1차 원인은 아키텍처다: Go판은 요청마다 pluginserver(go-pdk)와 PDK RPC를 왕복하고,
  pluginserver 프로세스가 단일 프로세스로 모든 worker 요청을 처리한다. Lua판은 nginx worker 안에서
  바로 실행된다(IPC 0). Hit 경로 10배는 이 구조 차이로 설명되는 규모다.
- Go MISS p99 837ms·RPS 134는 c50에서 pluginserver 큐 대기가 지배한 관측으로 보인다 —
  본 벤치는 Go판 최적 조건이 아니다(요청당 redis client 재생성 등 dc9c909 개선 이전 관성 포함).
  어느 쪽도 프로덕션 튜닝값이 아니며 "PoC 조건에서의 상대 비교"로만 읽는다.
- Lua Hit의 shm→Lua 복사 비용(본문 257B)은 이 규모에선 관측되지 않았다. 큰 본문 회귀는
  별도 측정 과제로 남긴다(계획 리스크 항목).
- 결론의 소유권: 수치는 판정 회의(U9)의 입력이다. 본 문서는 "유의미 열세 없음"까지만 말한다.

## 원시 기록

교차 3회 원시 RPS: HIT GO[1156 1730 1524] LUA[16092 13328 15175] ·
MISS GO[219 134 121] LUA[4708 4584 5299] — 세부 출력은 `./scripts/bench_gw.sh` 재실행으로 확보.
