package task

import (
	"time"

	"github.com/robfig/cron/v3"
)

type Policy int

const (
	PolicyParallel Policy = iota
	PolicySkip
	PolicyQueue
)

type Status string

const (
	StatusIdle     Status = "idle"
	StatusRunning  Status = "running"
	StatusDisabled Status = "disabled"
)

type JobFunc func()
type JobFuncWithError func() error

type Option func(*Task)

// Task 代表一个被调度的任务
type Task struct {
	Name    string        `json:"name"`
	Spec    string        `json:"spec"`
	Policy  Policy        `json:"policy"`
	Timeout time.Duration `json:"timeout"`
	Status  Status        `json:"status"`

	Func    JobFunc          `json:"-"`
	FuncErr JobFuncWithError `json:"-"`

	onSuccess func(duration time.Duration)
	onError   func(err error)

	s       *Scheduler
	entryID cron.EntryID
	running bool
	queue   chan struct{}
}

// Disable 禁用该任务，到了调度时间也不会执行
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
}

// Remove 从调度器中彻底移除该任务
func (t *Task) Remove() {
	t.s.Remove(t.Name)
}

// Global default scheduler proxy methods

// Add 注册一个新任务到全局默认调度器，并返回 Task 对象实例
func Add(name string, spec string, cmd any, opts ...Option) *Task {
	return DefaultScheduler.Add(name, spec, cmd, opts...)
}

// Get 获取全局默认调度器中指定名字的 Task 对象
func Get(name string) *Task {
	return DefaultScheduler.Get(name)
}

// List 获取全局默认调度器中所有的 Task 对象
func List() []*Task {
	return DefaultScheduler.List()
}

// Remove 根据名字从全局默认调度器中移除任务
func Remove(name string) {
	DefaultScheduler.Remove(name)
}

// Start 启动全局默认调度器
func Start() {
	DefaultScheduler.Start()
}

// Stop 停止全局默认调度器
func Stop() {
	DefaultScheduler.Stop()
}

// Options

func WithPolicy(p Policy) Option {
	return func(t *Task) {
		t.Policy = p
	}
}

func WithTimeout(d time.Duration) Option {
	return func(t *Task) {
		t.Timeout = d
	}
}

func OnSuccess(fn func(duration time.Duration)) Option {
	return func(t *Task) {
		t.onSuccess = fn
	}
}

func OnError(fn func(err error)) Option {
	return func(t *Task) {
		t.onError = fn
	}
}

func init() {
	// 默认在加载时启动，若无任务则仅占用极少量等待资源
	Start()
}
