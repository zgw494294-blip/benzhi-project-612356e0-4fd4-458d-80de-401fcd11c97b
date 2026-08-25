package domain

import (
	"math"
	"strings"
)

func ValidateCreate(projectCode string, boundary AreaBoundary, coordinate string, thresholds QualityThresholds, lines []string, actor string) error {
	if strings.TrimSpace(projectCode) == "" {
		return FieldError("projectCode", "任务编号不能为空")
	}
	if strings.TrimSpace(actor) == "" {
		return FieldError("actor", "创建人不能为空")
	}
	if strings.TrimSpace(coordinate) == "" {
		return FieldError("coordinateReference", "坐标基准不能为空")
	}
	if err := validateBoundary(boundary); err != nil {
		return err
	}
	if err := validateThresholds(thresholds); err != nil {
		return err
	}
	if len(lines) == 0 {
		return FieldError("plannedLineIDs", "至少登记一条计划测线")
	}
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			return FieldError("plannedLineIDs", "测线编号不能为空")
		}
		if _, exists := seen[line]; exists {
			return FieldError("plannedLineIDs", "测线编号不得重复")
		}
		seen[line] = struct{}{}
	}
	return nil
}

func validateBoundary(boundary AreaBoundary) error {
	if len(boundary.Points) < 3 {
		return FieldError("areaBoundary", "测区边界至少需要三个坐标点")
	}
	for _, point := range boundary.Points {
		if math.IsNaN(point.Longitude) || math.IsNaN(point.Latitude) || point.Longitude < -180 || point.Longitude > 180 || point.Latitude < -90 || point.Latitude > 90 {
			return FieldError("areaBoundary", "测区边界坐标超出合法范围")
		}
	}
	area := 0.0
	for i, point := range boundary.Points {
		next := boundary.Points[(i+1)%len(boundary.Points)]
		area += point.Longitude*next.Latitude - next.Longitude*point.Latitude
	}
	if math.Abs(area) < 1e-12 {
		return FieldError("areaBoundary", "测区边界不能退化为直线")
	}
	return nil
}

func validateThresholds(value QualityThresholds) error {
	if value.MaxCoverageGapRatio < 0 || value.MaxCoverageGapRatio > 1 {
		return FieldError("qualityThresholds.maxCoverageGapRatio", "覆盖缺口阈值必须在 0 到 1 之间")
	}
	if value.MaxEchoGapRatio < 0 || value.MaxEchoGapRatio > 1 {
		return FieldError("qualityThresholds.maxEchoGapRatio", "回波缺口阈值必须在 0 到 1 之间")
	}
	if value.MaxHeadingDeviation <= 0 || value.MaxHeadingDeviation > 180 {
		return FieldError("qualityThresholds.maxHeadingDeviation", "航向偏差阈值必须大于 0 且不超过 180")
	}
	if value.MinPositionConfidence <= 0 || value.MinPositionConfidence > 1 {
		return FieldError("qualityThresholds.minPositionConfidence", "定位置信度阈值必须大于 0 且不超过 1")
	}
	if value.MaxSideLobeNoise < 0 || value.MaxSideLobeNoise > 1 {
		return FieldError("qualityThresholds.maxSideLobeNoise", "旁瓣噪声阈值必须在 0 到 1 之间")
	}
	return nil
}

func ValidateRevision(revision SonarLineRevision) error {
	if strings.TrimSpace(revision.RevisionID) == "" {
		return FieldError("revisionID", "修订编号不能为空")
	}
	if strings.TrimSpace(revision.LineID) == "" {
		return FieldError("lineID", "测线编号不能为空")
	}
	if len(revision.CoverageSamples) == 0 {
		return FieldError("coverageSamples", "覆盖样本不能为空")
	}
	last := -1.0
	for _, sample := range revision.CoverageSamples {
		if sample.AlongTrackMeter < 0 || sample.AlongTrackMeter <= last {
			return FieldError("coverageSamples", "覆盖样本里程必须非负并严格递增")
		}
		last = sample.AlongTrackMeter
	}
	if revision.EchoGapRatio < 0 || revision.EchoGapRatio > 1 || revision.PositionConfidence < 0 || revision.PositionConfidence > 1 || revision.SideLobeNoise < 0 || revision.SideLobeNoise > 1 {
		return FieldError("revision", "比例和置信度字段必须在 0 到 1 之间")
	}
	if revision.HeadingDeviation < 0 || revision.HeadingDeviation > 180 {
		return FieldError("headingDeviation", "航向偏差必须在 0 到 180 之间")
	}
	if strings.TrimSpace(revision.CalibrationRef) == "" || strings.TrimSpace(revision.SubmittedBy) == "" {
		return FieldError("revision", "校准引用和提交人不能为空")
	}
	return nil
}
