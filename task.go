package task

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
)

type Policy int

const (
	PolicyParallel Policy = iota
	PolicySkip
)

type Status string

const (
	StatusIdle     Status = "idle"
	StatusRunning  Status = "running"
	StatusDisabled Status = "disabled"
)

// Config 任务配置
type Config struct {
	Policy                 Policy                                     `json:"policy"`
	Timeout                time.Duration                              `json:"timeout"`
	SuppressSuccessLog     bool                                       `json:"suppressSuccessLog"`
	MaxConsecutiveFailures int                                        `json:"maxConsecutiveFailures"`
	OnAutoDisable          func(name string, failures int, err error) `json:"-"`
}

// WithPolicy 创建指定策略的配置
func WithPolicy(p Policy) Config {
	return Config{Policy: p}
}

// WithTimeout 创建指定超时时间的配置
func WithTimeout(d time.Duration) Config {
	return Config{Timeout: d}
}

// Task 代表一个被调度的任务
type Task struct {
	Name                string `json:"name"`
	Spec                string `json:"spec"`
	Config              Config `json:"config"`
	Status              Status `json:"status"`
	ConsecutiveFailures int    `json:"consecutiveFailures"`

	Func func(ctx context.Context) error `json:"-"`

	s       *Scheduler
	entryID cron.EntryID
	running bool
	cancel  context.CancelFunc
}

// State returns a concurrency-safe snapshot of the task's runtime state.
func (t *Task) State() (Status, int) {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	return t.Status, t.ConsecutiveFailures
}

// Snapshot returns a concurrency-safe copy for inspection or serialization.
func (t *Task) Snapshot() *Task {
	t.s.mu.RLock()
	defer t.s.mu.RUnlock()
	snapshot := *t
	return &snapshot
}

// Disable 禁用该任务
func (t *Task) Disable() {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.Status = StatusDisabled
}

// Enable 启用该任务
func (t *Task) Enable() {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.Status = StatusIdle
	t.ConsecutiveFailures = 0
}

// Remove 从调度器中彻底移除该任务
func (t *Task) Remove() {
	t.s.Remove(t.Name)
}

// Add 注册一个新任务到全局默认调度器
func Add(name string, spec string, fn func(ctx context.Context) error, cfg ...Config) *Task {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return DefaultScheduler.Add(name, spec, fn, c)
}

// Get 获取指定名字的 Task
func Get(name string) *Task {
	return DefaultScheduler.Get(name)
}

// Enable enables a registered task. It returns false when the task is unknown.
func Enable(name string) bool {
	t := Get(name)
	if t == nil {
		return false
	}
	t.Enable()
	return true
}

// Disable disables a registered task. It returns false when the task is unknown.
func Disable(name string) bool {
	t := Get(name)
	if t == nil {
		return false
	}
	t.Disable()
	return true
}

// List 获取所有任务
func List() []*Task {
	return DefaultScheduler.List()
}

// Remove 从全局默认调度器中移除任务
func Remove(name string) {
	DefaultScheduler.Remove(name)
}

// Run 手动执行指定名字的任务
func Run(name string) {
	DefaultScheduler.Run(name)
}

// Run 手动执行该任务
func (t *Task) Run() {
	t.s.Run(t.Name)
}
