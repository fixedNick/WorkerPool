package errs

type QueuFullError struct{}

func (e *QueuFullError) Error() string {
	return "task queue is full"
}
