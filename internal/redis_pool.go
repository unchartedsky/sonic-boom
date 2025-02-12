package internal

import (
	"context"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"

	redis_store "github.com/eko/gocache/store/redis/v4"
	pool "github.com/jolestar/go-commons-pool/v2"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"runtime"
	"strconv"
	"time"
)

func NewRedisPoolConfig() *pool.ObjectPoolConfig {
	maxProcs := runtime.GOMAXPROCS(-1)

	config := pool.NewDefaultPoolConfig()
	config.MaxTotal = maxProcs * 3
	config.MaxIdle = config.MaxTotal
	config.MinIdle = maxProcs
	return config
}

type RedisPool struct {
	pool *pool.ObjectPool
}

func NewRedisPool(ctx context.Context, config RedisConfig) (*RedisPool, error) {
	p := pool.NewObjectPool(ctx, NewRedisMarshalerFactory(config), NewRedisPoolConfig())
	if p == nil {
		return nil, errors.New("Failed to create redis pool")
	}

	return &RedisPool{
		pool: p,
	}, nil
}

func (r *RedisPool) BorrowObject(ctx context.Context) (*RedisMarshaler, error) {
	pooled, err := r.pool.BorrowObject(ctx)
	if err != nil {
		return nil, err
	}

	obj, ok := pooled.(*RedisMarshaler)
	if !ok {
		return nil, errors.New("Failed to get a pooled object: ")
	}
	return obj, nil
}

func (r *RedisPool) ReturnObject(ctx context.Context, obj *RedisMarshaler) error {
	return r.pool.ReturnObject(ctx, obj)
}

func convertRedisTimeout(timeout int, timeUnit time.Duration) time.Duration {
	if timeout < -1 {
		return time.Duration(-1)
	} else if timeout <= 0 {
		return time.Duration(timeout)
	}

	return time.Duration(timeout) * timeUnit
}

func newRedisClient(conf *RedisConfig) *redis.Client {
	if conf.PoolTimeout > 0 && conf.PoolTimeout <= conf.ReadTimeout {
		conf.PoolTimeout = conf.ReadTimeout + 1
	}

	opts := redis.Options{
		Addr:            conf.Host + ":" + strconv.Itoa(conf.Port),
		DB:              conf.DBNumber,
		PoolSize:        conf.PoolSize,
		MaxRetries:      conf.MaxRetries,
		MinRetryBackoff: convertRedisTimeout(conf.MinRetryBackoffMs, time.Millisecond),
		MaxRetryBackoff: convertRedisTimeout(conf.MaxRetryBackoffMs, time.Millisecond),
		DialTimeout:     convertRedisTimeout(conf.DialTimeout, time.Second),
		ReadTimeout:     convertRedisTimeout(conf.ReadTimeout, time.Second),
		WriteTimeout:    convertRedisTimeout(conf.WriteTimeout, time.Second),
		PoolTimeout:     convertRedisTimeout(conf.PoolTimeout, time.Second),
		ConnMaxIdleTime: convertRedisTimeout(conf.IdleTimeout, time.Second),
	}

	return redis.NewClient(&opts)
}

type RedisMarshaler struct {
	Marshaler *marshaler.Marshaler
	client    redis.UniversalClient
}

func (obj *RedisMarshaler) Close() error {
	if obj.client == nil {
		return nil
	}
	return obj.client.Close()
}

type RedisMarshalerFactory struct {
	conf RedisConfig
}

func NewRedisMarshalerFactory(conf RedisConfig) *RedisMarshalerFactory {
	return &RedisMarshalerFactory{
		conf: conf,
	}
}

func (f *RedisMarshalerFactory) MakeObject(ctx context.Context) (*pool.PooledObject, error) {
	redisClient := newRedisClient(&f.conf)
	if redisClient == nil {
		return nil, errors.New("Failed to create redis client")
	}

	redisStore := redis_store.NewRedis(redisClient)
	cacheManager := cache.New[any](redisStore)
	marshal := marshaler.New(cacheManager)

	return pool.NewPooledObject(
			&RedisMarshaler{
				Marshaler: marshal,
				client:    redisClient,
			}),
		nil
}

func (f *RedisMarshalerFactory) DestroyObject(ctx context.Context, object *pool.PooledObject) error {
	if obj, ok := object.Object.(*RedisMarshaler); ok {
		return obj.Close()
	}
	return nil
}

func (f *RedisMarshalerFactory) ValidateObject(ctx context.Context, object *pool.PooledObject) bool {
	// do validate
	return true
}

func (f *RedisMarshalerFactory) ActivateObject(ctx context.Context, object *pool.PooledObject) error {
	// do activate
	return nil
}

func (f *RedisMarshalerFactory) PassivateObject(ctx context.Context, object *pool.PooledObject) error {
	// do passivate
	return nil
}
