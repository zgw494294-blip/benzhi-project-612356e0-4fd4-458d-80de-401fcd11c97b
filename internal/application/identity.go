package application

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"sonarqa/internal/domain"
)

type RandomIDGenerator struct{}

func (RandomIDGenerator) NewID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("生成安全随机编号失败: %v", err))
	}
	return strings.TrimSuffix(prefix, "-") + "-" + hex.EncodeToString(buffer)
}

type Actor struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

const (
	RoleProcessor = "processor"
	RoleReviewer  = "reviewer"
	RoleArchivist = "archivist"
)

func (a Actor) Require(role string) error {
	if strings.TrimSpace(a.ID) == "" {
		return domainError("ACTOR_REQUIRED", "必须提供操作者编号")
	}
	if a.Role != role {
		return domainError("ROLE_FORBIDDEN", "当前操作者角色不允许执行该操作")
	}
	return nil
}

func domainError(code, message string) error {
	return &applicationError{Code: code, Message: message}
}

type applicationError struct {
	Code    string
	Message string
}

func (e *applicationError) Error() string { return e.Message }

func ErrorCode(err error) string {
	if typed, ok := err.(*applicationError); ok {
		return typed.Code
	}
	return domain.ErrorCode(err)
}
