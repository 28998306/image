// Package workerpool 号池注册任务的全局并发执行器。
//
// 设计要点：
//
//  1. 全局有 1 个 Pool 单例，通过 `Submit(taskID)` 投递任务。
//  2. Pool 内部有固定大小（默认 5）的 goroutine 池消费 channel。
//     超过容量的任务在 channel 中排队（buffer=1024），不会丢。
//  3. 应用启动时调用 Recover() 把上次进程异常退出留下的 running 任务
//     标记为 failed（防止"幽灵任务"卡在 running 永不结束）。
//  4. 任务执行函数 RunFn 由 service 层注入，pool 不感知业务。
package workerpool

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

// RunFn 执行单个注册任务的回调，由 RegisterTaskService 注入。
type RunFn func(ctx context.Context, taskID uint64)

// RecoverFn 启动时恢复一次性回调（把僵死的 running 任务标 failed）。
type RecoverFn func(ctx context.Context) error

// Pool 全局并发 worker pool。
type Pool struct {
	mu          sync.Mutex
	concurrency int
	started     int
	queue       chan uint64
	runFn       RunFn
	stopOnce    sync.Once
	stopped     chan struct{}
	wg          sync.WaitGroup
}

const (
	maxConcurrency = 64
	defaultConc    = 5
)

func clampConcurrency(n int) int {
	if n <= 0 {
		return defaultConc
	}
	if n > maxConcurrency {
		return maxConcurrency
	}
	return n
}

// New 创建一个 Pool。concurrency<=0 时回落到 5，超过 64 截断到 64。
func New(concurrency int, runFn RunFn) *Pool {
	return &Pool{
		concurrency: clampConcurrency(concurrency),
		queue:       make(chan uint64, 1024),
		runFn:       runFn,
		stopped:     make(chan struct{}),
	}
}

// Start 启动 worker goroutine。重复调用会按当前 concurrency 补齐缺口。
func (p *Pool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := p.started; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker(i)
		p.started++
	}
	log.Printf("register worker pool started concurrency=%d", p.concurrency)
}

// Concurrency 当前并发数。
func (p *Pool) Concurrency() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.concurrency
}

// Resize 在线调整并发数。
func (p *Pool) Resize(n int) int {
	n = clampConcurrency(n)
	p.mu.Lock()
	defer p.mu.Unlock()
	prev := p.concurrency
	p.concurrency = n
	if n > p.started {
		for i := p.started; i < n; i++ {
			p.wg.Add(1)
			go p.worker(i)
			p.started++
		}
	}
	if n != prev {
		log.Printf("register worker pool resized old=%d new=%d workers=%d", prev, n, p.started)
	}
	return n
}

// Submit 投递一个任务 ID。
func (p *Pool) Submit(taskID uint64) {
	select {
	case <-p.stopped:
		log.Printf("register pool stopped, drop task id=%d", taskID)
	default:
	}
	p.queue <- taskID
}

// Stop 等待全部 worker 完成。
func (p *Pool) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopped)
		close(p.queue)
	})
	p.wg.Wait()
}

func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for taskID := range p.queue {
		p.safeRun(taskID, id)
	}
}

func (p *Pool) safeRun(taskID uint64, workerID int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("register worker panic worker=%d task_id=%d panic=%v\n%s", workerID, taskID, r, debug.Stack())
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	p.runFn(ctx, taskID)
}
