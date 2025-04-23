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
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Run the monitoring loop
	if *runID != "" {
		log.Printf("Monitoring specific run ID: %s", *runID)
		monitorSpecificRun(*runID, config)
	} else if *experimentID != "" {
		log.Printf("Monitoring active runs in experiment ID: %s", *experimentID)
		monitorExperiment(*experimentID, config)
	} else {
		log.Println("Monitoring all active runs")
		monitorAllActiveRuns(config)
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

	// Set defaults
	if config.PollInterval == 0 {
		config.PollInterval = 30 // Default to 30 seconds
	}

	return &config, nil
}

// Monitor a specific MLflow run
func monitorSpecificRun(runID string, config *types.AppConfig) {
	for {
		run, err := getRunDetails(runID, config)
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
				if err := stopRun(runID, config); err != nil {
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
func monitorExperiment(experimentID string, config *types.AppConfig) {
	for {
		activeRuns, err := getActiveRunsInExperiment(experimentID, config)
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
			checkRunMetrics(run.Info.RunID, config)
		}

		time.Sleep(time.Duration(config.PollInterval) * time.Second)
	}
}

func monitorAllActiveRuns(config *types.AppConfig) {
	for {
		activeRuns, err := getAllActiveRuns(config)
		if err != nil {
			log.Printf("Error fetching active runs: %v", err)
			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		if len(activeRuns.Runs) == 0 {
			log.Println("No active runs found")
			time.Sleep(time.Duration(config.PollInterval) * time.Second)
			continue
		}

		// Check each active run
		for _, run := range activeRuns.Runs {
			checkRunMetrics(run.Info.RunID, config)
		}

		time.Sleep(time.Duration(config.PollInterval) * time.Second)
	}
}

func checkRunMetrics(runID string, config *types.AppConfig) {
	run, err := getRunDetails(runID, config)
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
			if err := stopRun(runID, config); err != nil {
				log.Printf("Failed to stop run: %v", err)
			}

			return
		}
	}

	log.Printf("Run %s metrics are within acceptable thresholds", runID)
}

// Get details of a specific run
func getRunDetails(runID string, config *types.AppConfig) (*types.GetRunResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/get?run_id=%s", config.MLflowTrackingURI, runID)

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch run details: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MLflow API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var runResponse types.GetRunResponse
	if err := json.Unmarshal(body, &runResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &runResponse, nil
}

// Get all active runs in a specific experiment
func getActiveRunsInExperiment(experimentID string, config *types.AppConfig) (*types.GetRunsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/search", config.MLflowTrackingURI)

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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MLflow API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var runsResponse types.GetRunsResponse
	if err := json.Unmarshal(body, &runsResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &runsResponse, nil
}

// Get all active runs across all experiments
func getAllActiveRuns(config *types.AppConfig) (*types.GetRunsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/search", config.MLflowTrackingURI)

	// Create search payload for active runs
	requestBody := `{
		"filter": "attributes.status = 'RUNNING'"
	}`

	resp, err := http.Post(endpoint, "application/json", strings.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch active runs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MLflow API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var runsResponse types.GetRunsResponse
	if err := json.Unmarshal(body, &runsResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &runsResponse, nil
}

// Stop an MLflow run
func stopRun(runID string, config *types.AppConfig) error {
	endpoint := fmt.Sprintf("%s/api/2.0/mlflow/runs/update", config.MLflowTrackingURI)

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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MLflow API returned status code %d", resp.StatusCode)
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
		return fmt.Errorf("Telegram API returned status code %d", resp.StatusCode)
	}

	log.Println("Successfully sent Telegram notification")
	return nil
}
