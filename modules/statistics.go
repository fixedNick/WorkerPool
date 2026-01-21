package modules

import (
	"sync/atomic"
	"time"
)

type StatisticsCollector interface {
	Start()
	TaskStart()
	TaskFinished(success bool)

	// Getters
	AvgThroughput() float64 // Tasks per second
	TasksTotal() int
	TasksSucceed() int
	TasksFailed() int
	StartTime() time.Time
}

type Statistics struct {
	startTime   time.Time
	totalTasks  atomic.Int32
	successfull atomic.Int32
	failed      atomic.Int32
}

func (s *Statistics) Start()     { s.startTime = time.Now() }
func (s *Statistics) TaskStart() { s.totalTasks.Add(1) }
func (s *Statistics) TaskFinished(success bool) {
	if success {
		s.successfull.Add(1)
	} else {
		s.failed.Add(1)
	}
}

// Getters
func (s *Statistics) AvgThroughput() float64 {
	return float64(s.TasksTotal()) / time.Since(s.startTime).Seconds()
}
func (s *Statistics) TasksSucceed() int    { return int(s.successfull.Load()) }
func (s *Statistics) TasksFailed() int     { return int(s.failed.Load()) }
func (s *Statistics) TasksTotal() int      { return int(s.totalTasks.Load()) }
func (s *Statistics) StartTime() time.Time { return s.startTime }
