package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Kong/go-pdk"
)

type CacheKey struct {
	Consumer string
	// Principal은 consumer 미확정 요청의 자격증명(Authorization)이다 — 익명 공용
	// 슬롯 대신 자격증명별 슬롯으로 분리한다(⑥ 요청층 자격증명 vary). consumer가
	// 있거나 vary_headers에 Authorization이 선언돼 있으면 빈 값으로 남는다.
	Principal string
	Service   string
	Route     string
	Method    string `validate:"required" `
	URL       string `validate:"required" `
	QueryArgs map[string][]string
	Headers   map[string]string
	Body      []byte
	CacheTTL  int
}

func (c *CacheKey) String() string {
	return fmt.Sprintf(
		"CacheKey{Consumer: %s, Principal: %s, Service: %s, Route: %s, Method: %s, URL: %s, QueryArgs: %v, Headers: %v, BodyLen: %d, CacheTTL: %d}",
		c.Consumer, c.Principal, c.Service, c.Route, c.Method, c.URL, c.QueryArgs, c.Headers, len(c.Body), c.CacheTTL,
	)
}

func consumerID(kong *pdk.PDK) (string, error) {
	obj, err := kong.Client.GetConsumer()
	if err != nil {
		return "", err
	}
	return obj.Id, nil
}

func serviceID(kong *pdk.PDK) (string, error) {
	obj, err := kong.Router.GetService()
	if err != nil {
		return "", err
	}
	return obj.Id, nil
}

func routeID(kong *pdk.PDK) (string, error) {
	obj, err := kong.Router.GetRoute()
	if err != nil {
		return "", err
	}
	return obj.Id, nil
}

func NewCacheKey(kong *pdk.PDK, conf *Config, body []byte, cacheTTL int) (string, error) {
	logger := conf.logger

	consumerID, err := consumerID(kong)
	if err != nil {
		logger.Error().Err(err).Msg("Getting consumerID has failed")
	}
	if consumerID == "" {
		logger.Debug().Msg("consumerID is empty")
	}

	serviceID, err := serviceID(kong)
	if err != nil {
		logger.Error().Err(err).Msg("Getting serviceID has failed")
	}
	if serviceID == "" {
		logger.Debug().Msg("serviceID is empty")
	}

	routeID, err := routeID(kong)
	if err != nil {
		logger.Error().Err(err).Msg("Getting routeID has failed")
	}
	if routeID == "" {
		logger.Debug().Msg("routeID is empty")
	}

	method, err := kong.Request.GetMethod()
	if err != nil {
		logger.Error().Err(err).Msg("Getting method has failed")
	}
	if method == "" {
		logger.Debug().Msg("method is empty")
	}

	path, err := kong.Request.GetPath()
	if err != nil {
		logger.Error().Err(err).Msg("Getting path has failed")
	}
	if path == "" {
		logger.Debug().Msg("path is empty")
	}

	//pathWithQuery, err := kong.Request.GetPathWithQuery()
	//if err != nil {
	//	_ = log.Err("pathWithQuery is not set")
	//}
	//_ = log.Debug("pathWithQuery is: ", pathWithQuery)

	authInVary := false
	headers := make(map[string]string)
	for _, v := range conf.VaryHeaders {
		if strings.EqualFold(v, "Authorization") {
			authInVary = true
		}
		headerValue, error := kong.Request.GetHeader(v)
		if headerValue == "" || error != nil {
			logger.Debug().Msgf("vary header %s is not found", v)
			continue
		}
		headers[v] = headerValue
		logger.Debug().Msgf("vary header %s is found: %s", v, headerValue)
	}

	// 요청층 자격증명 vary(⑥): consumer 미확정 요청이 Authorization을 갖고 있으면 익명
	// 공용 슬롯 대신 자격증명별 슬롯으로 분리한다. vary_headers에 선언된 Authorization은
	// 이미 vary 성분으로 들어가므로 건너뛴다. cache_control=true(기본) 라우트는 가드가
	// Authorization 요청을 Bypass하므로 여기에 닿는 건 cache_control: false 라우트뿐이다.
	principal := ""
	if consumerID == "" && !authInVary {
		principal, _ = kong.Request.GetHeader("Authorization")
	}

	queryArgs, err := kong.Request.GetQuery(1000)
	if err != nil {
		logger.Error().Err(err).Msg("Getting queryArgs has failed")
		//return
	}
	if conf.isDebug() {
		if queryArgs == nil {
			logger.Debug().Msg("queryArgs is empty")
		} else {
			for key, v := range queryArgs {
				logger.Debug().Msgf("Query arg %s is found: %s", key, v)
			}
		}
	}

	cacheKey := &CacheKey{
		Consumer:  consumerID,
		Principal: principal,
		Service:   serviceID,
		Route:     routeID,
		Method:    method,
		URL:       path,
		Headers:   headers,
		QueryArgs: queryArgs,
		Body:      body,
		CacheTTL:  cacheTTL,
	}

	if errs := configValidator.Struct(cacheKey); errs != nil {
		logger.Error().Err(errs).Msg("validation error")
		return "", errs
	}

	return generateCacheKeyID(logger, cacheKey)
}

// generateCacheKeyID 는 CacheKey를 결정론적 바이트열로 직렬화해 sha256 hex로
// 압축한다(이전 xxhash64는 비암호 해시라 2^32 지점에서 충돌했다). 직렬화 규칙
// (길이 접두 필드 + 정렬된 맵 항목)이 결정론성의 전부다 — 맵 삽입 순서와
// 무관하게 같은 논리적 키는 같은 바이트열이 된다. 이전 hashstructure 대비 요청
// 핫패스의 reflect 비용이 사라지고, 알고리즘 전환은 CacheVersion
// 마커(sonic_boom.go cacheVersion)로 구획된다.
func generateCacheKeyID(logger *Logger, cacheKey *CacheKey) (string, error) {
	var b strings.Builder
	writeField := func(parts ...string) {
		for _, p := range parts {
			b.WriteString(strconv.Itoa(len(p)))
			b.WriteByte(':')
			b.WriteString(p)
			b.WriteByte('|')
		}
	}

	writeField(cacheKey.Consumer, cacheKey.Principal, cacheKey.Service, cacheKey.Route, cacheKey.Method, cacheKey.URL)

	// 맵 항목은 키 정렬 후 기록 — 삽입 순서가 결정론성을 깨지 않게.
	// 값 하나하나 길이 접두로 기록한다 — NUL join은 `x=a%00b`(단일 값)와
	// `x=a&x=b`(다중 값)가 같은 바이트열로 충돌했다(PE3).
	writeSortedMap := func(m map[string][]string) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			vs := append([]string(nil), m[k]...)
			sort.Strings(vs)
			writeField(append([]string{"q", k}, vs...)...)
		}
	}
	writeSortedMap(cacheKey.QueryArgs)

	hKeys := make([]string, 0, len(cacheKey.Headers))
	for k := range cacheKey.Headers {
		hKeys = append(hKeys, k)
	}
	sort.Strings(hKeys)
	for _, k := range hKeys {
		writeField("h", k, cacheKey.Headers[k])
	}

	b.WriteString("b:")
	b.Write(cacheKey.Body)
	b.WriteString("|ttl:")
	b.WriteString(strconv.Itoa(cacheKey.CacheTTL))

	// 해시 형식 전환(십진수 xxhash → 64-hex sha256)에 버전 마커가 없어도 구 키는
	// 새 키와 절대 겹치지 않는다 — 재조회되지 않은 채 TTL 만료까지 잔존 후 소멸.
	// principal 성분 추가(2026-10-27)도 마커 없이 같은 원칙으로 자연 전환된다.
	sum := sha256.Sum256([]byte(b.String()))
	cacheKeyID := hex.EncodeToString(sum[:])
	logger.Debug().Msgf("cache key id %s is generated from: %v", cacheKeyID, cacheKey)
	return cacheKeyID, nil
}
