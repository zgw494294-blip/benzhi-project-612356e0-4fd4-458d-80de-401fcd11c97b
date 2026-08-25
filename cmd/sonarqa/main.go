package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/httpapi"
	"sonarqa/internal/store"
)

func main() {
	if err := run(os.Args[1:], os.Getenv("PORT")); err != nil {
		fmt.Fprintf(os.Stderr, "声呐测线质量验收台启动失败：%v\n", err)
		os.Exit(1)
	}
}

func run(args []string, portEnvironment string) error {
	configuration, err := parseConfig(args, portEnvironment)
	if err != nil {
		return err
	}
	dataDirectory := configuration.DataDirectory
	if configuration.Selfcheck && !configuration.DataExplicit {
		dataDirectory, err = os.MkdirTemp("", "sonarqa-selfcheck-*")
		if err != nil {
			return fmt.Errorf("创建 selfcheck 临时存储: %w", err)
		}
		defer os.RemoveAll(dataDirectory)
	}
	repository, err := store.Open(dataDirectory)
	if err != nil {
		return fmt.Errorf("恢复本地存储失败: %w", err)
	}
	service := application.NewService(repository, application.SystemClock{}, application.RandomIDGenerator{})
	logger := log.New(os.Stderr, "sonarqa ", log.LstdFlags|log.LUTC)
	server := httpapi.NewServer(service, logger)
	if configuration.Selfcheck {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpapi.RunSelfcheck(ctx, server, configuration.Address); err != nil {
			return fmt.Errorf("selfcheck 未通过: %w", err)
		}
		fmt.Println("selfcheck 通过：已完成创建、修订、评估、复核、冻结和归档放行")
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Printf("监听 %s，数据目录 %s", configuration.Address, dataDirectory)
	if err := server.ListenAndServe(ctx, configuration.Address); err != nil {
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	}
	return nil
}
