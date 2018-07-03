package consensus

import "fmt"

const (
	ErrUnknown = 1
)

type ErrorCode struct {
	Code int
	Memo string
}

func (e *ErrorCode) GetCode() int {
	return e.Code
}

func (e *ErrorCode) Error() string {
	return fmt.Sprintf("%d - %s", e.Code, e.Memo)
}
