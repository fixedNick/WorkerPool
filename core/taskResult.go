package core

import (
	"time"
)

type TaskResult struct {
	workerID       int
	taskID         int
	contentLen     int
	responseStatus int
	retries        int

	url string

	totalDuration time.Duration
	duration      time.Duration
}

func NewTaskResult(wid, tid, clen, status int, url string, dur, tdur time.Duration) *TaskResult {
	return &TaskResult{
		workerID:       wid,
		taskID:         tid,
		contentLen:     clen,
		responseStatus: status,

		url: url,

		duration:      dur,
		totalDuration: tdur,
	}
}

// getters
func (t *TaskResult) WorkerID() int                { return t.workerID }
func (t *TaskResult) TaskID() int                  { return t.taskID }
func (t *TaskResult) ContentLen() int              { return t.contentLen }
func (t *TaskResult) ResponseStatus() int          { return t.responseStatus }
func (t *TaskResult) Url() string                  { return t.url }
func (t *TaskResult) Duration() time.Duration      { return t.duration }
func (t *TaskResult) TotalDuration() time.Duration { return t.totalDuration }
func (t *TaskResult) Retries() int                 { return t.retries }

// setters
func (t *TaskResult) SetDuration(d time.Duration)      { t.duration = d }
func (t *TaskResult) SetTotalDuration(d time.Duration) { t.totalDuration = d }
func (t *TaskResult) SetRetries(r int)                 { t.retries = r }

//
