package task

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron  *cron.Cron
	tasks map[string]*Task
	mu    sync.RWMutex
}

var DefaultScheduler = NewScheduler()

func NewScheduler() *Scheduler {
	return &Scheduler{
		cron:  cron.New(cron.WithSeconds()),
		tasks: make(map[string]*Task),
	}
}

func (s *Scheduler) Add(name string, spec string, cmd any, opts ...Option) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	t := &Task{
		Name:   name,
		Spec:   spec,
		Status: StatusIdle,
		s:      s,
	}

	switch fn := cmd.(type) {
	case func():
		t.Func = fn
	case func() error:
		t.FuncErr = fn
	default:
		panic("invalid job function: must be func() or func() error")
	}

	for _, opt := range opts {
		opt(t)
	}

	if t.Policy == PolicyQueue {
		t.queue = make(chan struct{}, 100)
	}

	// Remove old task if exists to replace it
	if old, exists := s.tasks[name]; exists {
		s.cron.Remove(old.entryID)
	}

	s.tasks[name] = t
	id, _ := s.cron.AddFunc(spec, func() {
		s.runTask(name)
	})
	t.entryID = id

	return t
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

func (s *Scheduler) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[name]; ok {
		s.cron.Remove(t.entryID)
		delete(s.tasks, name)
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

	if t.Policy == PolicySkip && t.running {
		s.mu.Unlock()
		return
	}
	if t.Policy == PolicyQueue && t.running {
		s.mu.Unlock()
		t.queue <- struct{}{}
		return
	}

	t.running = true
	t.Status = StatusRunning
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		t.running = false
		t.Status = StatusIdle

		if t.Policy == PolicyQueue && len(t.queue) > 0 {
			<-t.queue
			s.mu.Unlock()
			go s.runTask(name)
		} else {
			s.mu.Unlock()
		}
	}()

	start := time.Now()
	var err error

	if t.Timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), t.Timeout)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			if t.FuncErr != nil {
				done <- t.FuncErr()
			} else {
				t.Func()
				done <- nil
			}
		}()

		select {
		case <-ctx.Done():
			err = fmt.Errorf("task %s timed out after %v", t.Name, t.Timeout)
		case e := <-done:
			err = e
		}
	} else {
		if t.FuncErr != nil {
			err = t.FuncErr()
		} else {
			t.Func()
		}
	}

	duration := time.Since(start)
	if err != nil {
		if t.onError != nil {
			t.onError(err)
		}
	} else {
		if t.onSuccess != nil {
			t.onSuccess(duration)
		}
	}
}
