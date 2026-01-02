package tasks

import (
	"reflect"
	"strings"
	"sync"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

func NewTaskRegistry(config models.ConfigImpl) *TaskRegistry {
	return &TaskRegistry{
		config:   config,
		handlers: make(map[string]Task),
	}
}

// TaskRegistry manages custom task handlers
type TaskRegistry struct {
	config   models.ConfigImpl
	handlers map[string]Task
	mu       sync.RWMutex
}

func (r *TaskRegistry) RegisterTasks(handlers ...Task) {
	for _, handler := range handlers {
		r.RegisterTask(handler)
	}
}

// RegisterTask registers a custom handler for a specific task type
func (r *TaskRegistry) RegisterTask(handler Task) {

	r.mu.Lock()
	defer r.mu.Unlock()

	taskName := getTaskName(handler)

	logrus.WithFields(logrus.Fields{
		"taskType": taskName,
		"taskName": handler.GetName(),
	}).Info("Registering custom task handler")

	r.handlers[taskName] = handler

}

// GetTaskHandler retrieves a handler for a specific task type
func (r *TaskRegistry) GetTaskHandler(taskType *model.TaskItem) (Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	taskName := getTaskName(taskType.Task)

	handler, exists := r.handlers[taskName]
	return handler, exists
}

func getTaskName(taskType any) string {

	t := reflect.TypeOf(taskType)

	// Handle pointer types
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	taskName := strings.ToLower(t.Name())
	taskName = strings.TrimSuffix(taskName, "task")

	return taskName

}
