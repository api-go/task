package queue

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"apigo.cc/go/cast"
	"apigo.cc/go/redis"
)

var delayedSequence uint64

const processingPlaceholder = "\x00apigo-task-processing\x00"

func pendingKey(name string) string { return "queue:" + name + ":pending" }
func processingKey(name, consumerID string) string {
	return "queue:" + name + ":" + strings.TrimSpace(consumerID) + ":processing"
}
func client() (*redis.Redis, error) {
	rd := redis.GetRedis("task", nil)
	if rd == nil {
		return nil, fmt.Errorf("task redis is unavailable")
	}
	if rd.Error != nil {
		return nil, rd.Error
	}
	return rd, nil
}

func Add(queueName string, item any) error {
	rd, err := client()
	if err != nil {
		return err
	}
	r := rd.Do("LPUSH", pendingKey(queueName), item)
	return r.Error
}

// AddAfter stores an item in the queue's persistent delayed lane until delay elapses.
func AddAfter(queueName string, item any, delay time.Duration) error {
	rd, err := client()
	if err != nil {
		return err
	}
	payload, err := cast.ToJSON(item)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("%d:%d", time.Now().UnixNano(), atomic.AddUint64(&delayedSequence, 1))
	member := id + "\x00" + payload
	r := rd.Do("ZADD", delayedKey(queueName), float64(time.Now().Add(delay).UnixMilli()), member)
	return r.Error
}

func delayedKey(name string) string { return "queue:" + name + ":delayed" }

// PromoteDelayed moves due items into pending. Queue workers call this before Fetch.
// The current task runtime has one consumer per queue. Pending is written
// before delayed is removed so a crash can only result in a duplicate, never
// a lost task; consumers already have retry/idempotency safeguards.
func PromoteDelayed(queueName string, limit int) error {
	if limit < 1 {
		limit = 100
	}
	rd, err := client()
	if err != nil {
		return err
	}
	items, err := rd.ZRangeByScore(delayedKey(queueName), "-inf", time.Now().UnixMilli(), 0, limit)
	if err != nil {
		return err
	}
	for _, result := range items {
		member := result.String()
		parts := strings.SplitN(member, "\x00", 2)
		if len(parts) != 2 {
			_ = rd.ZREM(delayedKey(queueName), member)
			continue
		}
		_ = rd.Do("LPUSH", pendingKey(queueName), parts[1])
		_ = rd.ZREM(delayedKey(queueName), member)
	}
	return rd.Error
}

func Fetch(queueName, consumerID string, n int) ([]redis.Result, error) {
	if n < 1 {
		return []redis.Result{}, nil
	}
	rd, err := client()
	if err != nil {
		return nil, err
	}
	if err := PromoteDelayed(queueName, n); err != nil {
		return nil, err
	}
	processing := processingKey(queueName, consumerID)
	if items := processingItems(rd, processing); len(items) > 0 {
		return items, nil
	}
	for i := 0; i < n; i++ {
		if rd.LLEN(pendingKey(queueName)) == 0 {
			break
		}
		if rd.LLEN(processing) == 0 {
			// SugarDB requires the LMOVE destination to exist, while standard Redis
			// creates it. Pushing and immediately popping a marker initializes an
			// empty SugarDB list without retaining application-visible data.
			if r := rd.Do("LPUSH", processing, processingPlaceholder); r.Error != nil {
				return nil, r.Error
			}
			if r := rd.Do("LPOP", processing); r.Error != nil {
				return nil, r.Error
			}
		}
		result := rd.Do("LMOVE", pendingKey(queueName), processing, "RIGHT", "LEFT")
		if result.Error != nil {
			return nil, result.Error
		}
		if strings.TrimSpace(result.String()) == "" {
			break
		}
	}
	return processingItems(rd, processing), nil
}

func processingItems(rd *redis.Redis, key string) []redis.Result {
	items := rd.LRANGE(key, 0, -1)
	filtered := items[:0]
	for _, item := range items {
		if item.String() != processingPlaceholder {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func Finish(queueName, consumerID string) error {
	rd, err := client()
	if err != nil {
		return err
	}
	r := rd.Do("DEL", processingKey(queueName, consumerID))
	return r.Error
}

func FetchCount(queueName, consumerID string) int {
	_ = PromoteDelayed(queueName, 100)
	rd, err := client()
	if err != nil {
		return 0
	}
	processing := processingKey(queueName, consumerID)
	if count := len(processingItems(rd, processing)); count > 0 {
		return count
	}
	return rd.LLEN(pendingKey(queueName))
}

func Decode[T any](result redis.Result) (T, error) {
	var item T
	if err := cast.UnmarshalJSON(result.Bytes(), &item); err != nil {
		return item, fmt.Errorf("decode queue item: %w", err)
	}
	return item, nil
}
