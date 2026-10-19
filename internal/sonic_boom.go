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

	"github.com/eko/gocache/lib/v4/marshaler"

	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/server/kong_plugin_protocol"
	"github.com/creasty/defaults"
	"github.com/eko/gocache/lib/v4/cache"
	lib_store "github.com/eko/gocache/lib/v4/store"
	redis_store "github.com/eko/gocache/store/redis/v4"
	rediscluster_store "github.com/eko/gocache/store/rediscluster/v4"

	//lib_store "github.com/eko/gocache/lib/v4/store"
	"github.com/dgraph-io/ristretto/v2"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	"github.com/go-playground/validator/v10"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/umisama/go-regexpcache"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var (
	Version  = "1.0.4"
	Priority = 1

	// InMemory 설정값별로 캐시를 관리하기 위한 맵
	ristrettoClients sync.Map // map[InMemoryConfig]*ristretto.Cache
	cacheStores      sync.Map // map[InMemoryConfig]store.StoreInterface[any]

	// redis 클라이언트는 프로세스 수명으로 공유한다. 매 요청 생성 시
	// 다이얼·핸드셰이크가 요청마다 반복된다(실측 9.3배 지연, 2026-10-19).
	redisClients        sync.Map // map[RedisConfig]*redis.Client
	redisClusterClients sync.Map // map[redisClusterKey]*redis.ClusterClient

	tracer      = otel.Tracer("sonic-boom")
	otelEnabled = os.Getenv("OTEL_SDK_DISABLED") != "true"

	// 검증기는 프로세스당 1회 구성한다(validator.New()+등록이 실비용이고 인스턴스는
	// 동시 사용 안전). 라우트별 검증은 매 요청 정확히 수행한다.
	configValidator = newConfigValidator()
)

func newConfigValidator() *validator.Validate {
	validate := validator.New()

	// Redis 설정 validation을 위한 custom validation 추가
	validate.RegisterStructValidation(func(sl validator.StructLevel) {
		config := sl.Current().Interface().(Config)

		// Redis strategy일 때 Redis 설정 검증
		if config.Strategy == "redis" {
			if config.Redis.Host == "" {
				sl.ReportError(config.Redis.Host, "Host", "Redis.Host", "required", "")
			}
			if config.Redis.Port <= 0 || config.Redis.Port > 65536 {
				sl.ReportError(config.Redis.Port, "Port", "Redis.Port", "range", "")
			}
		}

		// Redis Cluster strategy일 때 RedisCluster 설정 검증
		if config.Strategy == "redis-cluster" {
			if len(config.RedisCluster.Addrs) == 0 {
				sl.ReportError(config.RedisCluster.Addrs, "Addrs", "RedisCluster.Addrs", "required", "")
			}
		}
	}, Config{})

	return validate
}

// TODO cache control 은 나중에 구현하자
type Config struct {
	ResponseCodes        []int              `json:"response_code" validate:"required,gte=0" default:"[200, 301, 404]"`
	RequestMethods       []string           `json:"request_method" validate:"required" default:"[\"GET\", \"HEAD\"]"`
	ContentTypes         []string           `json:"content_type" validate:"required" default:"[\"text/plain\", \"application/json\", \"application/json; charset=utf-8\"]"`
	VaryHeaders          []string           `json:"vary_headers" validate:"required" default:"[]"`
	Filters              []Filter           `json:"filters" validate:"required" default:"[]"`
	CacheTTL             int                `json:"cache_ttl" validate:"gt=0" default:"300"`
	CacheControl         bool               `json:"cache_control" validate:"" default:"false"`
	CacheableBodyMaxSize int                `json:"cacheable_body_max_size" validate:"gte=0" default:"0"`
	CacheVersion         string             `json:"cache_version" validate:"" default:""`
	Strategy             string             `json:"strategy" validate:"required,oneof=redis redis-cluster in-memory" default:"redis"`
	Redis                RedisConfig        `json:"redis" default:"{}"`
	RedisCluster         RedisClusterConfig `json:"redis_cluster" default:"{}"`
	InMemory             InMemoryConfig     `json:"in_memory" default:"{}"`
	LogConf              LogConfig          `json:"log" validate:"" default:"{}"`
	FillLock             FillLockConfig     `json:"fill_lock" default:"{}"`

	logger *Logger `validate:"-"`
}

// FillLockConfig 는 스탬피드 fill-lock 설정이다(로드맵 U3).
// WaitMs가 0이면 패자는 기다리지 않고 바로 통과한다.
type FillLockConfig struct {
	Enabled bool `json:"enabled" default:"true"`
	TTLMs   int  `json:"ttl_ms" validate:"gte=0" default:"3000"`
	WaitMs  int  `json:"wait_ms" validate:"gte=0" default:"200"`
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
	fromEnv := os.Getenv("CACHE_VERSION")
	if fromEnv != "" {
		conf.logger.Info().Msgf("CACHE_VERSION: %s", fromEnv)
		return fromEnv
	}

	return Version
}

// See https://github.com/Kong/go-pdk/issues/78
func (conf *Config) Init() {
	// logger는 매 Config 인스턴스마다 개별적으로 초기화
	conf.logger = NewLogger(&conf.LogConf)

	// 필터 TTL 상속은 요청마다 요청 시점에 filtered()에서 해석한다(go-pdk #78 동시성 모델에서
	// 설정 슬라이스를 변이하면 데이터 경합이 된다).

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

func convertRedisTimeout(timeout int, timeUnit time.Duration) time.Duration {
	if timeout < -1 {
		return time.Duration(-1)
	} else if timeout <= 0 {
		return time.Duration(timeout)
	}

	return time.Duration(timeout) * timeUnit
}

// redisClusterKey 는 RedisClusterConfig에서 비교 가능한 공유 키를 만든다.
// Addrs []string은 맵 키로 쓸 수 없어 결합한다.
type redisClusterKey struct {
	addrs             string
	username          string
	password          string
	poolSize          int
	maxRetries        int
	minRetryBackoffMs int
	maxRetryBackoffMs int
	dialTimeout       int
	readTimeout       int
	writeTimeout      int
	poolTimeout       int
	idleTimeout       int
}

func newRedisClusterKey(c RedisClusterConfig) redisClusterKey {
	return redisClusterKey{
		addrs:             strings.Join(c.Addrs, ","),
		username:          c.Username,
		password:          c.Password,
		poolSize:          c.PoolSize,
		maxRetries:        c.MaxRetries,
		minRetryBackoffMs: c.MinRetryBackoffMs,
		maxRetryBackoffMs: c.MaxRetryBackoffMs,
		dialTimeout:       c.DialTimeout,
		readTimeout:       c.ReadTimeout,
		writeTimeout:      c.WriteTimeout,
		poolTimeout:       c.PoolTimeout,
		idleTimeout:       c.IdleTimeout,
	}
}

func newRedisClientOptions(c RedisConfig) *redis.Options {
	opts := &redis.Options{
		Addr:            c.Host + ":" + strconv.Itoa(c.Port),
		Username:        c.Username,
		Password:        c.Password,
		DB:              c.DBNumber,
		PoolSize:        c.PoolSize,
		MaxRetries:      c.MaxRetries,
		MinRetryBackoff: convertRedisTimeout(c.MinRetryBackoffMs, time.Millisecond),
		MaxRetryBackoff: convertRedisTimeout(c.MaxRetryBackoffMs, time.Millisecond),
		DialTimeout:     convertRedisTimeout(c.DialTimeout, time.Second),
		ReadTimeout:     convertRedisTimeout(c.ReadTimeout, time.Second),
		WriteTimeout:    convertRedisTimeout(c.WriteTimeout, time.Second),
		PoolTimeout:     convertRedisTimeout(c.PoolTimeout, time.Second),
		ConnMaxIdleTime: convertRedisTimeout(c.IdleTimeout, time.Second),
	}

	// CSC(실험, 로드맵 U4): RESP3 필요. DB 0 전용 제한은 라이브러리가
	// 경고와 함께 처리한다(이 클라이언트는 standalone 전용).
	if c.ClientSideCache.Enabled {
		opts.Protocol = 3
		opts.ClientSideCacheConfig = &redis.ClientSideCacheConfig{
			MaxEntries:     c.ClientSideCache.MaxEntries,
			MaxMemoryBytes: int64(c.ClientSideCache.MaxMemoryBytes),
			MaxStaleness:   time.Duration(c.ClientSideCache.MaxStalenessMs) * time.Millisecond,
		}
	}
	return opts
}

func newRedisClusterOptions(c RedisClusterConfig) *redis.ClusterOptions {
	return &redis.ClusterOptions{
		Addrs:           c.Addrs,
		Username:        c.Username,
		Password:        c.Password,
		PoolSize:        c.PoolSize,
		MaxRetries:      c.MaxRetries,
		MinRetryBackoff: convertRedisTimeout(c.MinRetryBackoffMs, time.Millisecond),
		MaxRetryBackoff: convertRedisTimeout(c.MaxRetryBackoffMs, time.Millisecond),
		DialTimeout:     convertRedisTimeout(c.DialTimeout, time.Second),
		ReadTimeout:     convertRedisTimeout(c.ReadTimeout, time.Second),
		WriteTimeout:    convertRedisTimeout(c.WriteTimeout, time.Second),
		PoolTimeout:     convertRedisTimeout(c.PoolTimeout, time.Second),
		ConnMaxIdleTime: convertRedisTimeout(c.IdleTimeout, time.Second),
	}
}

// sharedRedisClient 는 설정값을 키로 프로세스 수명 클라이언트를 공유한다
// (go-redis 문서: "Client is meant to be long-lived and shared").
func sharedRedisClient(c RedisConfig) *redis.Client {
	if v, ok := redisClients.Load(c); ok {
		return v.(*redis.Client)
	}
	actual, _ := redisClients.LoadOrStore(c, redis.NewClient(newRedisClientOptions(c)))
	return actual.(*redis.Client)
}

func sharedRedisClusterClient(c RedisClusterConfig) *redis.ClusterClient {
	key := newRedisClusterKey(c)
	if v, ok := redisClusterClients.Load(key); ok {
		return v.(*redis.ClusterClient)
	}
	actual, _ := redisClusterClients.LoadOrStore(key, redis.NewClusterClient(newRedisClusterOptions(c)))
	return actual.(*redis.ClusterClient)
}

func (conf *Config) newCacheManager(ttl int) (*cache.Cache[any], *marshaler.Marshaler, error) {
	switch conf.Strategy {
	case "redis":
		// 클라이언트는 공유, store는 요청마다 생성 — TTL이 store 옵션에 굽는다.
		redisClient := sharedRedisClient(conf.Redis)
		cacheStore := redis_store.NewRedis(redisClient, lib_store.WithExpiration(time.Duration(ttl)*time.Second))
		cacheManager := cache.New[any](cacheStore)
		marshal := marshaler.New(cacheManager)
		return cacheManager, marshal, nil

	case "redis-cluster":
		// 클라이언트는 공유, store는 요청마다 생성 — TTL이 store 옵션에 굽는다.
		redisClient := sharedRedisClusterClient(conf.RedisCluster)
		cacheStore := rediscluster_store.NewRedisCluster(redisClient, lib_store.WithExpiration(time.Duration(ttl)*time.Second))
		cacheManager := cache.New[any](cacheStore)
		marshal := marshaler.New(cacheManager)
		return cacheManager, marshal, nil

	case "in-memory":
		// 기존 캐시 스토어가 있는지 확인
		if existingStore, ok := cacheStores.Load(conf.InMemory); ok {
			cacheStore := existingStore.(lib_store.StoreInterface) // store.StoreInterface를 lib_store.StoreInterface로 변경
			cacheManager := cache.New[any](cacheStore)
			marshal := marshaler.New(cacheManager)
			return cacheManager, marshal, nil
		}

		// 새로운 캐시 생성
		config := &ristretto.Config[string, any]{
			MaxCost:     int64(conf.InMemory.MaxCost),
			NumCounters: int64(conf.InMemory.NumCounters),
			BufferItems: int64(conf.InMemory.BufferItems),
		}

		// LoadOrStore를 사용하여 동시성 안전하게 생성
		client, err := ristretto.NewCache[string, any](config)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create ristretto cache: %v", err)
		}

		actualClient, _ := ristrettoClients.LoadOrStore(conf.InMemory, client)
		cacheStore := ristretto_store.NewRistretto[string, any](
			actualClient.(*ristretto.Cache[string, any]),
			lib_store.WithExpiration(time.Duration(ttl)*time.Second),
		)

		// 생성된 store를 저장
		cacheStores.Store(conf.InMemory, cacheStore)

		cacheManager := cache.New[any](cacheStore)
		marshal := marshaler.New(cacheManager)
		return cacheManager, marshal, nil

	default:
		return nil, nil, fmt.Errorf("unknown cache strategy: %s", conf.Strategy)
	}
}

// panicGuard는 페이즈 핸들러 공통 패닉 방어 클로저를 반환한다 — go-pdk 서버에 recover가
// 없어 핸들러 패닉이 프로세스 사망으로 이어진다. recover()는 지연 함수 본문에서 직접 호출돼야 한다.
func panicGuard(kong *pdk.PDK) func() {
	return func() {
		if err := recover(); err != nil {
			log.Printf("kong plugin panic at: %+v, err: %+v", time.Now(), err)
			if kong == nil {
				log.Printf("kong fatal err ===> kong is nil at: %+v", time.Now())
			} else {
				_ = kong.Log.Err(fmt.Sprint(err))
			}
		}
	}
}

func (conf *Config) Access(kong *pdk.PDK) {
	defer panicGuard(kong)()

	conf.Init()
	defer conf.Close() //nolint directives: gosimple

	logger := conf.logger

	method, err := kong.Request.GetMethod()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get request method")
		return
	}

	uri, err := kong.Request.GetPath()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get request path")
		return
	}

	var span trace.Span

	if otelEnabled {
		ctx := context.Background()
		_, span = tracer.Start(ctx, "sonic-boom.Access",
			trace.WithAttributes(
				attribute.String("http.method", method),
				attribute.String("http.url", uri),
			),
		)
		defer span.End()

		// Store span context in PDK context for later use in Response
		if err := SetPluginEx(kong, "span_context", span.SpanContext()); err != nil {
			logger.Error().Err(err).Msg("Failed to store span context")
		}
	}

	if err := conf.checkConfig(); err != nil {
		logger.Error().Err(err).Msg("Config check failed; bypassing")
		if err := kong.Response.SetHeader("X-Cache-Status", "Bypass"); err != nil {
			logger.Error().Err(err).Msg("SetHeader failed")
		}
		return
	}

	if conf.isDebug() {
		if err := kong.Response.SetHeader("X-sonic-boom-Plugin-Version", Version); err != nil {
			logger.Warn().Err(err).Msg("failed to set header")
		}
	}

	cacheable, cacheTTL := conf.cacheableRequest(kong)
	if !cacheable {
		if err := kong.Response.SetHeader("X-Cache-Status", "Bypass"); err != nil {
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

	if err := kong.Response.SetHeader("X-Cache-Key", cacheKeyID); err != nil {
		logger.Debug().Err(err).Msg("Failed to set header")
		//_ = log.Err("SetHeader failed: ", err.Error())
	}

	cm, marshal, err := conf.newCacheManager(cacheTTL)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create cache manager")
		return
	}

	lookupStart := time.Now()
	cached, err := marshal.Get(context.Background(), cacheKeyID, new(CacheValue))
	recordCacheLookup(conf.Strategy, time.Since(lookupStart))
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

		// 스탬피드 fill-lock: 승자(nil)는 프록시로 fill, 패자는 기한 안에
		// 채워진 값을 받아 히트로 서빙하거나 통과한다.
		if v := conf.fillLockOrWait(cm, marshal, cacheKeyID); v != nil {
			cached = v
		} else {
			err = conf.signalCacheReq(kong, CacheSignal{cacheKeyID, cacheTTL})
			if err != nil {
				logger.Error().Err(err).Msg("Failed to signal cache request")
				return
			}
			return
		}
	} else {
		logger.Debug().Msg("Cache hit")
	}

	cacheValue := cached.(*CacheValue)
	cacheSignal := CacheSignal{
		CacheKeyID: cacheKeyID,
		CacheTTL:   cacheTTL,
	}

	if cacheValue.Version != conf.CacheVersion {
		logger.Warn().Msgf("Cache version mismatch, purging: %s != %s", cacheValue.Version, conf.CacheVersion)
		if err := marshal.Delete(context.Background(), cacheKeyID); err != nil {
			logger.Error().Err(err).Msg("Purging cache failed")
			return
		}
		if err := conf.signalCacheReqWithStatus(kong, cacheSignal, "Bypass"); err != nil {
			logger.Error().Err(err).Msg("Failed to signal cache request")
			return
		}
		// 낡은 값을 서빙하지 않고 upstream으로 통과한다(원본 Lua `return signal_cache_req(...)` 시맨틱).
		return
	}

	if conf.CacheControl {
		logger.Error().Msg("Cache control is enabled but not implemented yet; bypassing")
		if err := kong.Response.SetHeader("X-Cache-Status", "Bypass"); err != nil {
			logger.Error().Err(err).Msg("SetHeader failed")
		}
		return
	}

	// don't serve stale data; res may be stored for up to `conf.storage_ttl` secs
	now := time.Now()
	secs := now.Unix()

	if (secs - cacheValue.Timestamp) > int64(cacheTTL) {
		if err := conf.signalCacheReqWithStatus(kong, cacheSignal, "Refresh"); err != nil {
			logger.Error().Err(err).Msg("Failed to signal cache request")
			return
		}
		// 만료된 값은 재검증을 위해 upstream으로 통과한다(원본 Lua `return signal_cache_req(...)` 시맨틱).
		return
	}

	// we have cache data yo!
	if err := kong.Nginx.SetCtx("KONG_PROXIED", true); err != nil {
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

	age := strconv.FormatInt(secs-cacheValue.Timestamp, 10)
	cacheValue.Headers["Age"] = []string{age}
	cacheValue.Headers["X-Cache-Status"] = []string{"Hit"}
	recordCacheStatus(conf.Strategy, "Hit")

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
			ttl := filter.CacheTTL
			if ttl == 0 {
				ttl = conf.CacheTTL
			}
			return true, ttl
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
	validate := configValidator

	return validate.Struct(conf)
}

func (conf *Config) signalCacheReq(kong *pdk.PDK, signal CacheSignal) error {
	return conf.signalCacheReqWithStatus(kong, signal, "")
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
	recordCacheStatus(conf.Strategy, cacheStatus)

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
		return nil, fmt.Errorf("%s", x.Error)

	default:
		return out.GetContent(), nil
	}
}

func (conf *Config) Response(kong *pdk.PDK) {
	defer panicGuard(kong)()

	conf.Init()
	defer conf.Close() //nolint directives: gosimple

	logger := conf.logger

	var span trace.Span

	if otelEnabled {
		// Get the parent span context
		spanCtxRaw, err := GetPluginAny(kong, "span_context")
		if err != nil {
			logger.Error().Err(err).Msg("Failed to get span context")
			return
		}
		spanCtx, ok := spanCtxRaw.(trace.SpanContext)
		if !ok {
			logger.Error().Msg("Invalid span context type")
			return
		}

		ctx := context.Background()
		ctx = trace.ContextWithSpanContext(ctx, spanCtx)
		_, span = tracer.Start(ctx, "sonic-boom.Response")
		defer span.End()
	}

	httpStatus, err := kong.Response.GetStatus()
	if err != nil {
		logger.Error().Err(err).Msg("Getting response status failed")
		return
	}

	if otelEnabled {
		span.SetAttributes(
			attribute.Int("http.status_code", httpStatus),
		)
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
		if err := kong.Response.SetHeader("X-Cache-Status", "Bypass"); err != nil {
			logger.Error().Err(err).Msg("Setting header `X-Cache-Status` failed")
			return
		}
		return
	}

	// ProxyCacheHandler:body_filter
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
	}
	if conf.CacheableBodyMaxSize > 0 && len(rawBody) > conf.CacheableBodyMaxSize {
		logger.Debug().Msgf("Body length is bigger than allowed body_max_size: %d", conf.CacheableBodyMaxSize)
		return
	}

	now := time.Now()
	secs := now.Unix()

	ttl := cacheSignal.CacheTTL
	cacheValue := &CacheValue{
		Status:    httpStatus,
		Headers:   headers,
		Body:      rawBody,
		BodyLen:   len(rawBody),
		Timestamp: secs,
		TTL:       int64(ttl),
		Version:   conf.CacheVersion,
	}
	logger.Debug().Msgf("cacheValue: %+v", cacheValue)

	cm, marshal, err := conf.newCacheManager(ttl)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create cache manager")
		return
	}

	cacheKeyID := cacheSignal.CacheKeyID
	if err := marshal.Set(context.Background(), cacheKeyID, cacheValue, lib_store.WithExpiration(time.Duration(ttl)*time.Second)); err != nil {
		logger.Error().Err(err).Msg("Cache set failed")
		return
	}

	// fill 완료 — fill-lock 대기자들을 즉시 풀어준다(TTL이 상한).
	releaseFillLock(cm, cacheKeyID)
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

	return false
}
