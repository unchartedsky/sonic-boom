-- U1은 최소 스키마(빈 config). 전 필드는 U3에서 KTD7(구현된 동작의 필드만)에 따라 채운다.
return {
  name = "sonic-boom-lua",
  fields = {
    {
      config = {
        type = "record",
        fields = {},
      },
    },
  },
}
