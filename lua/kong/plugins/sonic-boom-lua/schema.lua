-- U1은 최소 스키마. 전 필드는 U3에서 KTD7(구현된 동작의 필드만)에 따라 채운다.
-- selfcheck는 구현된 개발 기능(U2 검증 진입) — 골든 벡터 자기검증 라우트용.
return {
  name = "sonic-boom-lua",
  fields = {
    {
      config = {
        type = "record",
        fields = {
          { selfcheck = { type = "boolean", default = false } },
        },
      },
    },
  },
}
