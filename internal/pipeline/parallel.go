package pipeline

import (
	"context"
	"sync"
)

func runParallel(ctx context.Context, n, tasks int, fn func(i int) error) error {
	if tasks == 0 {
		return nil
	}
	if n < 1 {
		n = 1
	}
	if n > tasks {
		n = tasks
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var once sync.Once
	var first error
	sem := make(chan struct{}, n)
	for i := 0; i < tasks; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			if err := fn(i); err != nil {
				once.Do(func() { first = err })
				cancel()
			}
		}(i)
	}
	wg.Wait()
	return first
}
