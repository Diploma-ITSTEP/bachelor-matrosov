package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gidra39/mlflow-autostop/config"
	"log"
	"net/http"
)

type SlackMessage struct {
	Text string `json:"text"`
}

func SendSlackNotification(message string, config config.Config) error {
	if config.SlackWebhookURL == "" {
		return fmt.Errorf("slack webhook URL is not configured")
	}

	slackMessage := SlackMessage{
		Text: message,
	}

	payload, err := json.Marshal(slackMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal slack message: %v", err)
	}

	resp, err := http.Post(config.SlackWebhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack API returned status code %d", resp.StatusCode)
	}

	log.Println("Successfully sent Slack notification")
	return nil
}
