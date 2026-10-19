package internal

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Kong/go-pdk"
	"github.com/cespare/xxhash/v2"
)

type CacheKey struct {
	Consumer  string
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
		"CacheKey{Consumer: %s, Service: %s, Route: %s, Method: %s, URL: %s, QueryArgs: %v, Headers: %v, BodyLen: %d, CacheTTL: %d}",
		c.Consumer, c.Service, c.Route, c.Method, c.URL, c.QueryArgs, c.Headers, len(c.Body), c.CacheTTL,
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

	headers := make(map[string]string)
	for _, v := range conf.VaryHeaders {
		headerValue, error := kong.Request.GetHeader(v)
		if headerValue == "" || error != nil {
			logger.Debug().Msgf("vary header %s is not found", v)
			continue
		}
		headers[v] = headerValue
		logger.Debug().Msgf("vary header %s is found: %s", v, headerValue)
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

// generateCacheKeyID 는 CacheKey를 결정론적 바이트열로 직렬화해 xxhash로
// 압축한다. 직렬화 규칙(길이 접두 필드 + 정렬된 맵 항목)이 결정론성의 전부다 —
// 맵 삽입 순서와 무관하게 같은 논리적 키는 같은 바이트열이 된다. 이전
// hashstructure 대비 요청 핫패스의 reflect 비용이 사라지고, 알고리즘 전환은
// CacheVersion 마커(sonic_boom.go cacheVersion)로 구획된다.
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

	writeField(cacheKey.Consumer, cacheKey.Service, cacheKey.Route, cacheKey.Method, cacheKey.URL)

	// 맵 항목은 키 정렬 후 기록 — 삽입 순서가 결정론성을 깨지 않게.
	writeSortedMap := func(m map[string][]string) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			vs := append([]string(nil), m[k]...)
			sort.Strings(vs)
			writeField("q", k, strings.Join(vs, "\x00"))
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

	hash := xxhash.Sum64([]byte(b.String()))
	cacheKeyID := strconv.FormatUint(hash, 10)
	logger.Debug().Msgf("cache key id %s is generated from: %v", cacheKeyID, cacheKey)
	return cacheKeyID, nil
}
