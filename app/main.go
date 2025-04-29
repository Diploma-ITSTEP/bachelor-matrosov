package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gidra39/mlflow-autostop/types"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	// Parse command line arguments
	configPath := flag.String("config", "config.json", "Path to configuration file")
	runID := flag.String("run-id", "", "MLflow run ID to monitor (optional)")
	experimentID := flag.String("experiment-id", "", "MLflow experiment ID to monitor (optional)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Enable debug mode if requested
	if *debug {
		log.Println("Debug mode enabled - verbose logging activated")
	}

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Log the MLflow tracking URI for debugging
	log.Printf("Using MLflow tracking URI: %s", config.MLflowTrackingURI)

	// Run the monitoring loop
	if *runID != "" {
		log.Printf("Monitoring specific run ID: %s", *runID)
		monitorSpecificRun(*runID, config, *debug)
	} else if *experimentID != "" {
		log.Printf("Monitoring active runs in experiment ID: %s", *experimentID)
		monitorExperiment(*experimentID, config, *debug)
	} else {
		log.Println("Monitoring all active runs")
		monitorAllActiveRuns(config, *debug)
	}
}

// Load configuration from JSON file
func loadConfig(path string) (*types.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	var config types.AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	// Set defaults if needed
	if config.PollInterval == 0 {
		config.PollInterval = 30 // Default to 30 seconds
	}

	return &config, nil
}

// Monitor a specific MLflow run
func monitorSpecificRun(runID string, config *types.AppConfig, debug bool) {
	for {
		run, err := getRunDetails(runID, config, debug)
		if err != nil {
			log.Printf("Error fetching run details: %v", err)
			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		// Check if run is still active
		if run.Run.Info.Status != "RUNNING" {
			log.Printf("Run %s is no longer active (status: %s), stopping monitoring",
				runID, run.Run.Info.Status)
			return
		}

		// Check metrics against thresholds
		for _, metric := range run.Run.Data.Metrics {
			threshold, exists := config.MetricThresholds[metric.Key]
			if exists && metric.Value > threshold {
				msg := fmt.Sprintf("🚫 Stopping run %s: Metric %s = %.4f exceeded threshold %.4f",
					runID, metric.Key, metric.Value, threshold)
				log.Println(msg)

				// Send Telegram notification
				if err := sendTelegramNotification(msg, config); err != nil {
					log.Printf("Failed to send Telegram notification: %v", err)
				}

				// Stop the run
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

// Monitor all active runs in an experiment
func monitorExperiment(experimentID string, config *types.AppConfig, debug bool) {
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

		// Check each active run
		for _, run := range activeRuns.Runs {
			checkRunMetrics(run.Info.RunID, config, debug)
		}

		time.Sleep(time.Duration(config.PollInterval) * time.Second)
	}
}

// Monitor all active runs across all experiments
func monitorAllActiveRuns(config *types.AppConfig, debug bool) {
	for {
		activeRuns, err := getAllActiveRuns(config, debug)
		if err != nil {
			log.Printf("Error fetching active runs: %v", err)
			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		if len(activeRuns.Runs) == 0 {
			log.Println("No active runs found")

			// In debug mode, list all runs regardless of status to verify API access
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

		// Check each active run
		for _, run := range activeRuns.Runs {
			checkRunMetrics(run.Info.RunID, config, debug)
		}

		time.Sleep(time.Duration(config.PollInterval) * time.Second)
	}
}

// Check metrics for a specific run against thresholds
func checkRunMetrics(runID string, config *types.AppConfig, debug bool) {
	run, err := getRunDetails(runID, config, debug)
	if err != nil {
		log.Printf("Error fetching details for run %s: %v", runID, err)
		return
	}

	// Check metrics against thresholds
	for _, metric := range run.Run.Data.Metrics {
		threshold, exists := config.MetricThresholds[metric.Key]
		if exists && metric.Value > threshold {
			msg := fmt.Sprintf("🚫 Stopping run %s: Metric %s = %.4f exceeded threshold %.4f",
				runID, metric.Key, metric.Value, threshold)
			log.Println(msg)

			// Send Telegram notification
			if err := sendTelegramNotification(msg, config); err != nil {
				log.Printf("Failed to send Telegram notification: %v", err)
			}

			// Stop the run
			if err := stopRun(runID, config, debug); err != nil {
				log.Printf("Failed to stop run: %v", err)
			}

			return
		}
	}

	log.Printf("Run %s metrics are within acceptable thresholds", runID)
}

// Get details of a specific run
func getRunDetails(runID string, config *types.AppConfig, debug bool) (*types.GetRunResponse, error) {
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

// Get all active runs in a specific experiment
func getActiveRunsInExperiment(experimentID string, config *types.AppConfig, debug bool) (*types.GetRunsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/search", config.MLflowTrackingURI)

	if debug {
		log.Printf("Debug: Searching for active runs in experiment %s at: %s",
			experimentID, endpoint)
	}

	// Create search payload
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

// Get all runs regardless of status (for debugging)
func getAllRuns(config *types.AppConfig, debug bool) (*types.GetRunsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/search", config.MLflowTrackingURI)

	if debug {
		log.Printf("Debug: Searching for all runs at: %s", endpoint)
	}

	// Create search payload with no filters
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

// Get all active runs across all experiments
func getAllActiveRuns(config *types.AppConfig, debug bool) (*types.GetRunsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/search", config.MLflowTrackingURI)

	if debug {
		log.Printf("Debug: Searching for active runs at: %s", endpoint)
	}

	// Create search payload with different filter formats to try
	// MLflow API might have different versions or implementations
	requestBodies := []string{
		`{"filter": "attributes.status = 'RUNNING'"}`,
		`{"filter": "status = 'RUNNING'"}`,
		`{"run_view_type": "ACTIVE_ONLY"}`,
	}

	// Try each request body format
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

		// If we found any runs, return this response
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

	// If all formats returned 0 runs, return the empty response
	return &types.GetRunsResponse{Runs: []struct {
		Info types.RunInfo `json:"info"`
		Data struct {
			Metrics []types.Metric `json:"metrics"`
		} `json:"data"`
	}{}}, nil
}

// Stop an MLflow run
func stopRun(runID string, config *types.AppConfig, debug bool) error {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/update", config.MLflowTrackingURI)

	if debug {
		log.Printf("Debug: Stopping run %s at: %s", runID, endpoint)
	}

	// Create payload to stop the run
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

// Send notification to Telegram
func sendTelegramNotification(message string, config *types.AppConfig) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.TelegramBotToken)

	// Create URL parameters
	params := url.Values{}
	params.Add("chat_id", config.TelegramChatID)
	params.Add("text", message)
	params.Add("parse_mode", "Markdown")

	// Send request
	resp, err := http.PostForm(endpoint, params)
	if err != nil {
		return fmt.Errorf("failed to send Telegram notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status code %d", resp.StatusCode)
	}

	log.Println("Successfully sent Telegram notification")
	return nil
}
