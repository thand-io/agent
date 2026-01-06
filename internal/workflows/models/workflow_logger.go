package models

import (
	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/workflow"
)

type Fields = logrus.Fields

// LogBuilder provides a fluent interface for logging
type LogBuilder struct {
	logger log.Logger
	fields []any
}

func (b *LogBuilder) WithError(err error) *LogBuilder {
	if err != nil {
		b.fields = append(b.fields, logrus.ErrorKey, err)
	}
	return b
}

func (b *LogBuilder) WithField(key string, value any) *LogBuilder {
	b.fields = append(b.fields, key, value)
	return b
}

func (b *LogBuilder) WithFields(fields Fields) *LogBuilder {
	for k, v := range fields {
		b.fields = append(b.fields, k, v)
	}
	return b
}

func (b *LogBuilder) Debug(msg string) {
	b.logger.Debug(msg, b.fields...)
}

func (b *LogBuilder) Info(msg string) {
	b.logger.Info(msg, b.fields...)
}

func (b *LogBuilder) Warn(msg string) {
	b.logger.Warn(msg, b.fields...)
}

func (b *LogBuilder) Error(msg string) {
	b.logger.Error(msg, b.fields...)
}

func (r *WorkflowTask) GetLogger() *LogBuilder {
	var logger log.Logger
	if r.HasTemporalContext() {
		logger = workflow.GetLogger(r.GetTemporalContext())
	} else if activity.IsActivity(r.GetContext()) {
		logger = activity.GetLogger(r.GetContext())
	} else {
		// Use the existing global logger
		logger = &LogrusAdapter{logger: logrus.StandardLogger()}
	}
	return &LogBuilder{logger: logger}
}

type LogrusAdapter struct {
	logger *logrus.Logger
}

func (l *LogrusAdapter) Debug(msg string, keyvals ...any) {
	l.toEntry(keyvals...).Debug(msg)
}

func (l *LogrusAdapter) Info(msg string, keyvals ...any) {
	l.toEntry(keyvals...).Info(msg)
}

func (l *LogrusAdapter) Warn(msg string, keyvals ...any) {
	l.toEntry(keyvals...).Warn(msg)
}

func (l *LogrusAdapter) Error(msg string, keyvals ...any) {
	l.toEntry(keyvals...).Error(msg)
}

func (l *LogrusAdapter) toEntry(keyvals ...any) *logrus.Entry {
	fields := make(logrus.Fields)
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			key, ok := keyvals[i].(string)
			if !ok {
				continue
			}
			fields[key] = keyvals[i+1]
		}
	}
	return l.logger.WithFields(fields)
}
