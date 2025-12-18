package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-resty/resty/v2"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

type execInfo struct {
	execution *models.WorkflowExecutionInfo
}

type errorMsg struct {
	err error
}

type tuiModel struct {
	workflowID  string
	execution   *models.WorkflowExecutionInfo
	spinner     spinner.Model
	loading     bool
	err         error
	lastUpdate  time.Time
	approvedAt  *time.Time
	quitting    bool
	liveUpdates bool
	serverUrl   string
	authToken   string
}

func newTuiModel(workflowID, serverUrl, authToken string) tuiModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#3b82f6"))

	return tuiModel{
		workflowID:  workflowID,
		spinner:     s,
		loading:     true,
		liveUpdates: true,
		serverUrl:   serverUrl,
		authToken:   authToken,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchStatus)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case execInfo:

		execution := msg.execution

		if execution == nil {
			m.err = fmt.Errorf("no execution data received")
			return m, tea.Quit
		}

		m.loading = false

		m.execution = execution
		m.lastUpdate = time.Now()

		if execution.Approved != nil && *execution.Approved && m.approvedAt == nil {
			m.approvedAt = execution.GetAuthorizationTime()
		}

		// Continue polling if workflow is still running
		if m.isWorkflowRunning() {
			return m, tea.Tick(time.Second*3, func(t time.Time) tea.Msg {
				return m.fetchStatus()
			})
		} else {
			// Workflow has completed
			return m, tea.Quit
		}

	case errorMsg:
		m.loading = false
		m.err = msg.err
		m.liveUpdates = false
		return m, tea.Quit

	case tea.WindowSizeMsg:
		return m, nil
	}

	return m, nil
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}

	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %s", m.err.Error())) + "\n"
	}

	if m.loading {
		return fmt.Sprintf("\n %s Fetching workflow status...\n\n", m.spinner.View())
	}

	if m.execution == nil {
		return errorStyle.Render("No execution data available") + "\n"
	}

	var content strings.Builder

	var statusColor lipgloss.Color
	var statusText string
	switch strings.ToLower(m.execution.Status) {
	case "completed":
		statusColor = lipgloss.Color("#10b981")
		statusText = "COMPLETED"
	case "failed", "faulted":
		statusColor = lipgloss.Color("#ef4444")
		statusText = "FAILED"
	case "cancelled":
		statusColor = lipgloss.Color("#f59e0b")
		statusText = "CANCELLED"
	case "running":
		statusColor = lipgloss.Color("#3b82f6")
		statusText = "RUNNING"
	default:
		statusColor = lipgloss.Color("#6b7280")
		statusText = strings.ToUpper(m.execution.Status)
	}

	// Title
	content.WriteString(workflowTitleStyle.Render("Workflow Execution Status: "))
	statusBadge := statusBadgeStyle.Copy().Background(statusColor).Render(statusText)
	content.WriteString(statusBadge)
	content.WriteString("\n\n")

	// Current status section
	content.WriteString(m.renderStatusSection())
	content.WriteString("\n\n")

	// Last updated
	if !m.lastUpdate.IsZero() {
		content.WriteString(fmt.Sprintf("Last updated: %s", m.lastUpdate.Format("15:04:05")))
		content.WriteString("\n")
	}

	// Instructions
	content.WriteString("Press q to quit")
	content.WriteString("\n")

	return content.String()
}

func (m tuiModel) renderStatusSection() string {
	var section strings.Builder

	// Current task status
	currentTask := m.execution.Task
	if len(currentTask) == 0 {
		currentTask = "Initializing"
	}

	// Approval status
	approvalBadge := pendingApprovalStyle.Render("PENDING APPROVAL")
	if m.execution.Approved != nil {
		if *m.execution.Approved {
			approvalBadge = approvedStyle.Render("APPROVED")
		} else {
			approvalBadge = rejectedStyle.Render("REJECTED")
		}
	}
	section.WriteString(fmt.Sprintf("Approval:     %s", approvalBadge))
	section.WriteString("\n")

	if m.execution.Duration > 0 {

		totalDuration := time.Duration(m.execution.Duration) * time.Second
		// Format duration in PT1H format
		durationStr := common.FormatDuration(totalDuration)
		section.WriteString(fmt.Sprintf("Duration:      %s", durationStr))

		if m.approvedAt != nil {

			expirationTime := m.approvedAt.Add(totalDuration)

			// Calculate remaining time until expiration
			remaining := max(time.Until(expirationTime), 0)

			// Format remaining time in human readable format
			remainingStr := common.FormatDurationRemaining(remaining)

			section.WriteString(fmt.Sprintf(" (%s remaining)", remainingStr))
		}

		section.WriteString("\n")
	}

	// Current task
	section.WriteString("Current Task:  ")
	if m.isWorkflowRunning() {
		section.WriteString(fmt.Sprintf("%s %s", m.spinner.View(), currentTask))
	} else {
		section.WriteString(currentTask)
	}
	section.WriteString("\n")

	// User info
	if len(m.execution.Identities) > 0 {
		identities := m.execution.Identities
		displayStr := ""
		if len(identities) > 2 {
			displayStr = fmt.Sprintf("%s, %s + (%d more)", identities[0].String(), identities[1].String(), len(identities)-2)
		} else {
			strs := make([]string, len(identities))
			for i, id := range identities {
				strs[i] = id.String()
			}
			displayStr = strings.Join(strs, ", ")
		}
		section.WriteString(fmt.Sprintf("Identity:      %s", displayStr))
		section.WriteString("\n")
	} else {
		section.WriteString(fmt.Sprintf("Identity:      self (%s)", m.execution.User))
		section.WriteString("\n")
	}

	if len(m.execution.Role) != 0 {
		section.WriteString(fmt.Sprintf("Role:          %s", m.execution.Role))
		section.WriteString("\n")
	}

	return section.String()
}

func (m tuiModel) isWorkflowRunning() bool {
	if m.execution == nil {
		return false
	}
	status := strings.ToLower(m.execution.Status)
	return status == "running" || status == "pending" || status == "waiting" || status == "suspended"
}

func (m tuiModel) fetchStatus() tea.Msg {
	client := resty.New()
	url := fmt.Sprintf("%s/execution/%s", strings.TrimSuffix(m.serverUrl, "/"), m.workflowID)

	resp, err := client.R().
		SetAuthToken(m.authToken).
		SetHeader("Accept", "application/json").
		Get(url)

	if err != nil {
		return errorMsg{err: fmt.Errorf("failed to fetch status: %w", err)}
	}

	if resp.StatusCode() == http.StatusNotImplemented {
		// Check if this is a configuration error
		return errorMsg{err: fmt.Errorf("live updates are not supported by the server")}
	}

	if resp.StatusCode() != http.StatusOK {
		return errorMsg{err: fmt.Errorf("API error: %d - %s for: %s", resp.StatusCode(), string(resp.Body()), url)}
	}

	var apiResponse struct {
		Execution *models.WorkflowExecutionInfo `json:"execution"`
	}

	if err := json.Unmarshal(resp.Body(), &apiResponse); err != nil {
		return errorMsg{err: fmt.Errorf("failed to parse response: %w", err)}
	}

	return execInfo{execution: apiResponse.Execution}
}

// runWorkflowStatusTUI starts the TUI for live workflow status updates
func runWorkflowStatusTUI(workflowID, serverUrl, authToken string) error {
	model := newTuiModel(workflowID, serverUrl, authToken)
	program := tea.NewProgram(model)

	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Cast the final model back to tuiModel to access its fields
	finalTuiModel, ok := finalModel.(tuiModel)
	if !ok {
		return fmt.Errorf("unexpected model type returned from TUI")
	}

	// Check if live updates are not supported
	if !finalTuiModel.liveUpdates {
		return fmt.Errorf("live status updates not supported in this configuration")
	}

	if finalTuiModel.err != nil {
		return finalTuiModel.err
	}

	return nil
}
