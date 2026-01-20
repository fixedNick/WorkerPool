package core

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

type MetricsCollector interface {
	RecordTaskStart()
	RecordTaskComplete(taskDuration time.Duration, success bool, attempts int64)
	RecordQueuePush()
	RecordQueuePop()

	TasksProcessed() int
	TasksRetried() int
	TasksFailed() int
	QueueSize() int
	AverageProcessTime() time.Duration

	StartLogging(ctx context.Context, writer io.Writer, interval time.Duration)
}

type Metrics struct {
	tasksProcessed   atomic.Int64
	tasksFailed      atomic.Int64
	tasksRetried     atomic.Int64
	queueLen         atomic.Int64
	totalProcessTime atomic.Int64 // nanoseconds
}

func (m *Metrics) RecordTaskStart() { m.tasksProcessed.Add(1) }
func (m *Metrics) RecordTaskComplete(d time.Duration, s bool, attempts int64) {
	m.totalProcessTime.Add(d.Nanoseconds())
	m.tasksRetried.Add(attempts)
	if !s {
		m.tasksFailed.Add(1)
	}
}

func (m *Metrics) RecordQueuePush() { m.queueLen.Add(1) }
func (m *Metrics) RecordQueuePop()  { m.queueLen.Add(-1) }

func (m *Metrics) QueueSize() int      { return int(m.queueLen.Load()) }
func (m *Metrics) TasksRetried() int   { return int(m.tasksRetried.Load()) }
func (m *Metrics) TasksFailed() int    { return int(m.tasksFailed.Load()) }
func (m *Metrics) TasksProcessed() int { return int(m.tasksProcessed.Load()) }
func (m *Metrics) AverageProcessTime() time.Duration {
	return time.Duration(m.totalProcessTime.Load()/m.tasksProcessed.Load()) * time.Nanosecond
}

// TODO: Append other logging methods
func (m *Metrics) StartLogging(ctx context.Context, writer io.Writer, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, err := fmt.Fprintf(writer, "---- METRICS | Queue: %d <-+-> Total: %d <-+-> Fail: %d <-+-> Ret: %d <-+-> AvgTime: %dms ----\n\r",
				m.QueueSize(),
				m.TasksProcessed(),
				m.TasksFailed(),
				m.TasksRetried(),
				m.AverageProcessTime().Milliseconds())

			// Handle
			// recovery
			// notify
			// restart
			if err != nil {
				panic(err)
			}

		}
	}
}
