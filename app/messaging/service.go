package messaging

import (
	"github.com/gidra39/mlflow-autostop/config"
	"github.com/gidra39/mlflow-autostop/slack"
	"github.com/gidra39/mlflow-autostop/telegram"
	"strings"
)

const (
	ChannelTelegram = "TELEGRAM"
	ChannelSlack    = "SLACK"
	ChannelBoth     = "BOTH"
)

func SendNotification(message string, config config.Config) error {
	channels := strings.ToUpper(config.MessageChannels)
	if channels == "" {
		channels = ChannelTelegram
	}

	var telegramErr, slackErr error

	if channels == ChannelTelegram || channels == ChannelBoth {
		telegramErr = telegram.SendTelegramNotification(message, config)
	}

	if channels == ChannelSlack || channels == ChannelBoth {
		slackErr = slack.SendSlackNotification(message, config)
	}

	if channels == ChannelBoth {
		if telegramErr != nil && slackErr != nil {
			return telegramErr
		}
		return nil
	} else if channels == ChannelTelegram {
		return telegramErr
	} else if channels == ChannelSlack {
		return slackErr
	}

	return nil
}
