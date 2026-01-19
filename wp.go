package workerpool

import (
	"context"
	"fmt"
	"io"
	"log"
	"main/channels/worker_pool/workerpool/modules"
	"net/http"
	"sync"
	"time"
)

type Task interface {
	Execute(ctx context.Context, workerID int, client *http.Client) (*TaskResult, *TaskError)
	Id() int
	Url() string
}

type HTTPTask struct {
	id  int
	url string
}

func (t HTTPTask) Id() int {
	return t.id
}

func (t HTTPTask) Url() string {
	return t.url
}

func (task HTTPTask) Execute(ctx context.Context, wid int, client *http.Client) (*TaskResult, *TaskError) {
	req, err := http.NewRequestWithContext(ctx, "GET", task.url, nil)
	if err != nil {
		return nil, &TaskError{
			url:    task.url,
			taskID: task.id,
			err:    err,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, &TaskError{
			url:    task.url,
			taskID: task.id,
			err:    err,
		}
	}

	defer resp.Body.Close()
	bodySize, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return nil, &TaskError{
			url:    task.url,
			taskID: task.id,
			err:    err,
		}
	}

	return &TaskResult{
		url:            task.url,
		workerID:       wid,
		taskID:         task.id,
		responseStatus: resp.StatusCode,
		contentLen:     int(bodySize),
	}, nil
}

type TaskError struct {
	url    string
	taskID int
	err    error
}

type TaskResult struct {
	workerID       int
	taskID         int
	contentLen     int
	responseStatus int

	url string

	totalDuration time.Duration
	duration      time.Duration
}

type Config struct {
	WorkerCount int
	QueueSize   int

	TaskTimeout     time.Duration
	ShutdownTimeout time.Duration

	RetryDelay time.Duration
	MaxRetries int
}

type WorkerPool struct {
	config Config
	ctx    context.Context

	resultsQueue chan *TaskResult
	errorQueue   chan *TaskError
	taskQueue    chan Task

	// modules
	retryManager *modules.RetryManager

	wg     sync.WaitGroup
	client *http.Client
}

func NewWorkerPool(ctx context.Context, config Config) *WorkerPool {
	return &WorkerPool{
		config:       config,
		ctx:          ctx,
		resultsQueue: make(chan *TaskResult, config.QueueSize),
		errorQueue:   make(chan *TaskError, config.QueueSize),
		// taskQueue:    make(chan Task, config.QueueSize),
		wg:           sync.WaitGroup{},
		retryManager: modules.NewRetryManager("./channels/worker_pool/workerpool/cfg/retry.yaml"),
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        config.WorkerCount * 2,
				MaxIdleConnsPerHost: config.WorkerCount,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}
}

func (wp *WorkerPool) Start(source <-chan Task) <-chan *TaskResult {
	for i := 0; i < wp.config.WorkerCount; i++ {
		wp.wg.Add(1)
		go func(n int) {
			wp.worker(n, source)
		}(i)
	}

	go func() {
		wp.wg.Wait()
		close(wp.resultsQueue)
		close(wp.errorQueue)
	}()

	return wp.resultsQueue
}

func (wp *WorkerPool) worker(wid int, source <-chan Task) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case task, ok := <-source:
			if !ok {
				// channel closed, stop worker
				return
			}
			start := time.Now()
			result, err := wp.processWithRetry(wid, task)
			if err == nil {
				result.totalDuration = time.Since(start)
			}

			if err != nil {
				wp.errorQueue <- err
			} else {
				wp.resultsQueue <- result
			}
		}
	}

}

func (wp *WorkerPool) processWithRetry(wid int, task Task) (*TaskResult, *TaskError) {

	var lastError *TaskError
	for attempt := 0; true; attempt++ {

		if attempt > 0 {
			delay := wp.retryManager.GetDelay(attempt)
			log.Printf("[url -> %s] Retry in %.2fs [attempt -> %d/%d] ", task.Url(), delay.Seconds(), attempt, wp.retryManager.MaxRetries())
			select {
			case <-time.After(delay):
			case <-wp.ctx.Done():
				return nil, &TaskError{
					url:    task.Url(),
					taskID: task.Id(),
					err:    wp.ctx.Err(),
				}
			}
		}

		ctx, cancel := context.WithTimeout(wp.ctx, wp.config.TaskTimeout)
		start := time.Now()
		res, taskErr := task.Execute(ctx, wid, wp.client)
		execDuration := time.Since(start)
		cancel()

		if taskErr == nil {
			res.duration = execDuration
			return res, nil
		}

		if !wp.retryManager.ShouldRetry(taskErr.err, attempt) {
			lastError = taskErr
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

	buffSize := 20
	tasks := genTasks(buffSize)

	wp := NewWorkerPool(context.Background(), Config{
		WorkerCount:     5,
		QueueSize:       20,
		TaskTimeout:     time.Second * 2,
		ShutdownTimeout: time.Second * 5,
	})
	results := wp.Start(tasks)

	c := 0
	for result := range results {
		c++
		fmt.Printf("| [%dms (T:%.2d)] -- %d:[%d] STATUS `%d` --> LENGTH `%d\n\r",
			result.duration.Milliseconds(),
			result.totalDuration.Seconds(),
			result.workerID,
			result.taskID,
			result.responseStatus,
			result.contentLen,
		)
	}

	ec := 0
	for e := range wp.errorQueue {
		ec++
		fmt.Println("! ---")
		fmt.Printf("[id:%f] Url: %s\n\rErr: %s\n\r", e.taskID, e.url, e.err.Error())
		fmt.Println("! ---")
	}

	fmt.Printf("Total | Success: %d / Error: %d", c, ec)

}

func genTasks(buffSize int) <-chan Task {
	tasks := make(chan Task, buffSize)

	urls := []string{
		"https://google.com/", "https://vk.com/", "https://amazon.com/",
		"https://reddit.com/", "https://mail.ru/", "https://example.com/",
		"https://unks12.co", "https://moz.com", "https://cloudflare.com/",
		"https://www.randomlists.com/urls", "https://habr.com/ru/articles/901128/", "https://e5450.com/socket-2011-3/e5-2600-v3/xeon-e5-2640-v3/",
	}

	go func() {
		defer close(tasks)
		for i := 0; i < 10; i++ {
			for idx, url := range urls {
				task := HTTPTask{
					id:  idx * (i + 1),
					url: url,
				}
				tasks <- task
			}
		}
	}()

	return tasks
}
