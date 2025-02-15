package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/server/kong_plugin_protocol"
	"github.com/creasty/defaults"
	lib_store "github.com/eko/gocache/lib/v4/store"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/umisama/go-regexpcache"
	"gopkg.in/go-playground/validator.v9"
)

var Version = "1.0"
var Priority = 1

var once sync.Once

var redisPool *RedisPool

// TODO cache control 은 나중에 구현하자
type Config struct {
	ResponseCodes        []int       `json:"response_code" validate:"required,gte=0" default:"[200, 301, 404]"`
	RequestMethods       []string    `json:"request_method" validate:"required" default:"[\"GET\", \"HEAD\"]"`
	ContentTypes         []string    `json:"content_type" validate:"required" default:"[\"text/plain\", \"application/json\", \"application/json; charset=utf-8\"]"`
	VaryHeaders          []string    `json:"vary_headers" validate:"required" default:"[]"`
	Filters              []Filter    `json:"filters" validate:"required" default:"[]"`
	CacheTTL             int         `json:"cache_ttl" validate:"gte=0" default:"0"`
	CacheControl         bool        `json:"cache_control" validate:"" default:"false"`
	CacheableBodyMaxSize int         `json:"cacheable_body_max_size" validate:"gte=0" default:"0"`
	CacheVersion         string      `json:"cache_version" validate:"" default:""`
	Strategy             string      `json:"strategy" validate:"required,oneof=redis" default:"redis"`
	Redis                RedisConfig `json:"redis"`
	LogConf              LogConfig   `json:"log" validate:"" default:"{}"`

	logger *Logger `validate:"-"`
}

type Filter struct {
	Name     string `json:"name" validate:"required" default:""`
	Rules    []Rule `json:"rules" validate:"required" default:""`
	CacheTTL int    `json:"cache_ttl" validate:"gte=0" default:"0"`
}

type Rule struct {
	Header string `json:"header" validate:"" default:""`
	Regexp string `json:"regexp" validate:"required"`
}

func (r *Rule) pathRule() bool {
	return r.Header == ""
}

func (r *Rule) headerRule() bool {
	return r.Header != ""
}

type BaseFilter struct {
	Regexp   string `json:"regexp" validate:"required"`
	CacheTTL int    `json:"cache_ttl" validate:"gte=0" default:"0"`
}

func New() interface{} {
	config := &Config{}
	if err := defaults.Set(config); err != nil {
		panic(err)
	}

	return config
}

func (conf *Config) isDebug() bool {
	return conf.LogConf.LogLevel == zerolog.LevelDebugValue
}

func (conf *Config) cacheVersion() string {
	datadogVersion := os.Getenv("DD_VERSION")
	if datadogVersion != "" {
		conf.logger.Info().Msgf("DD_VERSION: %s", datadogVersion)
		return datadogVersion
	}

	return Version
}

// See https://github.com/Kong/go-pdk/issues/78
func (conf *Config) Init() {
	// Redis 풀과 같은 공유 리소스만 once.Do()로 초기화
	once.Do(func() {
		ctx := context.Background()
		pool, err := NewRedisPool(ctx, conf.Redis)
		if err != nil {
			panic(err)
		}
		redisPool = pool
	})

	// logger는 매 Config 인스턴스마다 개별적으로 초기화
	conf.logger = NewLogger(&conf.LogConf)

	// 나머지 설정들 초기화
	for _, filter := range conf.Filters {
		if defaults.CanUpdate(filter.CacheTTL) {
			filter.CacheTTL = conf.CacheTTL
		}
	}

	if conf.CacheVersion == "" {
		conf.CacheVersion = conf.cacheVersion()
	}
}

func (conf *Config) Close() error {
	if conf.logger != nil {
		conf.logger.Close()
	}
	return nil
}

func (conf *Config) Access(kong *pdk.PDK) {
	// From https://github.com/lampnick/kong-rate-limiting-golang/blob/master/custom-rate-limiting.go
	defer func() {
		if err := recover(); err != nil {
			log.Printf("kong plugin panic at: %+v, err: %+v", time.Now(), err)
			if kong == nil {
				log.Printf("kong fatal err ===> kong is nil at: %+v", time.Now())
			} else {
				_ = kong.Log.Err(fmt.Sprint(err))
			}
		}
	}()

	conf.Init()

	logger := conf.logger

	method, err := kong.Request.GetMethod()
	if err == nil {
		logger.Trace().Msgf("Request method: %s", method)
	}
	path, err := kong.Request.GetPath()
	if err == nil {
		logger.Trace().Msgf("Request path: %s", path)
	}

	if conf.isDebug() {
		if err = conf.checkConfig(); err != nil {
			logger.Fatal().Err(err).Msg("Config check failed")
			return
		}

		if err = kong.Response.SetHeader("X-sonic-boom-Plugin-Version", Version); err != nil {
			logger.Warn().Err(err).Msg("failed to set header")
		}
	}

	cacheable, cacheTTL := conf.cacheableRequest(kong)
	if !cacheable {
		if err = kong.Response.SetHeader("X-Cache-Status", "Bypass"); err != nil {
			logger.Error().Err(err).Msg("SetHeader failed")
			return
		}
		return
	}

	rawBody, err := kong.Request.GetRawBody()
	if err != nil {
		logger.Error().Err(err).Msg("Getting raw body has failed")
		return
	}
	if rawBody == nil {
		logger.Debug().Msg("raw body is empty")
	} else {
		logger.Debug().Msgf("Raw body length is %d", len(rawBody))
	}

	cacheKeyID, err := NewCacheKey(kong, conf, rawBody, cacheTTL)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create cache key")
		return
	}

	if conf.isDebug() {
		if err = kong.Response.SetHeader("X-Cache-Key", cacheKeyID); err != nil {
			logger.Debug().Err(err).Msg("Failed to set header")
			//_ = log.Err("SetHeader failed: ", err.Error())
		}
	}

	ctx := context.Background()

	pooled, err := redisPool.BorrowObject(ctx)
	if err != nil {
		conf.logger.Error().Err(err).Msg("Failed to borrow marshaler from Redis pool")
		return
	}
	defer func() {
		poolErr := redisPool.ReturnObject(ctx, pooled)
		if poolErr != nil {
			conf.logger.Error().Err(poolErr).Msg("Failed to return marshaler to Redis pool")
		}
	}()

	cached, err := pooled.Marshaler.Get(ctx, cacheKeyID, new(CacheValue))
	if cached == nil || err != nil || err == redis.Nil {
		logger.Debug().Msg("Cache miss")

		if err == redis.Nil {
			logger.Debug().Err(err).Msgf("Unable to get cache key '%s' from the cache", cacheKeyID)
		} else if err != nil {
			if err.Error() == lib_store.NOT_FOUND_ERR {
				logger.Debug().Err(err).Msgf("Unable to get cache key '%s' from the cache", cacheKeyID)
			} else {
				logger.Error().Err(err).Msgf("Unable to get cache key '%s' from the cache", cacheKeyID)
			}
		}

		// this request wasn't found in the data store, but the client only wanted
		// cache data. see https://tools.ietf.org/html/rfc7234#section-5.2.1.7
		//if conf.cache_control and cc["only-if-cached"] then
		//	return kong.response.exit(ngx.HTTP_GATEWAY_TIMEOUT)
		//end

		if err = SetPlugin(kong, "reqBody", rawBody); err != nil {
			logger.Error().Err(err).Msg("Failed to set reqBody in plugin context")
			return
		}
		logger.Debug().Msg("Request body is saved to Context")

		err = conf.signalCacheReq(kong, CacheSignal{cacheKeyID, cacheTTL})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to signal cache request")
			return
		}
		return
	}

	logger.Debug().Msg("Cache hit")

	cacheValue := cached.(*CacheValue)
	cacheSignal := CacheSignal{
		CacheKeyID: cacheKeyID,
		CacheTTL:   cacheTTL,
	}

	if cacheValue.Version != conf.CacheVersion {
		logger.Warn().Msgf("Cache version mismatch, purging: %s != %s", cacheValue.Version, conf.CacheVersion)
		if err = pooled.Marshaler.Delete(ctx, cacheKeyID); err != nil {
			logger.Error().Err(err).Msg("Purging cache failed")
			return
		}
		if err = conf.signalCacheReqWithStatus(kong, cacheSignal, "Bypass"); err != nil {
			logger.Error().Err(err).Msg("Failed to signal cache request")
			return
		}
	}

	//-- figure out if the client will accept our cache value
	if conf.CacheControl {
		logger.Fatal().Msg("Cache control is enabled but not implemented yet")
		return
	} else {
		//-- don't serve stale data; res may be stored for up to `conf.storage_ttl` secs
		now := time.Now()
		secs := now.Unix()

		if (secs - cacheValue.Timestamp) > int64(conf.CacheTTL) {
			if err = conf.signalCacheReqWithStatus(kong, cacheSignal, "Refresh"); err != nil {
				logger.Error().Err(err).Msg("Failed to signal cache request")
				return
			}
		}
	}

	// we have cache data yo!
	// expose response data for logging plugins
	// 그래서 어디에 쓴다는 건지... https://github.com/search?q=org%3AKong+proxy_cache_hit&type=code
	responseData := ""
	//responseData = {
	//	res = res,
	//	req = {
	//		body = res.req_body,
	//	},
	//	server_addr = ngx.var.server_addr,
	//}

	if responseData != "" {
		if err = kong.Ctx.SetShared("proxy_cache_hit", responseData); err != nil {
			logger.Error().Err(err).Msg("Failed to set shared context")
			return
		}
	}
	if err = kong.Nginx.SetCtx("KONG_PROXIED", true); err != nil {
		logger.Error().Err(err).Msg("Failed to set nginx context `KONG_PROXIED`")
		return
	}

	for key := range cacheValue.Headers { //nolint:gosimple,gofmt
		// NOTE: https://github.dev/Kong/kong/blob/master/kong/plugins/proxy-cache/handler.lua 를 베꼈는데 의미를 잘 모르겠다.
		if !overwritableHeader(key) {
			delete(cacheValue.Headers, key)
		}

		if headerToDelete(key) {
			delete(cacheValue.Headers, key)
		}
	}

	now := time.Now()
	secs := now.Unix()
	age := strconv.FormatInt(secs-cacheValue.Timestamp, 10)
	cacheValue.Headers["Age"] = []string{age}
	cacheValue.Headers["X-Cache-Status"] = []string{"Hit"}

	logger.Debug().Msgf("CacheValue Headers: %+v", cacheValue.Headers)
	kong.Response.Exit(cacheValue.Status, cacheValue.Body, cacheValue.Headers)
}

func (conf *Config) cacheableRequest(kong *pdk.PDK) (bool, int) {
	if !conf.cacheableRequestMethod(kong) {
		conf.logger.Debug().Msg("Request method is not cacheable")
		return false, 0
	}

	cacheable, ttl := conf.filtered(kong)
	if cacheable {
		return cacheable, ttl
	}

	// check for explicit disallow directives
	// TODO note that no-cache isnt quite accurate here
	//if conf.cache_control and (cc["no-store"] or cc["no-cache"] or
	//	ngx.var.authorization) then
	//	return false
	//end

	return false, 0
}

func (conf *Config) filtered(kong *pdk.PDK) (bool, int) {
	filters := conf.Filters
	if len(filters) == 0 {
		return true, conf.CacheTTL
	}

	for _, filter := range filters {
		if conf.rulesFiltered(kong, filter.Rules) {
			return true, filter.CacheTTL
		}
	}

	conf.logger.Debug().Msg("Header does not match any filter")
	return false, 0
}

func (conf *Config) rulesFiltered(kong *pdk.PDK, rules []Rule) bool {
	for _, rule := range rules {
		if rule.pathRule() {
			if ok := conf.cacheablePath(kong, rule); !ok {
				return false
			}
		} else if rule.headerRule() {
			if ok := conf.cacheableHeader(kong, rule); !ok {
				return false
			}
		} else {
			panic(fmt.Sprintf("Unknown rule type: %T", rule))
		}
	}
	return true
}

func (conf *Config) cacheablePath(kong *pdk.PDK, rule Rule) bool {
	if !rule.pathRule() {
		panic("Rule is not a path rule")
	}

	path, err := kong.Request.GetPathWithQuery()
	if err != nil {
		return false
	}

	r := regexpcache.MustCompile(rule.Regexp)
	if r.MatchString(path) {
		conf.logger.Debug().Msgf("Path %s is cacheable", path)
		return true
	}

	conf.logger.Debug().Msgf("Path %s is not cacheable", path)
	return false
}

func (conf *Config) cacheableHeader(kong *pdk.PDK, rule Rule) bool {
	if !rule.headerRule() {
		panic("Rule is not a header rule")
	}

	v, err := kong.Request.GetHeader(rule.Header)
	if err != nil {
		conf.logger.Error().Err(err).Msgf("Failed to get header: %s", rule.Header)
		return false
	}

	if v == "" {
		conf.logger.Debug().Msgf("Header %s is empty", rule.Header)
		return false
	}

	r := regexpcache.MustCompile(rule.Regexp)
	if r.MatchString(v) {
		return true
	}

	conf.logger.Debug().Msgf("Header %s is not cacheable", rule.Header)
	return false
}

func (conf *Config) cacheableRequestMethod(kong *pdk.PDK) bool {
	method, err := kong.Request.GetMethod()
	if err != nil {
		conf.logger.Error().Err(err).Msg("Failed to get request method")
		return false
	}

	for _, s := range conf.RequestMethods {
		if strings.EqualFold(method, s) {
			return true
		}
	}

	return false
}

// -- http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html#sec13.5.1
// -- note content-length is not strictly hop-by-hop but we will be
// -- adjusting it here anyhow
var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailers":            true,
	"transfer-encoding":   true,
	"upgrade":             true,
	"content-length":      true,
}

func overwritableHeader(header string) bool {
	nHeader := strings.ToLower(header)
	return !hopByHopHeaders[nHeader] &&
		!strings.Contains(nHeader, "ratelimit-remaining")
}

var headersToDelete = map[string]bool{
	"x-cache-status": true,
	"age":            true,
}

func headerToDelete(header string) bool {
	nHeader := strings.ToLower(header)
	return headersToDelete[nHeader]
}

func (conf *Config) checkConfig() error {
	validate := validator.New()
	err := validate.Struct(conf)
	if err != nil {
		return err
	}
	return nil
}

func (conf *Config) signalCacheReq(kong *pdk.PDK, signal CacheSignal) error {
	return conf.signalCacheReqWithStatus(kong, signal, "")
}

type CacheSignal struct {
	CacheKeyID string `json:"cache_key_id" validate:"required"`
	CacheTTL   int    `json:"cache_ttl" validate:"gte=0" default:"0"`
}

func (conf *Config) signalCacheReqWithStatus(kong *pdk.PDK, signal CacheSignal, cacheStatus string) error {
	logger := conf.logger
	logger.Debug().Msgf("signal: %+v", signal)

	if err := SetPluginEx(kong, "cacheSignal", signal); err != nil {
		logger.Error().Err(err).Msgf("Failed to set cacheSignal in plugin context: %+v", signal)
		return err
	}
	logger.Debug().Msgf("proxy_cache is stored: %+v", signal)

	if cacheStatus == "" {
		cacheStatus = "Miss"
	}

	if err := kong.Response.SetHeader("X-Cache-Status", cacheStatus); err != nil {
		logger.Error().Err(err).Msg("Setting header `X-Cache-Status` failed")
	}
	logger.Debug().Msgf("X-Cache-Status: %s", cacheStatus)

	return nil
}

// NOTE kong.Request.GetRawBody() 의 구현을 베꼈다
func serviceResponseRawBody(kong *pdk.PDK) ([]byte, error) {
	out := new(kong_plugin_protocol.RawBodyResult)
	err := kong.ServiceResponse.Ask(`kong.service.response.get_raw_body`, nil, out)
	if err != nil {
		return nil, err
	}

	switch x := out.Kind.(type) {
	case *kong_plugin_protocol.RawBodyResult_Content:
		return x.Content, nil

	case *kong_plugin_protocol.RawBodyResult_BodyFilepath:
		return os.ReadFile(x.BodyFilepath)

	case *kong_plugin_protocol.RawBodyResult_Error:
		return nil, errors.New(x.Error)

	default:
		return out.GetContent(), nil
	}
}

func (conf *Config) Response(kong *pdk.PDK) {
	conf.Init()

	logger := conf.logger
	method, err := kong.Request.GetMethod()
	if err == nil {
		logger.Trace().Msgf("Request method: %s", method)
	}
	path, err := kong.Request.GetPath()
	if err == nil {
		logger.Trace().Msgf("Request path: %s", path)
	}

	logger.Debug().Msg("Response is called")

	cacheSignal := CacheSignal{}
	err = GetPluginAnyEx(kong, "cacheSignal", &cacheSignal)
	if err != nil {
		logger.Debug().Err(err).Msgf("Failed to get cacheKeyID from plugin context")
		return
	}
	if cacheSignal.CacheKeyID == "" {
		logger.Debug().Msg("No cached object found")
		return
	}
	logger.Debug().Msgf("cacheKeyID type is %s", reflect.TypeOf(cacheSignal))
	logger.Debug().Msgf("cacheKeyID is found: %v", cacheSignal)

	// ProxyCacheHandler:header_filter
	if !conf.cacheableResponse(kong) {
		if err = kong.Response.SetHeader("X-Cache-Status", "Bypass"); err != nil {
			logger.Error().Err(err).Msg("Setting header `X-Cache-Status` failed")
			return
		}
		return
	}

	// ProxyCacheHandler:body_filter
	httpStatus, err := kong.Response.GetStatus()
	if err != nil {
		logger.Error().Err(err).Msg("Getting response status failed")
		return
	}

	headers, err := kong.Response.GetHeaders(1000)
	if err != nil {
		logger.Error().Err(err).Msg("Getting response headers failed")
		return
	}
	if conf.isDebug() {
		for k, v := range headers {
			logger.Debug().Msgf("Response header: %s: %s", k, v)
		}
	}

	rawBody, err := serviceResponseRawBody(kong)
	if err != nil {
		logger.Error().Err(err).Msg("Getting response body has failed")
		return
	}
	if rawBody == nil {
		logger.Debug().Msg("Response body is empty")
	} else {
		logger.Debug().Msgf("Response body length is %d", len(rawBody))
		if conf.CacheableBodyMaxSize > 0 && len(rawBody) > conf.CacheableBodyMaxSize {
			logger.Debug().Msgf("Body length is bigger than allowed body_max_size: %d", conf.CacheableBodyMaxSize)
			return
		}
	}

	now := time.Now()
	secs := now.Unix()

	//	proxy_cache.res_headers = resp_get_headers(0, true)
	//	proxy_cache.res_ttl = conf.cache_control and resource_ttl(cc) or conf.cache_ttl
	_, err = GetPluginAny(kong, "reqBody")
	if err != nil {
		logger.Error().Err(err).Msgf("Failed to get reqBody from plugin context")
		return
	}

	cacheValue := &CacheValue{
		Status:    httpStatus,
		Headers:   headers,
		Body:      rawBody,
		BodyLen:   len(rawBody),
		Timestamp: secs,
		TTL:       int64(conf.CacheTTL),
		Version:   conf.CacheVersion,
		//ReqBody: reqBody.([]byte),
	}
	validate := validator.New()
	if err = validate.Struct(conf); err != nil {
		logger.Error().Err(err).Msg("Cache value validation failed")
		//validationErrors := err.(validator.ValidationErrors)
		return
	}
	logger.Debug().Msgf("cacheValue: %+v", cacheValue)

	ctx := context.Background()

	pooled, err := redisPool.BorrowObject(ctx)
	if err != nil {
		conf.logger.Error().Err(err).Msg("Failed to borrow marshaler")
		return
	}
	defer func() {
		poolErr := redisPool.ReturnObject(ctx, pooled)
		if poolErr != nil {
			conf.logger.Error().Err(poolErr).Msg("Failed to return marshaler to Redis pool")
		}
	}()

	cacheKeyID := cacheSignal.CacheKeyID
	if err = pooled.Marshaler.Set(ctx, cacheKeyID, cacheValue, lib_store.WithExpiration(time.Duration(cacheValue.TTL)*time.Second)); err != nil {
		logger.Error().Err(err).Msg("Cache set failed")
		return
	}
	logger.Debug().Msgf("Cache set: %s", cacheKeyID)
}

func (conf *Config) cacheableResponse(kong *pdk.PDK) bool {
	if !conf.cacheableResponseStatus(kong) {
		return false
	}

	if !conf.cacheableResponseContentType(kong) {
		return false
	}

	return true
}

func (conf *Config) cacheableResponseStatus(kong *pdk.PDK) bool {
	status, err := kong.Response.GetStatus()
	if err != nil {
		conf.logger.Error().Err(err).Msg("Getting response status failed")
		return false
	}

	for _, s := range conf.ResponseCodes {
		if status == s {
			return true
		}
	}

	return false
}

func (conf *Config) cacheableResponseContentType(kong *pdk.PDK) bool {
	// Lua 에선 아래와 같이 처리한다. content_type의 타입이 table인 경우는 어떻게 할까?
	// if not content_type or type(content_type) == "table" or content_type == "" then
	contentType, err := kong.Nginx.GetVar("sent_http_content_type")
	if err != nil {
		conf.logger.Error().Err(err).Msg("Getting response content type failed")
		return false
	}
	conf.logger.Debug().Msgf("Response content type: %s", contentType)

	if contentType == "" {
		return false
	}
	for _, ct := range conf.ContentTypes {
		if strings.EqualFold(contentType, ct) {
			return true
		}
	}

	// TODO cache control은 나중에 구현하자
	//if conf.cache_control and (cc["private"] or cc["no-store"] or cc["no-cache"])
	//then
	//	return false
	//end
	//
	//if conf.cache_control and resource_ttl(cc) <= 0 then
	//	return false
	//end

	return true
}
