package internal

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func Benchmark_InMemory_Set(b *testing.B) {
	cfg := newInMemoryConfigForTest()
	cm, _, err := cfg.newCacheManager(60)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i)
			val := []byte(fmt.Sprintf("bench-value-%d", i))
			// Ristretto 비동기 쓰기 특성으로 인한 에러는 무시
			_ = cm.Set(context.Background(), key, val)
			i++
		}
	})
}

func Benchmark_InMemory_Get(b *testing.B) {
	cfg := newInMemoryConfigForTest()
	cm, _, err := cfg.newCacheManager(60)
	if err != nil {
		b.Fatal(err)
	}

	// 사전 데이터 설정
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		val := []byte(fmt.Sprintf("bench-value-%d", i))
		err := cm.Set(context.Background(), key, val)
		if err != nil {
			b.Fatal(err)
		}
	}

	// 비동기 쓰기 완료 대기
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i%1000)
			_, err := cm.Get(context.Background(), key)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

func Benchmark_InMemory_SetGet(b *testing.B) {
	cfg := newInMemoryConfigForTest()
	cm, _, err := cfg.newCacheManager(60)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i)
			val := []byte(fmt.Sprintf("bench-value-%d", i))

			// Set (에러 무시)
			_ = cm.Set(context.Background(), key, val)

			// Get (에러 무시)
			_, _ = cm.Get(context.Background(), key)
			i++
		}
	})
}

func Benchmark_InMemory_Marshal_Set(b *testing.B) {
	cfg := newInMemoryConfigForTest()
	_, m, err := cfg.newCacheManager(60)
	if err != nil {
		b.Fatal(err)
	}

	cv := CacheValue{
		Status:    200,
		Headers:   map[string][]string{"Content-Type": {"application/json"}},
		Body:      []byte(`{"message":"benchmark test"}`),
		BodyLen:   25,
		Timestamp: time.Now().Unix(),
		TTL:       60,
		Version:   "1.0",
		ReqBody:   []byte{},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-marshal-key-%d", i)
			// 에러 무시
			_ = m.Set(context.Background(), key, cv)
			i++
		}
	})
}

func Benchmark_InMemory_Marshal_Get(b *testing.B) {
	cfg := newInMemoryConfigForTest()
	_, m, err := cfg.newCacheManager(60)
	if err != nil {
		b.Fatal(err)
	}

	cv := CacheValue{
		Status:    200,
		Headers:   map[string][]string{"Content-Type": {"application/json"}},
		Body:      []byte(`{"message":"benchmark test"}`),
		BodyLen:   25,
		Timestamp: time.Now().Unix(),
		TTL:       60,
		Version:   "1.0",
		ReqBody:   []byte{},
	}

	// 사전 데이터 설정
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench-marshal-key-%d", i)
		err := m.Set(context.Background(), key, cv)
		if err != nil {
			b.Fatal(err)
		}
	}

	// 비동기 쓰기 완료 대기
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-marshal-key-%d", i%1000)
			var out CacheValue
			_, err := m.Get(context.Background(), key, &out)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

func Benchmark_InMemory_Eviction(b *testing.B) {
	cfg := newInMemoryConfigForTest()
	cfg.InMemory.MaxCost = 1024 // 작은 캐시 크기로 제한
	cm, _, err := cfg.newCacheManager(60)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("eviction-key-%d", i)
		val := []byte("0123456789") // 10바이트 값
		// 에러 무시
		_ = cm.Set(context.Background(), key, val)
	}
}

func Benchmark_InMemory_Concurrent(b *testing.B) {
	cfg := newInMemoryConfigForTest()
	cm, _, err := cfg.newCacheManager(60)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("concurrent-key-%d", i)
			val := []byte(fmt.Sprintf("concurrent-value-%d", i))

			// 50% 확률로 Set 또는 Get
			if i%2 == 0 {
				// Set 에러 무시
				_ = cm.Set(context.Background(), key, val)
			} else {
				// Get 에러 무시
				_, _ = cm.Get(context.Background(), key)
			}
			i++
		}
	})
}
