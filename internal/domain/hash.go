package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

func StableHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("编码哈希输入: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type assessmentDigest struct {
	AcceptanceID string        `json:"acceptanceID"`
	RevisionRefs [][2]string   `json:"revisionRefs"`
	RuleVersion  string        `json:"ruleVersion"`
	Outcomes     []RuleOutcome `json:"outcomes"`
}

func AssessmentHash(id string, refs map[string]string, version string, outcomes []RuleOutcome) (string, error) {
	keys := make([]string, 0, len(refs))
	for key := range refs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([][2]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, [2]string{key, refs[key]})
	}
	return StableHash(assessmentDigest{AcceptanceID: id, RevisionRefs: pairs, RuleVersion: version, Outcomes: outcomes})
}

type manifestDigest struct {
	AcceptanceID     string       `json:"acceptanceID"`
	ProjectCode      string       `json:"projectCode"`
	Lines            []FrozenLine `json:"lines"`
	AssessmentID     string       `json:"assessmentID"`
	AssessmentHash   string       `json:"assessmentHash"`
	ApprovedFindings []string     `json:"approvedFindings"`
	FrozenBy         string       `json:"frozenBy"`
	FrozenAt         string       `json:"frozenAt"`
}

func ComputeManifestHash(manifest FrozenManifest) (string, error) {
	lines := append([]FrozenLine(nil), manifest.Lines...)
	sort.Slice(lines, func(i, j int) bool { return lines[i].LineID < lines[j].LineID })
	findings := append([]string(nil), manifest.ApprovedFindings...)
	sort.Strings(findings)
	return StableHash(manifestDigest{
		AcceptanceID: manifest.AcceptanceID, ProjectCode: manifest.ProjectCode,
		Lines: lines, AssessmentID: manifest.AssessmentID, AssessmentHash: manifest.AssessmentHash,
		ApprovedFindings: findings, FrozenBy: manifest.FrozenBy, FrozenAt: manifest.FrozenAt.UTC().Format(time.RFC3339Nano),
	})
}

type releaseDigest struct {
	ReleaseID    string `json:"releaseID"`
	AcceptanceID string `json:"acceptanceID"`
	ProjectCode  string `json:"projectCode"`
	ManifestHash string `json:"manifestHash"`
	IssuedBy     string `json:"issuedBy"`
	IssuedAt     string `json:"issuedAt"`
}

func ComputeReleaseDigest(release ArchiveRelease) (string, error) {
	return StableHash(releaseDigest{
		ReleaseID: release.ReleaseID, AcceptanceID: release.AcceptanceID, ProjectCode: release.ProjectCode,
		ManifestHash: release.ManifestHash, IssuedBy: release.IssuedBy, IssuedAt: release.IssuedAt.UTC().Format(time.RFC3339Nano),
	})
}
