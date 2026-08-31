package task

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"apigo.cc/go/id"
	"apigo.cc/go/log"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron    *cron.Cron
	tasks   map[string]*Task
	mu      sync.RWMutex
	wg      sync.WaitGroup
	logger  *log.Logger
	running bool
}

var DefaultScheduler = NewScheduler()

func NewScheduler() *Scheduler {
	return &Scheduler{
		cron:  cron.New(cron.WithSeconds()),
		tasks: make(map[string]*Task),
	}
}

func (s *Scheduler) Add(name string, spec string, fn func(context.Context) error, cfg Config) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove old task if exists
	if old, exists := s.tasks[name]; exists {
		s.cron.Remove(old.entryID)
		if old.cancel != nil {
			old.cancel()
		}
	}

	t := &Task{
		Name:   name,
		Spec:   spec,
		Config: cfg,
		Status: StatusIdle,
		Func:   fn,
		s:      s,
	}

	id, err := s.cron.AddFunc(spec, func() {
		s.runTask(name)
	})
	if err != nil {
		log.DefaultLogger.Error("failed to add task", "name", name, "spec", spec, "err", err)
		return nil
	}

	t.entryID = id
	s.tasks[name] = t
	return t
}

func (s *Scheduler) Start(ctx context.Context, logger *log.Logger) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	if logger != nil {
		s.logger = logger
	}
	s.mu.Unlock()

	s.cron.Start()
	return nil
}

func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	s.cron.Stop()

	s.mu.Lock()
	for _, t := range s.tasks {
		if t.cancel != nil {
			t.cancel()
		}
	}
	s.mu.Unlock()

	// Wait for running tasks with context
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) Status() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.running {
		return "stopped", errors.New("scheduler is not running")
	}
	return "running", nil
}

// Start 开始全局默认调度器
func Start(ctx context.Context, logger *log.Logger) error {
	return DefaultScheduler.Start(ctx, logger)
}

// Stop 停止全局默认调度器
func Stop(ctx context.Context) error {
	return DefaultScheduler.Stop(ctx)
}

func (s *Scheduler) Run(name string) {
	s.mu.RLock()
	_, ok := s.tasks[name]
	s.mu.RUnlock()
	if ok {
		go s.runTask(name)
	}
}

func (s *Scheduler) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[name]; ok {
		s.cron.Remove(t.entryID)
		if t.cancel != nil {
			t.cancel()
		}
		delete(s.tasks, name)
		log.DefaultLogger.Info("task removed", "name", name)
	}
}

func (s *Scheduler) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

func (s *Scheduler) Get(name string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks[name]
}

func (s *Scheduler) runTask(name string) {
	s.mu.Lock()
	t, ok := s.tasks[name]
	if !ok || t.Status == StatusDisabled {
		s.mu.Unlock()
		return
	}

	if t.Config.Policy == PolicySkip && t.running {
		log.DefaultLogger.Debug("task skipped", "name", name)
		s.mu.Unlock()
		return
	}

	t.running = true
	t.Status = StatusRunning
	s.mu.Unlock()

	s.wg.Add(1)
	defer s.wg.Done()
	taskLogger := log.DefaultLogger
	s.mu.RLock()
	parentLogger := s.logger
	s.mu.RUnlock()
	if parentLogger != nil {
		taskLogger = parentLogger.New(id.Get10Bytes14MPerSecond())
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithLogger(ctx, taskLogger)
	if t.Config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, t.Config.Timeout)
	}

	s.mu.Lock()
	t.cancel = cancel
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		t.running = false
		if t.Status != StatusDisabled {
			t.Status = StatusIdle
		}
		t.cancel = nil
		s.mu.Unlock()
	}()

	start := time.Now()
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				taskLogger.Error("task panic recovered", "name", name, "err", r, "stack", string(debug.Stack()))
				err = fmt.Errorf("task panic: %v", r)
			}
		}()
		return t.Func(ctx)
	}()
	duration := time.Since(start)

	if err != nil {
		s.mu.Lock()
		t.ConsecutiveFailures++
		failures := t.ConsecutiveFailures
		autoDisabled := t.Config.MaxConsecutiveFailures > 0 && failures >= t.Config.MaxConsecutiveFailures
		if autoDisabled {
			t.Status = StatusDisabled
		}
		onAutoDisable := t.Config.OnAutoDisable
		s.mu.Unlock()
		taskLogger.Error("task failed", "name", name, "err", err, "duration", duration.String())
		if autoDisabled {
			taskLogger.Error("task auto disabled", "name", name, "consecutiveFailures", failures)
			if onAutoDisable != nil {
				onAutoDisable(name, failures, err)
			}
		}
	} else if !t.Config.SuppressSuccessLog {
		s.mu.Lock()
		t.ConsecutiveFailures = 0
		s.mu.Unlock()
		taskLogger.Info("task success", "name", name, "duration", duration.String())
	} else {
		s.mu.Lock()
		t.ConsecutiveFailures = 0
		s.mu.Unlock()
	}
}

func traceID(logger *log.Logger) string {
	if logger == nil {
		return ""
	}
	return logger.GetTraceId()
}
