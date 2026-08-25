package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

func readEvents(path string) ([]eventRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取事件日志: %w", err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return nil, fmt.Errorf("事件日志尾部记录不完整: %s", path)
	}
	lines := bytes.Split(data, []byte{'\n'})
	events := make([]eventRecord, 0, len(lines))
	previous := ""
	for index, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event eventRecord
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("事件日志第 %d 行 JSON 无效: %w", index+1, err)
		}
		expectedSequence := int64(len(events) + 1)
		if event.Sequence != expectedSequence {
			return nil, fmt.Errorf("事件序号不连续: 期望 %d，实际 %d", expectedSequence, event.Sequence)
		}
		if event.PreviousHash != previous {
			return nil, fmt.Errorf("事件 %d 前序校验和不匹配", event.Sequence)
		}
		hash, err := calculateEventHash(event)
		if err != nil {
			return nil, fmt.Errorf("计算事件 %d 校验和: %w", event.Sequence, err)
		}
		if hash != event.Hash {
			return nil, fmt.Errorf("事件 %d 校验链损坏", event.Sequence)
		}
		if event.State == nil || event.State.ID != event.AcceptanceID || event.State.Version != event.AggregateVersion {
			return nil, fmt.Errorf("事件 %d 的任务投影与元数据不一致", event.Sequence)
		}
		if err := event.State.ValidateIntegrity(); err != nil {
			return nil, fmt.Errorf("事件 %d 的业务投影无效: %w", event.Sequence, err)
		}
		events = append(events, event)
		previous = event.Hash
	}
	return events, nil
}

func readSnapshot(path string) (*snapshotFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取投影快照: %w", err)
	}
	var snapshot snapshotFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("投影快照 JSON 无效: %w", err)
	}
	if snapshot.SchemaVersion != snapshotSchemaVersion {
		return nil, fmt.Errorf("不支持的快照 schemaVersion: %d", snapshot.SchemaVersion)
	}
	hash, err := calculateSnapshotHash(snapshot)
	if err != nil {
		return nil, fmt.Errorf("计算快照校验和: %w", err)
	}
	if hash != snapshot.Checksum {
		return nil, fmt.Errorf("投影快照校验失败")
	}
	if snapshot.Aggregates == nil {
		return nil, fmt.Errorf("投影快照缺少 aggregates")
	}
	return &snapshot, nil
}

func validateSnapshotAnchor(snapshot *snapshotFile, events []eventRecord) error {
	if snapshot == nil {
		return nil
	}
	if snapshot.EventSequence < 0 || snapshot.EventSequence > int64(len(events)) {
		return fmt.Errorf("快照事件序号超出日志范围")
	}
	expectedHash := ""
	if snapshot.EventSequence > 0 {
		expectedHash = events[snapshot.EventSequence-1].Hash
	}
	if snapshot.EventHash != expectedHash {
		return fmt.Errorf("快照锚点与事件校验链不匹配")
	}
	return nil
}
