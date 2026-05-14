package task

import (
	"context"
	"errors"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"apigo.cc/go/log"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron    *cron.Cron
	tasks   map[string]*Task
	mu      sync.RWMutex
	wg      sync.WaitGroup
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
	s.mu.Unlock()

	s.cron.Start()
	if logger != nil {
		logger.Info("scheduler started")
	} else {
		log.DefaultLogger.Info("scheduler started")
	}
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
		log.DefaultLogger.Info("scheduler stopped gracefully")
		return nil
	case <-ctx.Done():
		log.DefaultLogger.Warning("scheduler stop timed out or cancelled", "err", ctx.Err())
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

	ctx, cancel := context.WithCancel(context.Background())
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
		t.Status = StatusIdle
		t.cancel = nil
		s.mu.Unlock()
	}()

	// Recover
	defer func() {
		if r := recover(); r != nil {
			log.DefaultLogger.Error("task panic recovered", "name", name, "err", r, "stack", string(debug.Stack()))
		}
	}()

	start := time.Now()
	err := t.Func(ctx)
	duration := time.Since(start)

	if err != nil {
		log.DefaultLogger.Error("task failed", "name", name, "err", err, "duration", duration.String())
	} else {
		log.DefaultLogger.Info("task success", "name", name, "duration", duration.String())
	}
}
