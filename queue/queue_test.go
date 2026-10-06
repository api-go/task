package queue_test

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
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

func TestRetryAndFailRejectBatchAcknowledgement(t *testing.T) {
	name := testQueueName(t)
	cleanupQueue(t, name, "default")
	for id := 1; id <= 2; id++ {
		if err := queue.Add(name, queueItem{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if items, err := queue.Fetch(name, "default", 2); err != nil || len(items) != 2 {
		t.Fatalf("batch fetch: %d, %v", len(items), err)
	}
	if err := queue.Retry(name, "default", queueItem{ID: 1}, 0); err == nil {
		t.Fatal("single-item retry acknowledged a batch")
	}
	if err := queue.Fail(name, "default", queueItem{ID: 1}); err == nil {
		t.Fatal("single-item failure acknowledged a batch")
	}
	stats, err := queue.Stats(name, "default")
	if err != nil || stats["processing"] != 2 || stats["delayed"] != 0 || stats["failed"] != 0 {
		t.Fatalf("rejected operation modified the batch: %#v, %v", stats, err)
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

func TestFetchDecodedQuarantinesMalformedMixedBatch(t *testing.T) {
	name := testQueueName(t)
	cleanupQueue(t, name, "default")
	cleanupQueue(t, name+":failed", "review")
	if err := queue.Add(name, queueItem{ID: 2, Text: "valid"}); err != nil {
		t.Fatal(err)
	}
	rd := redis.GetRedis("task", nil)
	if result := rd.Do("LPUSH", "queue:"+name+":pending", "{invalid JSON"); result.Error != nil {
		t.Fatal(result.Error)
	}
	if _, err := queue.FetchDecoded(name, "default", 10); err == nil {
		t.Fatal("malformed batch must report its quarantine")
	}
	stats, err := queue.Stats(name, "default")
	if err != nil || stats["pending"] != 1 || stats["processing"] != 0 || stats["failed"] != 1 {
		t.Fatalf("mixed batch recovery lost tasks: %#v, %v", stats, err)
	}
	items, err := queue.FetchDecoded(name, "default", 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("valid task did not recover: %#v, %v", items, err)
	}
}

func TestRetryAndTerminalFailureRetainTasks(t *testing.T) {
	name := testQueueName(t)
	cleanupQueue(t, name, "default")
	cleanupQueue(t, name+":failed", "review")
	first := queueItem{ID: 1, Text: "first attempt"}
	if err := queue.Add(name, first); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Fetch(name, "default", 1); err != nil {
		t.Fatal(err)
	}
	if err := queue.Retry(name, "default", first, -1); err == nil {
		t.Fatal("invalid delay accepted")
	}
	if queue.FetchCount(name, "default") != 1 {
		t.Fatal("invalid retry acknowledged the processing task")
	}
	retried := queueItem{ID: 1, Text: "second attempt"}
	if err := queue.Retry(name, "default", retried, 0); err != nil {
		t.Fatal(err)
	}
	items, err := queue.Fetch(name, "default", 1)
	if err != nil || len(items) != 1 {
		t.Fatalf("retry not recovered: %v, %v", items, err)
	}
	got, err := queue.Decode[queueItem](items[0])
	if err != nil || got != retried {
		t.Fatalf("retry payload changed: %+v, %v", got, err)
	}
	if err := queue.Fail(name, "default", retried); err != nil {
		t.Fatal(err)
	}
	stats, err := queue.Stats(name, "default")
	if err != nil || stats["failed"] != 1 || stats["pending"] != 0 || stats["processing"] != 0 || stats["delayed"] != 0 {
		t.Fatalf("terminal failure was not visible: %#v, %v", stats, err)
	}
	if queue.FetchCount(name, "default") != 0 {
		t.Fatal("terminal failure remained in the worker queue")
	}
	items, err = queue.Fetch(name+":failed", "review", 1)
	if err != nil || len(items) != 1 {
		t.Fatalf("terminal failure lost: %v, %v", items, err)
	}
	got, err = queue.Decode[queueItem](items[0])
	if err != nil || got != retried {
		t.Fatalf("terminal payload changed: %+v, %v", got, err)
	}
}

func TestPeekFailedDoesNotConsumeTerminalItems(t *testing.T) {
	name := testQueueName(t)
	cleanupQueue(t, name+":failed", "review")
	if err := queue.Add(name+":failed", map[string]any{"id": "one"}); err != nil {
		t.Fatal(err)
	}
	items, err := queue.PeekFailed(name, 0, 1)
	if err != nil || len(items) != 1 || !strings.Contains(items[0], "one") {
		t.Fatalf("failed page = %#v, %v", items, err)
	}
	items, err = queue.PeekFailed(name, 1, 1)
	if err != nil || len(items) != 0 {
		t.Fatalf("second page = %#v, %v", items, err)
	}
	if count, err := queue.Stats(name, "default"); err != nil || count["failed"] != 1 {
		t.Fatalf("peek consumed failed item: %#v, %v", count, err)
	}
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
