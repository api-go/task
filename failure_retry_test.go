package task_test

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/api-go/task"
)

func waitTaskIdle(t *testing.T, job *task.Task, failures int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, count := job.State()
		if status != task.StatusRunning && count == failures {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("task did not finish")
}

func TestFailureCooldownPreservesExplicitPauseAndRecovers(t *testing.T) {
	s := task.NewScheduler()
	s.Start(context.Background(), nil)
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	var calls atomic.Int32
	job := s.Add("cooldown", "@every 1h", func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("dependency unavailable")
		}
		return nil
	}, task.Config{Policy: task.PolicySkip, MaxConsecutiveFailures: 1, FailureRetryDelay: time.Hour, OnAutoDisable: func(string, int, error) { t.Error("cooldown persisted disable") }})
	s.Run(job.Name)
	waitTaskIdle(t, job, 1)
	if job.Snapshot().RetryAt.IsZero() {
		t.Fatal("retry deadline missing")
	}
	s.Run(job.Name)
	if calls.Load() != 1 {
		t.Fatal("cooldown did not prevent immediate retry")
	}
	job.Disable()
	s.Run(job.Name)
	if status, _ := job.State(); status != task.StatusDisabled || calls.Load() != 1 {
		t.Fatal("automatic recovery overrode explicit pause")
	}
	job.Enable()
	s.Run(job.Name)
	deadline := time.Now().Add(time.Second)
	for calls.Load() != 2 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	waitTaskIdle(t, job, 0)
	if calls.Load() != 2 || !job.Snapshot().RetryAt.IsZero() {
		t.Fatal("successful recovery did not reset cooldown")
	}
	// A short cooldown automatically permits a later scheduled invocation.
	automatic := s.Add("automatic-retry", "@every 1h", func(context.Context) error {
		if calls.Add(1) == 3 {
			return errors.New("temporary outage")
		}
		return nil
	}, task.Config{Policy: task.PolicySkip, MaxConsecutiveFailures: 1, FailureRetryDelay: time.Nanosecond})
	s.Run(automatic.Name)
	waitTaskIdle(t, automatic, 1)
	s.Run(automatic.Name)
	for calls.Load() != 4 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	waitTaskIdle(t, automatic, 0)
	if calls.Load() != 4 {
		t.Fatal("dependency recovery required manual enable")
	}
}
