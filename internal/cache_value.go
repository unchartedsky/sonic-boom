package internal

import "fmt"

// NOTE https://github.com/Kong/kong/blob/df14db5bf938e7d6cd9c0150336fe70fb96891c6/kong/plugins/proxy-cache/handler.lua#L414
// 에서는 req_body까지 캐시에 저장하지만 굳이 그럴 필요가 있나 싶다.
// req_body  = ctx.req_body,
type CacheValue struct {
	Status    int `validate:"required,gte=0"`
	Headers   map[string][]string
	Body      []byte ``
	BodyLen   int    `validate:"gte=0"`
	Timestamp int64  `validate:"required,gte=0"`
	TTL       int64  `validate:"required,gte=0"`
	Version   string `validate:"required"`
	ReqBody   []byte `validate:"required"`
}

func (v *CacheValue) String() string {
	return fmt.Sprintf("CacheValue{Status: %d, Headers: %v, BodyLen: %d, Timestamp: %d, TTL: %d, Version: %s}", v.Status, v.Headers, v.BodyLen, v.Timestamp, v.TTL, v.Version)
}
