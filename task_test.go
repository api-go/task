package task_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/api-go/task"
	"os"
)

func TestMain(m *testing.M) {
	task.DefaultScheduler.Start(context.Background(), nil)
	os.Exit(m.Run())
}

func TestTaskBasic(t *testing.T) {
	var count int32
	task.Add("test-basic", "@every 1s", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	})
	time.Sleep(2500 * time.Millisecond)

	if atomic.LoadInt32(&count) < 2 {
		t.Errorf("Expected at least 2 runs, got %d", count)
	}
	task.Remove("test-basic")
}

func TestTaskLifecycle(t *testing.T) {
	var count int32
	name := "test-lifecycle"
	tk := task.Add(name, "@every 1s", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	tk.Disable()
	time.Sleep(1500 * time.Millisecond)
	if atomic.LoadInt32(&count) != 0 {
		t.Errorf("Expected 0 runs while disabled, got %d", count)
	}

	tasks := task.List()
	found := false
	for _, item := range tasks {
		if item.Name == name {
			found = true
			if item.Status != task.StatusDisabled {
				t.Errorf("Expected status disabled, got %s", item.Status)
			}
		}
	}
	if !found {
		t.Errorf("Task not found in list")
	}

	tk.Enable()
	time.Sleep(2500 * time.Millisecond)
	if atomic.LoadInt32(&count) == 0 {
		t.Errorf("Expected task to run after enabling")
	}

	tk.Remove()
	if task.Get(name) != nil {
		t.Errorf("Expected task to be nil after remove")
	}
}

func TestTaskPolicySkip(t *testing.T) {
	var count int32
	task.Add("test-skip", "@every 1s", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		time.Sleep(1500 * time.Millisecond)
		return nil
	}, task.Config{Policy: task.PolicySkip})

	time.Sleep(3500 * time.Millisecond)

	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("Expected exactly 2 runs due to skip policy, got %d", count)
	}
	task.Remove("test-skip")
}

func TestTaskTimeout(t *testing.T) {
	var timeoutHit int32
	task.Add("test-timeout", "@every 1s", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				atomic.AddInt32(&timeoutHit, 1)
			}
			return ctx.Err()
		case <-time.After(2 * time.Second):
			return nil
		}
	}, task.Config{Timeout: 500 * time.Millisecond})

	time.Sleep(2500 * time.Millisecond)

	if atomic.LoadInt32(&timeoutHit) < 2 {
		t.Errorf("Expected at least 2 timeout hits, got %d", timeoutHit)
	}
	task.Remove("test-timeout")
}

func TestTaskPanicRecover(t *testing.T) {
	var count int32
	task.Add("test-panic", "@every 1s", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		panic("boom")
	})

	time.Sleep(2500 * time.Millisecond)

	if atomic.LoadInt32(&count) < 2 {
		t.Errorf("Expected at least 2 runs despite panics, got %d", count)
	}
	task.Remove("test-panic")
}

func TestTaskManualRun(t *testing.T) {
	var count int32
	name := "test-manual"
	tk := task.Add(name, "@every 1h", func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	// Manual run via global function
	task.Run(name)
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("Expected 1 run after manual Run, got %d", count)
	}

	// Manual run via task object
	tk.Run()
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("Expected 2 runs after manual Run, got %d", count)
	}
	task.Remove(name)
}

func TestTaskAutoDisablesAfterConsecutiveFailures(t *testing.T) {
	s := task.NewScheduler()
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())

	disabled := make(chan struct{}, 1)
	tk := s.Add("test-auto-disable", "@every 1h", func(context.Context) error {
		return errors.New("boom")
	}, task.Config{
		MaxConsecutiveFailures: 3,
		OnAutoDisable: func(name string, failures int, err error) {
			if name != "test-auto-disable" || failures != 3 || err == nil {
				t.Errorf("unexpected auto-disable callback: name=%s failures=%d err=%v", name, failures, err)
			}
			disabled <- struct{}{}
		},
	})

	for i := 0; i < 3; i++ {
		s.Run(tk.Name)
		deadline := time.Now().Add(time.Second)
		_, failures := tk.State()
		for failures < i+1 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
			_, failures = tk.State()
		}
	}
	select {
	case <-disabled:
	case <-time.After(time.Second):
		t.Fatal("auto-disable callback was not called")
	}
	status, failures := tk.State()
	if status != task.StatusDisabled || failures != 3 {
		t.Fatalf("task status=%s failures=%d, want disabled after 3 failures", status, failures)
	}
	tk.Enable()
	status, failures = tk.State()
	if status != task.StatusIdle || failures != 0 {
		t.Fatalf("enabled task status=%s failures=%d, want idle with reset failures", status, failures)
	}
}

func TestTaskSuccessResetsConsecutiveFailures(t *testing.T) {
	s := task.NewScheduler()
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())

	var calls atomic.Int32
	tk := s.Add("test-failure-reset", "@every 1h", func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("boom")
		}
		return nil
	}, task.Config{MaxConsecutiveFailures: 2})

	s.Run(tk.Name)
	deadline := time.Now().Add(time.Second)
	_, failures := tk.State()
	for failures != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		_, failures = tk.State()
	}
	s.Run(tk.Name)
	deadline = time.Now().Add(time.Second)
	status, failures := tk.State()
	for (calls.Load() < 2 || failures != 0 || status == task.StatusRunning) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		status, failures = tk.State()
	}
	if status != task.StatusIdle || failures != 0 {
		t.Fatalf("task status=%s failures=%d, want idle with reset failures", status, failures)
	}
}

func TestGracefulStop(t *testing.T) {
	s := task.NewScheduler()
	s.Start(context.Background(), nil)

	var finished int32
	// Use @every 1h to avoid auto-runs.
	s.Add("test-graceful", "@every 1h", func(ctx context.Context) error {
		time.Sleep(1 * time.Second)
		atomic.AddInt32(&finished, 1)
		return nil
	}, task.Config{})

	go s.Run("test-graceful") // Trigger manually

	time.Sleep(200 * time.Millisecond) // Let it start
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.Stop(ctx)
	duration := time.Since(start)

	if atomic.LoadInt32(&finished) != 1 {
		t.Errorf("Expected exactly 1 task to finish during graceful stop, got %d", atomic.LoadInt32(&finished))
	}
	if duration < 500*time.Millisecond {
		t.Errorf("Stop returned too quickly, didn't wait for task")
	}
}

func TestStopContinuesWaitingAfterTimeout(t *testing.T) {
	s := task.NewScheduler()
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	s.Add("test-repeated-stop", "@every 1h", func(context.Context) error {
		close(started)
		<-release
		return nil
	}, task.Config{})
	s.Run("test-repeated-stop")
	<-started

	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstCancel()
	if err := s.Stop(firstCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Stop error = %v, want context canceled", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- s.Stop(context.Background())
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("second Stop returned before task finished: %v", err)
	default:
	}

	close(release)
	if err := <-secondDone; err != nil {
		t.Fatalf("second Stop error = %v", err)
	}
}

func BenchmarkTaskRegistration(b *testing.B) {
	s := task.NewScheduler()
	fn := func(ctx context.Context) error { return nil }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Add("bench", "@every 1h", fn, task.Config{})
	}
}

func BenchmarkTaskExecutionOverhead(b *testing.B) {
	// Since we use cron, we can't easily benchmark the trigger frequency directly,
	// but we can benchmark the runTask method which contains the wrapper overhead.
	// We need to use unexported methods or reflection, or just benchmark a mocked run.
	// Actually, let's just benchmark the Add + List + Get operations which are common.
	s := task.NewScheduler()
	fn := func(ctx context.Context) error { return nil }
	s.Add("bench", "@every 1h", fn, task.Config{})

	b.Run("Get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.Get("bench")
		}
	})

	b.Run("List", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.List()
		}
	})
}
