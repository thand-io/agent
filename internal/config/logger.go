package config

import "github.com/sirupsen/logrus"

type thandLogger struct {

	// Create a stack to psuh on new events and pop off older ones
	// When an error/warn event is fired, flush the stack to the logger
	eventStack []*logrus.Entry
}

func NewThandLogger() *thandLogger {
	return &thandLogger{}
}

func (t *thandLogger) Fire(entry *logrus.Entry) error {

	// Push the new event onto the stack
	t.eventStack = append(t.eventStack, entry)

	return nil
}

func (t *thandLogger) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
	}
}

func (t *thandLogger) Clear() {
	t.eventStack = []*logrus.Entry{}
}

func (t *thandLogger) GetEvents() []*logrus.Entry {
	return t.eventStack
}
