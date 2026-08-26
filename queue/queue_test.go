package queue_test

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"apigo.cc/go/redis"
	"apigo.cc/go/task/queue"
)

type queueItem struct {
	ID   int
	Text string
}

func TestMain(m *testing.M) {
	redisURL := os.Getenv("TASK_QUEUE_TEST_REDIS")
	if redisURL == "" {
		redisURL = "redis://:@localhost:6379/14?connectTimeout=100ms&readTimeout=100ms&writeTimeout=100ms"
	}
	parsedURL, err := url.Parse(redisURL)
	if err != nil {
		fmt.Printf("Invalid TASK_QUEUE_TEST_REDIS: %v\n", err)
		os.Exit(1)
	}
	conn, err := net.DialTimeout("tcp", parsedURL.Host, 500*time.Millisecond)
	if err != nil {
		fmt.Printf("Redis server is not running at %s, skipping queue tests.\n", parsedURL.Host)
		os.Exit(0)
	}
	_ = conn.Close()

	redis.SetConfig("task", redisURL)
	os.Exit(m.Run())
}

func TestAddFetchFinish(t *testing.T) {
	queueName := testQueueName(t)
	cleanupQueue(t, queueName, "default")

	want := queueItem{ID: 1, Text: "first"}
	if err := queue.Add(queueName, want); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	items, err := queue.Fetch(queueName, "default", 10)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Fetch returned %d items, want 1", len(items))
	}
	rawProcessing := redis.GetRedis("task", nil).LRANGE("queue:"+queueName+":default:processing", 0, -1)
	if len(rawProcessing) != 1 {
		t.Fatalf("processing contains %d raw items, want only the fetched task", len(rawProcessing))
	}
	if rawProcessing[0].String() == "\x00apigo-task-processing\x00" {
		t.Fatal("processing retained its compatibility marker")
	}
	got, err := queue.Decode[queueItem](items[0])
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if got != want {
		t.Fatalf("Fetch returned %+v, want %+v", got, want)
	}
	if count := queue.FetchCount(queueName, "default"); count != 1 {
		t.Fatalf("FetchCount returned %d while processing, want 1", count)
	}

	if err := queue.Finish(queueName, "default"); err != nil {
		t.Fatalf("Finish failed: %v", err)
	}
	if count := queue.FetchCount(queueName, "default"); count != 0 {
		t.Fatalf("FetchCount returned %d after Finish, want 0", count)
	}
}

func TestFetchRecoversProcessingBeforePending(t *testing.T) {
	queueName := testQueueName(t)
	cleanupQueue(t, queueName, "default")

	processingItem := queueItem{ID: 1, Text: "processing"}
	if err := queue.Add(queueName, processingItem); err != nil {
		t.Fatalf("Add processing item failed: %v", err)
	}
	firstFetch, err := queue.Fetch(queueName, "default", 1)
	if err != nil {
		t.Fatalf("initial Fetch failed: %v", err)
	}
	if len(firstFetch) != 1 {
		t.Fatalf("initial Fetch returned %d items, want 1", len(firstFetch))
	}

	pendingItem := queueItem{ID: 2, Text: "pending"}
	if err := queue.Add(queueName, pendingItem); err != nil {
		t.Fatalf("Add pending item failed: %v", err)
	}
	recovered, err := queue.Fetch(queueName, "default", 50)
	if err != nil {
		t.Fatalf("recovery Fetch failed: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovery Fetch returned %d items, want the existing processing batch only", len(recovered))
	}
	got, err := queue.Decode[queueItem](recovered[0])
	if err != nil {
		t.Fatalf("Decode recovered item failed: %v", err)
	}
	if got != processingItem {
		t.Fatalf("recovery Fetch returned %+v, want %+v", got, processingItem)
	}

	if err := queue.Finish(queueName, "default"); err != nil {
		t.Fatalf("Finish recovered batch failed: %v", err)
	}
	next, err := queue.Fetch(queueName, "default", 1)
	if err != nil {
		t.Fatalf("Fetch pending item failed: %v", err)
	}
	if len(next) != 1 {
		t.Fatalf("Fetch pending item returned %d items, want 1", len(next))
	}
	got, err = queue.Decode[queueItem](next[0])
	if err != nil {
		t.Fatalf("Decode pending item failed: %v", err)
	}
	if got != pendingItem {
		t.Fatalf("Fetch after Finish returned %+v, want %+v", got, pendingItem)
	}
}

func TestAddAfterPromotesOnlyDueItems(t *testing.T) {
	queueName := testQueueName(t)
	cleanupQueue(t, queueName, "default")

	due := queueItem{ID: 1, Text: "due"}
	future := queueItem{ID: 2, Text: "future"}
	if err := queue.AddAfter(queueName, future, time.Hour); err != nil {
		t.Fatalf("AddAfter future item failed: %v", err)
	}
	if err := queue.AddAfter(queueName, due, -time.Millisecond); err != nil {
		t.Fatalf("AddAfter due item failed: %v", err)
	}

	items, err := queue.Fetch(queueName, "default", 10)
	if err != nil {
		t.Fatalf("Fetch delayed item failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Fetch returned %d delayed items, want only the due item", len(items))
	}
	got, err := queue.Decode[queueItem](items[0])
	if err != nil {
		t.Fatalf("Decode delayed item failed: %v", err)
	}
	if got != due {
		t.Fatalf("Fetch returned delayed item %+v, want %+v", got, due)
	}

	rd := redis.GetRedis("task", nil)
	if count := rd.Do("ZCARD", delayedKey(queueName)).Int(); count != 1 {
		t.Fatalf("delayed queue contains %d items after promotion, want the future item only", count)
	}
}

func testQueueName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("task-queue-test:%d:%s", time.Now().UnixNano(), t.Name())
}

func cleanupQueue(t *testing.T, queueName, consumerID string) {
	t.Helper()
	rd := redis.GetRedis("task", nil)
	if rd == nil || rd.Error != nil {
		t.Fatalf("task Redis is unavailable: %v", rd.Error)
	}
	keys := []string{
		"queue:" + queueName + ":pending",
		"queue:" + queueName + ":" + consumerID + ":processing",
		delayedKey(queueName),
	}
	if result := rd.Do("DEL", keys); result.Error != nil {
		t.Fatalf("clean queue before test: %v", result.Error)
	}
	t.Cleanup(func() {
		_ = rd.Do("DEL", keys)
	})
}

func delayedKey(queueName string) string {
	return "queue:" + queueName + ":delayed"
}
