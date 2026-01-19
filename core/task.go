package core

import (
	"context"
	"net/http"
)

type Task interface {
	Execute(ctx context.Context, workerID int, client *http.Client) (*TaskResult, *TaskError)
	Id() int
	Url() string
}
