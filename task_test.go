package task

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestTaskBasic(t *testing.T) {
	var count int32
	Add("test-basic", "@every 1s", func() {
		atomic.AddInt32(&count, 1)
	})
	time.Sleep(2500 * time.Millisecond)
	
	if atomic.LoadInt32(&count) < 2 {
		t.Errorf("Expected at least 2 runs, got %d", count)
	}
}

func TestTaskLifecycle(t *testing.T) {
	var count int32
	name := "test-lifecycle"
	tk := Add(name, "@every 1s", func() {
		atomic.AddInt32(&count, 1)
	})
	
	tk.Disable() // Use object method
	time.Sleep(1500 * time.Millisecond)
	if atomic.LoadInt32(&count) != 0 {
		t.Errorf("Expected 0 runs while disabled, got %d", count)
	}
	
	tasks := List()
	found := false
	for _, item := range tasks {
		if item.Name == name {
			found = true
			if item.Status != StatusDisabled {
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
	
	// Test Remove
	tk.Remove()
	if Get(name) != nil {
		t.Errorf("Expected task to be nil after remove")
	}
}

func TestTaskPolicySkip(t *testing.T) {
	var count int32
	Add("test-skip", "@every 1s", func() {
		atomic.AddInt32(&count, 1)
		time.Sleep(1500 * time.Millisecond) // Longer than interval
	}, WithPolicy(PolicySkip))
	
	time.Sleep(3500 * time.Millisecond) // T=0 (starts, running), T=1 (skip), T=2 (starts after T=0 finishes at 1.5, next trigger at T=2), T=3 (skip)
	
	// T=0: run 1 starts
	// T=1: skip
	// T=1.5: run 1 ends
	// T=2: run 2 starts
	// T=3: skip
	// T=3.5: run 2 ends
	
	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("Expected exactly 2 runs due to skip policy, got %d", count)
	}
}

func TestTaskTimeout(t *testing.T) {
	var errCount int32
	Add("test-timeout", "@every 1s", func() {
		time.Sleep(2 * time.Second)
	}, WithTimeout(500*time.Millisecond), OnError(func(err error) {
		atomic.AddInt32(&errCount, 1)
	}))
	
	time.Sleep(2500 * time.Millisecond)
	
	if atomic.LoadInt32(&errCount) < 2 {
		t.Errorf("Expected at least 2 timeout errors, got %d", errCount)
	}
}

func TestTaskWithError(t *testing.T) {
	var errCount int32
	Add("test-error", "@every 1s", func() error {
		return errors.New("expected error")
	}, OnError(func(err error) {
		atomic.AddInt32(&errCount, 1)
	}))
	
	time.Sleep(2500 * time.Millisecond)
	
	if atomic.LoadInt32(&errCount) < 2 {
		t.Errorf("Expected at least 2 errors, got %d", errCount)
	}
}
