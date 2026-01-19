package core

type TaskError struct {
	url    string
	taskID int
	err    error
}

func NewTaskError(url string, taskID int, err error) *TaskError {
	return &TaskError{
		url:    url,
		taskID: taskID,
		err:    err,
	}
}

// getters
func (t *TaskError) Err() error  { return t.err }
func (t *TaskError) TaskID() int { return t.taskID }
func (t *TaskError) Url() string { return t.url }
