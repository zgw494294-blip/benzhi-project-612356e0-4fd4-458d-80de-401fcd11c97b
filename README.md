# 声呐测线质量验收台

声呐测线质量验收台面向声呐数据处理员、海洋测绘质量复核员和测绘成果归档负责人，使用 JSON HTTP API 将测区验收任务从测线登记推进到质量评估、异常整改、独立复核、数据集冻结和归档放行。每个任务都有唯一编号和乐观并发版本；测线修订不可覆盖，写操作使用 `expectedVersion` 和 `Idempotency-Key`，冻结清单与放行凭据可离线校验。

## 状态流程

任务创建后进入 `Collecting`。所有计划测线提交修订后可执行确定性质量评估并进入 `Reviewing`。未通过规则会生成阻断异常，处理员必须登记原因、处置说明和更新后的证据修订；复验通过后由独立复核员逐项确认并作出决定。退回进入 `Rework`，补交修订后再次评估。全部异常闭环且复核通过后进入 `Frozen`，冻结清单哈希固定测线和评估引用；归档负责人签发不可变凭据后进入 `Released`。

## 构建、运行与测试

标准构建命令：

```text
go build ./cmd/sonarqa
```

标准测试命令：

```text
go test ./...
```

正常启动命令：

```text
go run ./cmd/sonarqa -addr=127.0.0.1:19081 -data=./data
```

启动入口默认监听 `127.0.0.1:19081`。显式 `-addr=127.0.0.1:<port>` 优先级最高；未提供 `-addr` 时，如果设置了 `PORT` 端口号，则绑定 `127.0.0.1:<PORT>`；否则使用默认地址。服务拒绝空地址、非法端口、`0.0.0.0` 和其他非回环地址。

有界自检会启动真实回环 HTTP 监听并自行关闭：

```text
go run ./cmd/sonarqa -selfcheck -addr=127.0.0.1:19081
```

`-data` 目录保存带递增序号和校验链的 `events.jsonl`，以及带 `schemaVersion`、原子替换写入的 `snapshot.json`。启动时会校验事件链、快照锚点和业务投影完整性，并重放快照之后的事件。

## API 入口

主要资源位于 `/api/v1/acceptances`。任务列表支持 `status`、`projectCode`、`createdFrom`、`createdTo`、`page` 和 `pageSize` 筛选分页，并返回状态统计。任务子资源提供测线 `/lines/{lineID}/revisions` 修订历史、`/assessments` 规则摘要、`/review-workbench` 复核工作台和 `/audit` 校验链审计。

业务写入包括提交 `/revisions`、执行 `/assessments`、单项 `/findings/{findingID}/remediation`、原子批量 `/findings/remediation-batch`、逐项 `/review`、`/review-decision`、`/freeze` 和 `/release`。操作者通过 `X-Actor-ID` 与 `X-Actor-Role` 传递，所有变更请求都需要 `Idempotency-Key`。
