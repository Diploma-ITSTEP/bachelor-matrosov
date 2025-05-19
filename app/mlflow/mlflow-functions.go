package mlflow

import (
	"encoding/json"
	"fmt"
	"github.com/gidra39/mlflow-autostop/config"
	"github.com/gidra39/mlflow-autostop/messaging"
	"github.com/gidra39/mlflow-autostop/types"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func MonitorSpecificRun(runID string, config config.Config, debug bool) {
	for {
		run, err := getRunDetails(runID, config, debug)
		if err != nil {
			log.Printf("Error fetching run details: %v", err)
			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		if run.Run.Info.Status != "RUNNING" {
			log.Printf("Run %s is no longer active (status: %s), stopping monitoring",
				runID, run.Run.Info.Status)
			return
		}

		for _, metric := range run.Run.Data.Metrics {
			threshold, exists := config.MetricThresholds[metric.Key]
			if exists && metric.Value > threshold {
				msg := fmt.Sprintf("🚫 Stopping run %s: Metric %s = %.4f exceeded threshold %.4f",
					runID, metric.Key, metric.Value, threshold)
				log.Println(msg)

				err := messaging.SendNotification(msg, config)
				if err != nil {
					log.Printf("Failed to send notification: %v", err)
				}

				if err := stopRun(runID, config, debug); err != nil {
					log.Printf("Failed to stop run: %v", err)
				}

				return
			}
		}

		log.Printf("Run %s metrics are within acceptable thresholds", runID)
		time.Sleep(time.Duration(config.PollInterval) * time.Second)
	}
}

func MonitorExperiment(experimentID string, config config.Config, debug bool) {
	for {
		activeRuns, err := getActiveRunsInExperiment(experimentID, config, debug)
		if err != nil {
			log.Printf("Error fetching active runs: %v", err)
			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		if len(activeRuns.Runs) == 0 {
			log.Printf("No active runs found in experiment %s", experimentID)
			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		for _, run := range activeRuns.Runs {
			checkRunMetrics(run.Info.RunID, config, debug)
		}

		time.Sleep(time.Duration(config.PollInterval) * time.Second)
	}
}

func MonitorAllActiveRuns(config config.Config, debug bool) {
	for {
		activeRuns, err := getAllActiveRuns(config, debug)
		if err != nil {
			log.Printf("Error fetching active runs: %v", err)
			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		if len(activeRuns.Runs) == 0 {
			log.Println("No active runs found")

			if debug {
				allRuns, err := getAllRuns(config, debug)
				if err != nil {
					log.Printf("Debug: Error fetching all runs: %v", err)
				} else {
					log.Printf("Debug: Found %d total runs (any status)", len(allRuns.Runs))
					for i, run := range allRuns.Runs {
						if i < 5 { // Only show first 5 to avoid log flooding
							log.Printf("Debug: Run ID: %s, Status: %s",
								run.Info.RunID, run.Info.Status)
						}
					}
				}
			}

			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		for _, run := range activeRuns.Runs {
			checkRunMetrics(run.Info.RunID, config, debug)
		}

		time.Sleep(time.Duration(config.PollInterval) * time.Second)
	}
}

func checkRunMetrics(runID string, config config.Config, debug bool) {
	run, err := getRunDetails(runID, config, debug)
	if err != nil {
		log.Printf("Error fetching details for run %s: %v", runID, err)
		return
	}

	for _, metric := range run.Run.Data.Metrics {
		threshold, exists := config.MetricThresholds[metric.Key]
		if exists && metric.Value > threshold {
			msg := fmt.Sprintf("🚫 Stopping run %s: Metric %s = %.4f exceeded threshold %.4f",
				runID, metric.Key, metric.Value, threshold)
			log.Println(msg)

			err := messaging.SendNotification(msg, config)
			if err != nil {
				log.Printf("Failed to send notification: %v", err)
			}

			if err := stopRun(runID, config, debug); err != nil {
				log.Printf("Failed to stop run: %v", err)
			}

			return
		}
	}

	log.Printf("Run %s metrics are within acceptable thresholds", runID)
}

func getRunDetails(runID string, config config.Config, debug bool) (*types.GetRunResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/get?run_id=%s", config.MLflowTrackingURI, runID)

	if debug {
		log.Printf("Debug: Fetching run details from: %s", endpoint)
	}

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch run details: %v", err)
	}
	defer resp.Body.Close()

	if debug {
		log.Printf("Debug: Run details API response status: %s", resp.Status)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MLflow API returned status code %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if debug {
		log.Printf("Debug: Run details API response body: %s", string(body))
	}

	var runResponse types.GetRunResponse
	if err := json.Unmarshal(body, &runResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &runResponse, nil
}

func getActiveRunsInExperiment(experimentID string, config config.Config, debug bool) (*types.GetRunsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/search", config.MLflowTrackingURI)

	if debug {
		log.Printf("Debug: Searching for active runs in experiment %s at: %s",
			experimentID, endpoint)
	}

	requestBody := fmt.Sprintf(`{
		"experiment_ids": ["%s"],
		"filter": "attributes.status = 'RUNNING'"
	}`, experimentID)

	resp, err := http.Post(endpoint, "application/json", strings.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch active runs: %v", err)
	}
	defer resp.Body.Close()

	if debug {
		log.Printf("Debug: Active runs API response status: %s", resp.Status)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MLflow API returned status code %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if debug {
		log.Printf("Debug: Active runs API response body: %s", string(body))
	}

	var runsResponse types.GetRunsResponse
	if err := json.Unmarshal(body, &runsResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &runsResponse, nil
}

func getAllRuns(config config.Config, debug bool) (*types.GetRunsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/search", config.MLflowTrackingURI)

	if debug {
		log.Printf("Debug: Searching for all runs at: %s", endpoint)
	}

	requestBody := `{"max_results": 100}`

	resp, err := http.Post(endpoint, "application/json", strings.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch all runs: %v", err)
	}
	defer resp.Body.Close()

	if debug {
		log.Printf("Debug: All runs API response status: %s", resp.Status)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MLflow API returned status code %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if debug {
		log.Printf("Debug: All runs API response body: %s", string(body))
	}

	var runsResponse types.GetRunsResponse
	if err := json.Unmarshal(body, &runsResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &runsResponse, nil
}

func getAllActiveRuns(config config.Config, debug bool) (*types.GetRunsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/search", config.MLflowTrackingURI)

	if debug {
		log.Printf("Debug: Searching for active runs at: %s", endpoint)
	}

	requestBodies := []string{
		`{"filter": "attributes.status = 'RUNNING'"}`,
		`{"filter": "status = 'RUNNING'"}`,
		`{"run_view_type": "ACTIVE_ONLY"}`,
	}

	for i, requestBody := range requestBodies {
		if debug {
			log.Printf("Debug: Trying request format %d: %s", i+1, requestBody)
		}

		resp, err := http.Post(endpoint, "application/json", strings.NewReader(requestBody))
		if err != nil {
			if debug {
				log.Printf("Debug: Request format %d failed with error: %v", i+1, err)
			}
			continue
		}

		defer resp.Body.Close()

		if debug {
			log.Printf("Debug: Request format %d response status: %s", i+1, resp.Status)
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			if debug {
				log.Printf("Debug: Request format %d failed with status %d: %s",
					i+1, resp.StatusCode, string(bodyBytes))
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if debug {
				log.Printf("Debug: Failed to read response body for format %d: %v", i+1, err)
			}
			continue
		}

		var runsResponse types.GetRunsResponse
		if err := json.Unmarshal(body, &runsResponse); err != nil {
			if debug {
				log.Printf("Debug: Failed to parse response for format %d: %v", i+1, err)
			}
			continue
		}

		if len(runsResponse.Runs) > 0 {
			if debug {
				log.Printf("Debug: Successfully found %d active runs using format %d",
					len(runsResponse.Runs), i+1)
			}
			return &runsResponse, nil
		}

		if debug {
			log.Printf("Debug: Request format %d returned 0 active runs", i+1)
		}
	}

	return &types.GetRunsResponse{Runs: []struct {
		Info types.RunInfo `json:"info"`
		Data struct {
			Metrics []types.Metric `json:"metrics"`
		} `json:"data"`
	}{}}, nil
}

func stopRun(runID string, config config.Config, debug bool) error {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/update", config.MLflowTrackingURI)

	if debug {
		log.Printf("Debug: Stopping run %s at: %s", runID, endpoint)
	}

	requestBody := fmt.Sprintf(`{
		"run_id": "%s",
		"status": "FAILED"
	}`, runID)

	resp, err := http.Post(endpoint, "application/json", strings.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("failed to stop run: %v", err)
	}
	defer resp.Body.Close()

	if debug {
		log.Printf("Debug: Stop run API response status: %s", resp.Status)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MLflow API returned status code %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	log.Printf("Successfully stopped run %s", runID)
	return nil
}
