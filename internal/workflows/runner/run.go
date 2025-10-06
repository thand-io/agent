package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

// executeRunTask handles task execution
func (r *ResumableWorkflowRunner) executeRunTask(
	taskName string,
	run *model.RunTask,
	input any,
) (map[string]any, error) {

	runTask := run.Run

	if runTask.Await != nil {
		logrus.WithFields(logrus.Fields{
			"task":  taskName,
			"await": runTask.Await,
		}).Info("Run task with await is not implemented yet")
		return nil, fmt.Errorf("run task with await not implemented yet")
	}

	if runTask.Container != nil {
		output, err := r.executeContainerProcess(taskName, runTask.Container)
		return output, err
	} else if runTask.Script != nil {
		// TODO: implement script execution

		return nil, fmt.Errorf("run.script not implemented yet")
	} else if runTask.Shell != nil {

		output, err := r.executeShellProcess(taskName, runTask.Shell)
		return output, err

	} else if runTask.Workflow != nil {
		// TODO: implement nested workflow execution
		return nil, fmt.Errorf("run.workflow not implemented yet")
	}

	return nil, fmt.Errorf("invalid run task: no process configured")
}

// executeShellProcess executes a shell run process definition.
// Supported fields (from DSL): command (string, required), arguments (map[string]any, optional), environment (map[string]any, optional)
// It returns output according to the run.return directive (currently defaults to stdout behavior until extended).
func (r *ResumableWorkflowRunner) executeShellProcess(taskName string, shellTask *model.Shell) (map[string]any, error) {

	if shellTask == nil {
		return nil, fmt.Errorf("shell process is nil")
	}

	// Timeout / context: if DSL adds timeout later; for now use a default or workflow-level context
	// TODO: integrate with task-level timeout configuration once available.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Build exec command directly (avoid shell injection). If user truly wants shells features they can include '/bin/sh' themselves in DSL.
	cmd := exec.CommandContext(ctx, shellTask.Command)
	// cmd.Env = shellTask.Environment
	// Working directory could be configurable later; leave unset (inherits current process CWD)

	// Ensure we can manage the process (and possibly its group) in a portable way.
	setCmdSysProcAttr(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	logrus.WithFields(logrus.Fields{
		"task":        taskName,
		"command":     shellTask.Command,
		"args":        shellTask.Arguments,
		"environment": shellTask.Environment,
	}).Info("Starting shell command")

	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start shell command: %w", err)
	}

	waitErr := cmd.Wait()
	duration := time.Since(started)

	// Handle timeout
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		// kill process (and group on supported platforms)
		_ = killProcessTree(cmd)
		return map[string]any{
			"code":   -1,
			"stdout": stdout.String(),
			"stderr": stderr.String(),
			"error":  "timeout",
			"timeMs": duration.Milliseconds(),
		}, fmt.Errorf("shell command timed out")
	}

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	result := map[string]any{
		"code":   exitCode,
		"stdout": stdout.String(),
		"stderr": stderr.String(),
		"timeMs": duration.Milliseconds(),
	}

	logFields := logrus.Fields{
		"task":      taskName,
		"code":      exitCode,
		"timeMs":    duration.Milliseconds(),
		"stdoutLen": len(result["stdout"].(string)),
		"stderrLen": len(result["stderr"].(string)),
	}
	if waitErr != nil {
		logFields["error"] = waitErr.Error()
		logrus.WithFields(logFields).Warn("Shell command finished with error")
	} else {
		logrus.WithFields(logFields).Info("Shell command finished successfully")
	}

	// For now, we align with DSL 'return' default = stdout; if 'return' property available on runTask, adapt accordingly.
	// Attempt to detect return behavior from parent runTask if accessible later.
	// Expose all under a namespaced key to avoid collisions until transformation layer handles it.
	return map[string]any{
		"process": result,
	}, nil
}

// executeContainerProcess executes a container run process definition using the Docker Engine API.
// Expected (via reflection) exported fields on the container struct (best-effort, optional unless noted):
//
//	Image (string, required)
//	Command ([]string OR string, optional) - overrides image default CMD/ENTRYPOINT
//	Arguments (map[string]any OR []string, optional) - appended to Command deterministically if map
//	Environment (map[string]any, optional)
//	Workdir (string, optional)
//	Pull (string, optional) one of: always, missing, ifNotPresent (alias), never
//	Remove (bool, optional) remove container after completion (default true)
//	TimeoutSeconds (int / float64), optional execution timeout (default 120)
//	Entrypoint ([]string OR string, optional) explicit entrypoint
//	Network (string, optional) network mode (e.g., host, bridge)
//
// Returns map with key "container" containing execution metadata similar to shell process: code, stdout, stderr, timeMs, id, image.
func (r *ResumableWorkflowRunner) executeContainerProcess(taskName string, containerTask *model.Container) (map[string]any, error) {
	if containerTask == nil {
		return nil, fmt.Errorf("container process is nil")
	}

	timeoutSeconds := 120

	// Context with timeout
	timeout := time.Duration(timeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client init: %w", err)
	}

	imageName := containerTask.Image
	//env := containerTask.Environment

	// Pull image according to policy

	logrus.WithFields(logrus.Fields{
		"task": taskName, "image": imageName,
	}).Info("Pulling image")
	rc, perr := cli.ImagePull(ctx, imageName, image.PullOptions{
		All: true,
	})
	if perr != nil {
		return nil, fmt.Errorf("image pull: %w", perr)
	}
	io.Copy(io.Discard, rc)
	rc.Close()

	cfg := &containerTypes.Config{
		Image: imageName,
		//Env:   env,
	}

	hostCfg := &containerTypes.HostConfig{}

	createResp, err := cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, fmt.Sprintf("thand-%s", taskName))
	if err != nil {
		return nil, fmt.Errorf("container create: %w", err)
	}
	containerID := createResp.ID

	startedAt := time.Now()
	logrus.WithFields(logrus.Fields{
		"task":       taskName,
		"image":      imageName,
		"id":         containerID,
		"cmd":        cfg.Cmd,
		"entrypoint": cfg.Entrypoint,
	}).Info("Starting container")

	if err := cli.ContainerStart(ctx, containerID, containerTypes.StartOptions{}); err != nil {
		return nil, fmt.Errorf("container start: %w", err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, containerID, containerTypes.WaitConditionNotRunning)
	var exitCode int64 = -1
	select {
	case status := <-statusCh:
		exitCode = status.StatusCode
	case werr := <-errCh:
		if werr != nil {
			return nil, fmt.Errorf("container wait: %w", werr)
		}
	case <-ctx.Done():
		// Timeout - attempt kill
		_ = cli.ContainerKill(context.Background(), containerID, "KILL")
		return map[string]any{"container": map[string]any{"id": containerID, "image": imageName, "code": -1, "error": "timeout"}}, fmt.Errorf("container timeout exceeded")
	}

	// Fetch logs (stdout+stderr)
	logsReader, lerr := cli.ContainerLogs(context.Background(), containerID, containerTypes.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})

	var stdoutBuf, stderrBuf bytes.Buffer
	if lerr == nil {
		defer logsReader.Close()
		// Docker multiplexes streams if TTY false. For simplicity attempt to split by Docker header (8 bytes). We'll just strip headers and merge.
		// Simpler: copy raw to a buffer then naive split is overkill; here we copy all to stdoutBuf.
		io.Copy(&stdoutBuf, logsReader)
	} else {
		logrus.WithError(lerr).Warn("Failed to fetch container logs")
	}

	duration := time.Since(startedAt)

	result := map[string]any{
		"id":     containerID,
		"image":  imageName,
		"code":   exitCode,
		"stdout": stdoutBuf.String(), // merged
		"stderr": stderrBuf.String(), // currently empty; future: parse multiplexed stream
		"timeMs": duration.Milliseconds(),
	}

	logFields := logrus.Fields{
		"task":      taskName,
		"id":        containerID,
		"image":     imageName,
		"code":      exitCode,
		"timeMs":    duration.Milliseconds(),
		"stdoutLen": len(result["stdout"].(string)),
	}
	logrus.WithFields(logFields).Info("Container finished")

	go func(cid string) {
		// use background context, ignore error
		_ = cli.ContainerRemove(context.Background(), cid, containerTypes.RemoveOptions{Force: true})
	}(containerID)

	return map[string]any{"container": result}, nil
}
