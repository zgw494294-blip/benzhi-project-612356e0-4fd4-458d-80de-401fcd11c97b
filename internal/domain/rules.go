package domain

import "sort"

const CurrentRuleSetVersion = "sonar-quality-rules/1.0.0"

func EvaluateRules(revision SonarLineRevision, thresholds QualityThresholds) []RuleOutcome {
	uncovered := 0
	for _, sample := range revision.CoverageSamples {
		if !sample.Covered {
			uncovered++
		}
	}
	coverageGap := float64(uncovered) / float64(len(revision.CoverageSamples))
	outcomes := []RuleOutcome{
		newOutcome(revision, "COVERAGE_GAP", coverageGap, thresholds.MaxCoverageGapRatio, "lte", "覆盖缺口比例不得超过阈值"),
		newOutcome(revision, "ECHO_GAP", revision.EchoGapRatio, thresholds.MaxEchoGapRatio, "lte", "回波缺口比例不得超过阈值"),
		newOutcome(revision, "HEADING_DEVIATION", revision.HeadingDeviation, thresholds.MaxHeadingDeviation, "lte", "航向偏差不得超过阈值"),
		newOutcome(revision, "POSITION_CONFIDENCE", revision.PositionConfidence, thresholds.MinPositionConfidence, "gte", "定位置信度不得低于阈值"),
		newOutcome(revision, "SIDE_LOBE_NOISE", revision.SideLobeNoise, thresholds.MaxSideLobeNoise, "lte", "旁瓣噪声不得超过阈值"),
	}
	return outcomes
}

func newOutcome(revision SonarLineRevision, code string, observed, threshold float64, comparator, description string) RuleOutcome {
	passed := observed <= threshold
	if comparator == "gte" {
		passed = observed >= threshold
	}
	return RuleOutcome{LineID: revision.LineID, RevisionID: revision.RevisionID, RuleCode: code, Passed: passed, Observed: observed, Threshold: threshold, Comparator: comparator, Description: description}
}

func SortOutcomes(outcomes []RuleOutcome) {
	sort.Slice(outcomes, func(i, j int) bool {
		if outcomes[i].LineID == outcomes[j].LineID {
			return outcomes[i].RuleCode < outcomes[j].RuleCode
		}
		return outcomes[i].LineID < outcomes[j].LineID
	})
}
