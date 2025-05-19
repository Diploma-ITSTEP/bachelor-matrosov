package config

import (
	"github.com/gidra39/mlflow-autostop/validation"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

var ErrFileNotFound = errors.New("file not found")

// Config contains all application configuration settings
// config/config.go - update the Config struct
type Config struct {
	MLflowTrackingURI           string             `json:"MLFLOW_TRACKING_URI" koanf:"MLFLOW_TRACKING_URI" validate:"required"`
	TelegramBotToken            string             `json:"TELEGRAM_BOT_TOKEN" koanf:"TELEGRAM_BOT_TOKEN"`
	TelegramChatID              string             `json:"TELEGRAM_CHAT_ID" koanf:"TELEGRAM_CHAT_ID"`
	PollInterval                int                `json:"POLL_INTERVAL_SECONDS" koanf:"POLL_INTERVAL_SECONDS" validate:"required,gt=0"`
	MetricThresholds            map[string]float64 `json:"METRIC_THRESHOLDS" koanf:"METRIC_THRESHOLDS"`
	TelegramBotDefaultChannelID int                `json:"TELEGRAM_BOT_DEFAULT_CHANNEL_ID" koanf:"TELEGRAM_BOT_DEFAULT_CHANNEL_ID"`
	SlackWebhookURL             string             `json:"SLACK_WEBHOOK_URL" koanf:"SLACK_WEBHOOK_URL"`
	MessageChannels             string             `json:"MESSAGE_CHANNELS" koanf:"MESSAGE_CHANNELS" default:"TELEGRAM"`
}

func Load(configFile string) Config {
	k := koanf.New(".")

	if configFile != "" {
		if err := k.Load(file.Provider(configFile), nil); err != nil {
			log.Warn().Err(err).Str("file", configFile).Msg("unable to load config file")
		} else {
			log.Info().Str("file", configFile).Msg("loaded configuration from file")
		}
	}

	// Load from environment variables (higher priority)
	if err := k.Load(env.Provider("", ".", func(s string) string { return s }), nil); err != nil {
		log.Fatal().Err(err).Caller().Msg("koanf: error loading env")
	}

	config := Config{}

	if err := k.Unmarshal("", &config); err != nil {
		log.Fatal().Err(err).Caller().Msg("koanf: error unmarshalling config")
	}

	if err := validation.Validate.Struct(config); err != nil {
		log.Fatal().Err(err).Caller().Msg("koanf: error validating config")
	}
	return config
}

func SearchUpwardsForFile(filename string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if wd == "/" {
			return "", errors.Wrap(ErrFileNotFound, filename)
		}

		file := filepath.Join(wd, filename)
		if _, err := os.Stat(file); err == nil {
			return file, nil
		}

		wd = filepath.Dir(wd)
	}
}

func LoadDotEnv(fileName string) {
	file, err := SearchUpwardsForFile(fileName)
	if err != nil {
		log.Warn().Err(err).Msgf("failed to find %s file", fileName)
		return
	}

	if err := godotenv.Load(file); err != nil {
		log.Fatal().Err(err).Msg("invalid .env file")
	}

	log.Info().Msgf("loaded environment variables from %s", file)
}

// LoadConfig is the main entry point for configuration loading
func LoadConfig(envFile string, configFiles ...string) Config {
	if envFile != "" {
		LoadDotEnv(envFile)
	}

	for _, configFile := range configFiles {
		foundFile, err := SearchUpwardsForFile(configFile)
		if err == nil {
			return Load(foundFile)
		}
	}

	// If no config file found, load from environment only
	return Load("")
}
