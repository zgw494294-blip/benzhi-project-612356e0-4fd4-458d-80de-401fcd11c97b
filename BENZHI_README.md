# BENZHI_README

基于 Go 实现的声呐测线质量验收台 HTTP API 项目，一款后端服务，声呐测线质量验收台提供从测区任务创建、测线修订提交、确定性质量评估、异常整改与复验、独立复核、冻结清单到归档放行凭据签发的可审计 JSON HTTP 服务。

## 项目说明
- 项目：benzhi-project-612356e0-4fd4-458d-80de-401fcd11c97b
- 项目用途：声呐测线质量验收台提供从测区任务创建、测线修订提交、确定性质量评估、异常整改与复验、独立复核、冻结清单到归档放行凭据签发的可审计 JSON HTTP 服务。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/sonarqa -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-612356e0-4fd4-458d-80de-401fcd11c97b-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-612356e0-4fd4-458d-80de-401fcd11c97b-arm64 linux/arm64
docker run -it benzhi-project-612356e0-4fd4-458d-80de-401fcd11c97b-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/sonarqa -selfcheck -addr=127.0.0.1:19081`
