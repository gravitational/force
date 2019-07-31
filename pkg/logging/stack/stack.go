package stack

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/logging"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

type Hook struct {
	client *logging.Client
	logger *logging.Logger
}

type Config struct {
	Context context.Context
	Creds   []byte
}

func (c *Config) CheckAndSetDefaults() error {
	if c.Context == nil {
		return trace.BadParameter("missing parameter Context")
	}
	if len(c.Creds) == 0 {
		return trace.BadParameter("missing parameter Creds")
	}
	return nil
}

// NewHook returns a new hook
func NewHook(cfg Config) (*Hook, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	type creds struct {
		ProjectID string `json:"project_id"`
	}
	var cred creds
	err := json.Unmarshal(cfg.Creds, &cred)
	if err != nil {
		return nil, trace.BadParameter("failed to parse credentials, unsupported format, expected JSON: %v", err)
	}
	if cred.ProjectID == "" {
		return nil, trace.BadParameter("credentials are missing project id")
	}

	client, err := logging.NewClient(cfg.Context,
		fmt.Sprintf("projects/%v", cred.ProjectID),
		option.WithCredentialsJSON(cfg.Creds))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Hook{client: client, logger: client.Logger("default")}, nil
}

// Fire is invoked by logrus and sends log to Stackdriver.
func (h *Hook) Fire(entry *logrus.Entry) error {
	if entry == nil {
		return trace.BadParameter("missing parameter entry")
	}
	h.logger.Log(convertEntry(entry))
	return nil
}

func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func convertEntry(entry *logrus.Entry) logging.Entry {
	return logging.Entry{
		Timestamp: entry.Time,
		Severity:  convertLevel(entry.Level),
		Labels:    convertLabels(entry.Data),
		Payload:   entry.Message,
	}
}

func convertLabels(data map[string]interface{}) map[string]string {
	labels := make(map[string]string, len(data))
	for k, v := range data {
		switch x := v.(type) {
		case string:
			labels[k] = x
		default:
			labels[k] = fmt.Sprintf("%v", v)
		}
	}
	return labels
}

func convertLevel(level logrus.Level) logging.Severity {
	switch level {
	case logrus.TraceLevel, logrus.DebugLevel:
		return logging.Debug
	case logrus.InfoLevel:
		return logging.Info
	case logrus.WarnLevel:
		return logging.Warning
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		return logging.Error
	default:
		return logging.Default
	}
}
