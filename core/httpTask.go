package core

import (
	"context"
	"io"
	"net/http"
)

type HTTPTask struct {
	id  int
	url string
}

func NewHTTPTask(id int, url string) HTTPTask {
	return HTTPTask{
		id:  id,
		url: url,
	}
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
