-- 제품 식별(게이트웨이별). 어댑터가 모듈 로드 시 1회 덮어쓴다 — nginx 프로세스 하나에 어댑터 하나.
-- 코어는 이 값을 호출 시점에 읽는다(로드 시점 업밸류로 캡처하지 않는다 — 로드 순서 무관).
-- label: 로그 접두, key_prefix: 캐시 키 접두(게이트웨이 간 키 공간 분리), version: cache_version 폴백.
return {
  label = "sonic-boom-lua: ",
  key_prefix = "sbl",
  version = "0.1.0",
}
