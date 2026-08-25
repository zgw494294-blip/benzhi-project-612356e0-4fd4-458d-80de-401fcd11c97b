package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

func RunSelfcheck(ctx context.Context, server *Server, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("selfcheck 监听失败: %w", err)
	}
	actualAddress := listener.Addr().String()
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
	serveDone := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if err == http.ErrServerClosed {
			err = nil
		}
		serveDone <- err
	}()
	shutdown := func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return errorsJoin(httpServer.Shutdown(shutdownCtx), <-serveDone)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	base := "http://" + actualAddress
	post := func(path string, body any, actor application.Actor, key string, target any, expected int) error {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(encoded))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Actor-ID", actor.ID)
		request.Header.Set("X-Actor-Role", actor.Role)
		request.Header.Set("Idempotency-Key", key)
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != expected {
			return fmt.Errorf("selfcheck %s 返回 %d，期望 %d", path, response.StatusCode, expected)
		}
		return json.NewDecoder(response.Body).Decode(target)
	}
	get := func(path string, target any) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("selfcheck GET %s 返回 %d", path, response.StatusCode)
		}
		return json.NewDecoder(response.Body).Decode(target)
	}
	if err := waitHealthy(ctx, client, base+"/healthz"); err != nil {
		_ = shutdown()
		return err
	}
	processor := application.Actor{ID: "processor-selfcheck", Role: application.RoleProcessor}
	reviewer := application.Actor{ID: "reviewer-selfcheck", Role: application.RoleReviewer}
	archivist := application.Actor{ID: "archivist-selfcheck", Role: application.RoleArchivist}
	createBody := map[string]any{
		"projectCode":         "SELF-CHECK-001",
		"areaBoundary":        map[string]any{"points": []map[string]float64{{"longitude": 120, "latitude": 30}, {"longitude": 120.1, "latitude": 30}, {"longitude": 120.1, "latitude": 30.1}, {"longitude": 120, "latitude": 30.1}}},
		"coordinateReference": "CGCS2000",
		"qualityThresholds":   map[string]float64{"maxCoverageGapRatio": 0.2, "maxEchoGapRatio": 0.1, "maxHeadingDeviation": 5, "minPositionConfidence": 0.9, "maxSideLobeNoise": 0.1},
		"plannedLineIDs":      []string{"L-001"},
	}
	var created application.MutationResult
	if err := post("/api/v1/acceptances", createBody, processor, "selfcheck-create-001", &created, http.StatusCreated); err != nil {
		_ = shutdown()
		return err
	}
	acceptanceID := created.AcceptanceID
	version := created.Version
	revisionBody := map[string]any{"expectedVersion": version, "lineID": "L-001", "coverageSamples": []map[string]any{{"alongTrackMeter": 0, "covered": true}, {"alongTrackMeter": 100, "covered": true}, {"alongTrackMeter": 200, "covered": true}}, "echoGapRatio": 0.02, "headingDeviation": 1, "positionConfidence": 0.98, "sideLobeNoise": 0.02, "calibrationRef": "cal-2026-001"}
	if err := post("/api/v1/acceptances/"+acceptanceID+"/revisions", revisionBody, processor, "selfcheck-revision-001", &created, http.StatusCreated); err != nil {
		_ = shutdown()
		return err
	}
	version = created.Version
	if err := post("/api/v1/acceptances/"+acceptanceID+"/assessments", map[string]any{"expectedVersion": version}, processor, "selfcheck-assess-001", &created, http.StatusCreated); err != nil {
		_ = shutdown()
		return err
	}
	version = created.Version
	if err := post("/api/v1/acceptances/"+acceptanceID+"/review-decision", map[string]any{"expectedVersion": version, "approved": true, "note": "质量规则全部通过，批准冻结"}, reviewer, "selfcheck-review-001", &created, http.StatusOK); err != nil {
		_ = shutdown()
		return err
	}
	version = created.Version
	if err := post("/api/v1/acceptances/"+acceptanceID+"/freeze", map[string]any{"expectedVersion": version}, reviewer, "selfcheck-freeze-001", &created, http.StatusOK); err != nil {
		_ = shutdown()
		return err
	}
	version = created.Version
	if err := post("/api/v1/acceptances/"+acceptanceID+"/release", map[string]any{"expectedVersion": version}, archivist, "selfcheck-release-001", &created, http.StatusCreated); err != nil {
		_ = shutdown()
		return err
	}
	var release domain.ArchiveRelease
	if err := get("/api/v1/acceptances/"+acceptanceID+"/release", &release); err != nil {
		_ = shutdown()
		return err
	}
	if err := domain.VerifyRelease(release); err != nil {
		_ = shutdown()
		return err
	}
	if err := shutdown(); err != nil {
		return err
	}
	return nil
}

func waitHealthy(ctx context.Context, client *http.Client, url string) error {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("selfcheck 健康检查超时")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func errorsJoin(first, second error) error {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return fmt.Errorf("%v; %v", first, second)
}
