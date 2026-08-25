package domain

import "fmt"

// Error exposes a stable code without leaking implementation details to transports.
type Error struct {
	Code    string
	Message string
	Field   string
}

func (e *Error) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func NewError(code, message string) error {
	return &Error{Code: code, Message: message}
}

func FieldError(field, message string) error {
	return &Error{Code: "VALIDATION_FAILED", Field: field, Message: message}
}

func ErrorCode(err error) string {
	if typed, ok := err.(*Error); ok {
		return typed.Code
	}
	return "INTERNAL_ERROR"
}

var (
	ErrNotFound        = NewError("NOT_FOUND", "验收任务不存在")
	ErrVersionConflict = NewError("VERSION_CONFLICT", "任务版本已变化，请刷新后重试")
	ErrFrozen          = NewError("DATASET_FROZEN", "数据集冻结后禁止修改业务数据")
	ErrStateConflict   = NewError("STATE_CONFLICT", "当前任务状态不允许该操作")
	ErrUnauthorized    = NewError("ROLE_FORBIDDEN", "当前角色无权执行该操作")
)
