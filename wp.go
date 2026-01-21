package workerpool

import (
	"context"
	"errors"
	"fmt"
	"log"
	"main/channels/worker_pool/workerpool/core"
	errs "main/channels/worker_pool/workerpool/errors"
	"main/channels/worker_pool/workerpool/modules"
	ratelimiter "main/channels/worker_pool/workerpool/modules/rateLimiter"
	retry "main/channels/worker_pool/workerpool/modules/retryManager"
	"net/http"
	"os"
	"sync"
	"time"
)

type Config struct {
	WorkerCount int
	QueueSize   int

	RateLimit int

	TaskTimeout     time.Duration
	ShutdownTimeout time.Duration

	MetricsEnable bool
}

type WorkerPool struct {
	config Config
	ctx    context.Context

	resultsQueue chan *core.TaskResult
	errorQueue   chan *core.TaskError
	taskQueue    chan core.Task

	// modules
	retryManager modules.RetryManager
	rateLimiter  modules.RateLimiter
	metrics      modules.MetricsCollector
	statistics   modules.StatisticsCollector

	wg     sync.WaitGroup
	client *http.Client
}

func NewWorkerPool(ctx context.Context, config Config) *WorkerPool {
	return &WorkerPool{
		config:       config,
		ctx:          ctx,
		resultsQueue: make(chan *core.TaskResult, config.QueueSize),
		errorQueue:   make(chan *core.TaskError, config.QueueSize),
		taskQueue:    make(chan core.Task, config.QueueSize),
		metrics:      &modules.Metrics{},
		statistics:   &modules.Statistics{},
		wg:           sync.WaitGroup{},
		retryManager: retry.NewRetryManager("./channels/worker_pool/workerpool/cfg/retry.yaml"),
		rateLimiter:  ratelimiter.NewRateLimiter(ctx, config.RateLimit),
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        config.WorkerCount * 2,
				MaxIdleConnsPerHost: config.WorkerCount,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}
}

func (wp *WorkerPool) Submit(task core.Task) error {
	select {
	case wp.taskQueue <- task:
		if wp.config.MetricsEnable {
			wp.metrics.RecordQueuePush()
		}
		return nil
	case <-wp.ctx.Done():
		return wp.ctx.Err()
	default:
		return &errs.QueuFullError{}
	}
}

func (wp *WorkerPool) Start() <-chan *core.TaskResult {
	for i := 0; i < wp.config.WorkerCount; i++ {
		wp.wg.Add(1)
		go func(n int) {
			wp.worker(n)
		}(i)
	}

	// Start modules
	if wp.config.MetricsEnable {
		go wp.metrics.StartLogging(wp.ctx, os.Stdout, time.Second*3)
	}

	wp.statistics.Start()

	//

	go func() {
		wp.wg.Wait()
		close(wp.resultsQueue)
		close(wp.errorQueue)
	}()

	return wp.resultsQueue
}

func (wp *WorkerPool) worker(wid int) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case task, ok := <-wp.taskQueue:
			if !ok {
				// channel closed, stop worker
				return
			}

			if err := wp.rateLimiter.Wait(wp.ctx); err != nil {
				return
			}

			wp.statistics.TaskStart()
			if wp.config.MetricsEnable {
				wp.metrics.RecordQueuePop()
				wp.metrics.RecordTaskStart()
			}

			start := time.Now()
			result, err := wp.processWithRetry(wid, task)

			log.Printf("AvgTaskPerSec: %.2f. %d/%d [%d]",
				wp.statistics.AvgThroughput(),
				wp.statistics.TasksSucceed(),
				wp.statistics.TasksFailed(),
				wp.statistics.TasksTotal(),
			)
			if err == nil {
				result.SetTotalDuration(time.Since(start))
				wp.resultsQueue <- result
				continue
			}

			wp.errorQueue <- err
		}
	}

}

func (wp *WorkerPool) processWithRetry(wid int, task core.Task) (*core.TaskResult, *core.TaskError) {

	var lastError *core.TaskError
	for attempt := 1; true; attempt++ {

		if attempt > 1 {
			delay := wp.retryManager.GetDelay(attempt)
			log.Printf("[url -> %s] Retry in %.2fs [attempt -> %d/%d] ", task.Url(), delay.Seconds(), attempt, wp.retryManager.MaxRetries())
			select {
			case <-time.After(delay):
			case <-wp.ctx.Done():
				return nil, core.NewTaskError(
					task.Url(),
					task.Id(),
					wp.ctx.Err(),
				)
			}
		}

		ctx, cancel := context.WithTimeout(wp.ctx, wp.config.TaskTimeout)
		start := time.Now()
		res, taskErr := task.Execute(ctx, wid, wp.client)
		execDuration := time.Since(start)
		cancel()

		if taskErr == nil {
			res.SetDuration(execDuration)
			res.SetRetries(attempt)

			wp.statistics.TaskFinished(true)
			if wp.config.MetricsEnable {
				wp.metrics.RecordTaskComplete(execDuration, true, int64(attempt))
			}
			return res, nil
		}

		if !wp.retryManager.ShouldRetry(taskErr.Err(), attempt) {
			lastError = taskErr

			wp.statistics.TaskFinished(false)
			if wp.config.MetricsEnable {
				wp.metrics.RecordTaskComplete(execDuration, false, int64(attempt))
			}
			break
		}
	}

	return nil, lastError
}

func (wp *WorkerPool) GracefulShutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(wp.ctx, timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		fmt.Println("Gracefull shutdown finished by timeout")
	case <-done:
		fmt.Println("Gracefull shutdown finished")
	}
}

func Run() {
	ctx := context.WithoutCancel(context.Background())
	wp := NewWorkerPool(ctx, Config{
		WorkerCount:     5,
		QueueSize:       20,
		TaskTimeout:     time.Second * 2,
		ShutdownTimeout: time.Second * 5,
		RateLimit:       120,
		MetricsEnable:   true,
	})

	genTasks(wp)

	results := wp.Start()

	c := 0
	go func() {
		for result := range results {
			c++
			fmt.Printf("| [%dms(%.2fs)][retries:%d] -- %d:[%d] STATUS `%d` --> LENGTH `%d\n\r",
				result.Duration().Milliseconds(),
				result.TotalDuration().Seconds(),
				result.Retries(),
				result.WorkerID(),
				result.TaskID(),
				result.ResponseStatus(),
				result.ContentLen(),
			)
		}
	}()

	ec := 0
	for e := range wp.errorQueue {
		ec++
		fmt.Println("! ---")
		fmt.Printf("[id:%d ] Url: %s\n\rErr: %s\n\r",
			e.TaskID(),
			e.Url(),
			e.Err().Error(),
		)
		fmt.Println("! ---")
	}

	fmt.Printf("Total | Success: %d / Error: %d", c, ec)
}

func genTasks(wp *WorkerPool) {
	urls := []string{
		"https://google.com/", "https://vk.com/", "https://amazon.com/",
		"https://reddit.com/", "https://mail.ru/", "https://example.com/",
		"https://unks12.co", "https://moz.com", "https://cloudflare.com/",
		"https://www.randomlists.com/urls", "https://habr.com/ru/articles/901128/", "https://e5450.com/socket-2011-3/e5-2600-v3/xeon-e5-2640-v3/",
	}

	delay := time.Millisecond * 500

	go func(delay time.Duration) {

		defer close(wp.taskQueue)
		for i := 0; i < 10; i++ {
			for idx, url := range urls {
				task := core.NewHTTPTask(idx*(i+1), url)

				for {
					// Possible queue full error
					// Check and if true - delay
					var queuFull *errs.QueuFullError
					err := wp.Submit(task)
					if err == nil {
						break
					}

					if errors.As(err, &queuFull) {
						time.Sleep(delay)
						continue
					} else {
						panic(err)
					}
				}
			}
		}
	}(delay)
}
