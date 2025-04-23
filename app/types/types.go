package types

// Configuration structure
type AppConfig struct {
	MLflowTrackingURI string             `json:"mlflow_tracking_uri"`
	TelegramBotToken  string             `json:"telegram_bot_token"`
	TelegramChatID    string             `json:"telegram_chat_id"`
	PollInterval      int                `json:"poll_interval_seconds"`
	MetricThresholds  map[string]float64 `json:"metric_thresholds"`
}

type RunInfo struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	ExperimentID string `json:"experiment_id"`
}

type Metric struct {
	Key       string  `json:"key"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
	Step      int     `json:"step"`
}

type GetRunsResponse struct {
	Runs []struct {
		Info RunInfo `json:"info"`
		Data struct {
			Metrics []Metric `json:"metrics"`
		} `json:"data"`
	} `json:"runs"`
}

type GetRunResponse struct {
	Run struct {
		Info RunInfo `json:"info"`
		Data struct {
			Metrics []Metric `json:"metrics"`
		} `json:"data"`
	} `json:"run"`
}
