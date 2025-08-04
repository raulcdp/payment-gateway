package worker

import (
	"context"
	"fmt"
	"sync"
)

type WorkerPool[T any] struct {
	workers int
	tasks   chan T
	handler func(ctx context.Context, req T) error
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewWorkerPool[T any](ctx context.Context, workerCount int, handler func(ctx context.Context, req T) error) *WorkerPool[T] {
	ctx, cancel := context.WithCancel(ctx)
	return &WorkerPool[T]{
		workers: workerCount,
		tasks:   make(chan T),
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (wp *WorkerPool[T]) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go func() {
			defer wp.wg.Done()
			for {
				select {
				case task, ok := <-wp.tasks:
					if !ok {
						return
					}
					err := wp.handler(wp.ctx, task)
					if err != nil {
						fmt.Println("[WORKER] Error processing task:", err)
						continue
					}
				case <-wp.ctx.Done():
					return
				}
			}
		}()
	}
}

func (wp *WorkerPool[T]) Submit(task T) {
	fmt.Println("[WORKER] Submitting task:", task)
	wp.tasks <- task
}

func (wp *WorkerPool[T]) Stop() {
	wp.cancel()
	close(wp.tasks)
	wp.wg.Wait()
}
