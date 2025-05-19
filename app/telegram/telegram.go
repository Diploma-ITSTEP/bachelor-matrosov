package telegram

import (
	"fmt"
	"github.com/gidra39/mlflow-autostop/config"
	"log"
	"net/http"
	"net/url"
)

func SendTelegramNotification(message string, config config.Config) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.TelegramBotToken)

	params := url.Values{}
	params.Add("chat_id", config.TelegramChatID)
	params.Add("text", message)
	params.Add("parse_mode", "HTML")

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
