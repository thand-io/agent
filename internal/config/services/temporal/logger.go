package temporal

import (
	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/log"
)

// logrusLogger is an adapter that implements the Temporal log.Logger interface using logrus
type logrusLogger struct {
	logger *logrus.Logger
}

func newLogrusLogger() log.Logger {
	return &logrusLogger{
		logger: logrus.StandardLogger(),
	}
}

func (l *logrusLogger) Debug(msg string, keyvals ...any) {
	l.logger.WithFields(l.fieldsFromKeyvals(keyvals...)).Debug(msg)
}

func (l *logrusLogger) Info(msg string, keyvals ...any) {
	l.logger.WithFields(l.fieldsFromKeyvals(keyvals...)).Info(msg)
}

func (l *logrusLogger) Warn(msg string, keyvals ...any) {
	l.logger.WithFields(l.fieldsFromKeyvals(keyvals...)).Warn(msg)
}

func (l *logrusLogger) Error(msg string, keyvals ...any) {
	l.logger.WithFields(l.fieldsFromKeyvals(keyvals...)).Error(msg)
}

func (l *logrusLogger) fieldsFromKeyvals(keyvals ...any) logrus.Fields {
	fields := logrus.Fields{}
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			if key, ok := keyvals[i].(string); ok {
				fields[key] = keyvals[i+1]
			}
		}
	}
	return fields
}
