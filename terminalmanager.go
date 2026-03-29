package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"

	"github.com/google/uuid"
)

type terminalExitStatus struct {
	ExitCode *int   `json:"exitCode"`
	Signal   string `json:"signal"`
}

type terminalCreateParams struct {
	SessionID       string            `json:"sessionId"`
	Command         string            `json:"command"`
	Args            []string          `json:"args"`
	Cwd             string            `json:"cwd"`
	Env             map[string]string `json:"env"`
	OutputByteLimit *int              `json:"outputByteLimit"`
}

type terminalIDParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

type terminalProcess struct {
	id          string
	sessionID   string
	root        string
	cmd         *exec.Cmd
	outputLimit int

	mu         sync.Mutex
	output     string
	truncated  bool
	exitStatus *terminalExitStatus
	done       chan struct{}
}

type TerminalManager struct {
	mu        sync.Mutex
	terminals map[string]*terminalProcess
}

func NewTerminalManager() *TerminalManager {
	return &TerminalManager{
		terminals: map[string]*terminalProcess{},
	}
}

func (tm *TerminalManager) create(params terminalCreateParams, workspaceRoot string) (map[string]any, error) {
	if params.Command == "" {
		return nil, errors.New("terminal command is required")
	}

	cwd := workspaceRoot
	if params.Cwd != "" {
		cwd = params.Cwd
	}
	if cwd == "" {
		return nil, errors.New("terminal working directory is required")
	}
	if err := ensurePathWithinRoot(workspaceRoot, cwd); err != nil {
		return nil, err
	}

	cmd := exec.Command(params.Command, params.Args...)
	cmd.Dir = cwd
	cmd.Env = mergeEnv(os.Environ(), params.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	process := &terminalProcess{
		id:          uuid.NewString(),
		sessionID:   params.SessionID,
		root:        workspaceRoot,
		cmd:         cmd,
		outputLimit: outputLimitOrDefault(params.OutputByteLimit),
		done:        make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	tm.mu.Lock()
	tm.terminals[process.id] = process
	tm.mu.Unlock()

	go process.capture(stdout)
	go process.capture(stderr)
	go process.wait()

	return map[string]any{
		"terminalId": process.id,
	}, nil
}

func (tm *TerminalManager) output(params terminalIDParams) (map[string]any, error) {
	process, err := tm.get(params.TerminalID)
	if err != nil {
		return nil, err
	}

	process.mu.Lock()
	defer process.mu.Unlock()

	result := map[string]any{
		"output":    process.output,
		"truncated": process.truncated,
	}
	if process.exitStatus != nil {
		result["exitStatus"] = process.exitStatus
	}
	return result, nil
}

func (tm *TerminalManager) waitForExit(ctx context.Context, params terminalIDParams) (map[string]any, error) {
	process, err := tm.get(params.TerminalID)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-process.done:
		process.mu.Lock()
		defer process.mu.Unlock()
		return map[string]any{
			"exitCode": nullableInt(process.exitStatus),
			"signal":   nullableSignal(process.exitStatus),
		}, nil
	}
}

func (tm *TerminalManager) kill(params terminalIDParams) (map[string]any, error) {
	process, err := tm.get(params.TerminalID)
	if err != nil {
		return nil, err
	}

	if process.cmd.Process == nil {
		return map[string]any{}, nil
	}

	if err := process.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return nil, err
	}

	return map[string]any{}, nil
}

func (tm *TerminalManager) release(params terminalIDParams) (map[string]any, error) {
	process, err := tm.get(params.TerminalID)
	if err != nil {
		return nil, err
	}

	if process.cmd.Process != nil {
		_ = process.cmd.Process.Kill()
	}

	tm.mu.Lock()
	delete(tm.terminals, params.TerminalID)
	tm.mu.Unlock()

	return map[string]any{}, nil
}

func (tm *TerminalManager) shutdown() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for id, process := range tm.terminals {
		if process.cmd.Process != nil {
			_ = process.cmd.Process.Kill()
		}
		delete(tm.terminals, id)
	}
}

func (tm *TerminalManager) terminalSnapshot(id string) (ToolCallContent, bool) {
	process, err := tm.get(id)
	if err != nil {
		return ToolCallContent{}, false
	}

	process.mu.Lock()
	defer process.mu.Unlock()

	content := ToolCallContent{
		Type:        "terminal",
		TerminalID:  id,
		Output:      process.output,
		IsTruncated: process.truncated,
	}
	if process.exitStatus != nil {
		if process.exitStatus.ExitCode != nil {
			content.ExitCode = *process.exitStatus.ExitCode
			content.ExitCodeSet = true
		}
	}
	return content, true
}

func (tm *TerminalManager) get(id string) (*terminalProcess, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	process, ok := tm.terminals[id]
	if !ok {
		return nil, errors.New("terminal not found")
	}
	return process, nil
}

func (tp *terminalProcess) capture(stream io.ReadCloser) {
	defer stream.Close()

	reader := bufio.NewReader(stream)
	for {
		chunk, err := reader.ReadString('\n')
		if chunk != "" {
			tp.appendOutput(chunk)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			tp.appendOutput(err.Error() + "\n")
			return
		}
	}
}

func (tp *terminalProcess) appendOutput(chunk string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	next := tp.output + chunk
	if tp.outputLimit > 0 && len(next) > tp.outputLimit {
		next = truncateToLastBytes(next, tp.outputLimit)
		tp.truncated = true
	}
	tp.output = next
}

func (tp *terminalProcess) wait() {
	defer close(tp.done)

	err := tp.cmd.Wait()
	status := &terminalExitStatus{}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if exitErr.ExitCode() >= 0 {
					code := exitErr.ExitCode()
					status.ExitCode = &code
				}
				if waitStatus.Signaled() {
					status.Signal = waitStatus.Signal().String()
				}
			}
		}
	} else {
		code := 0
		status.ExitCode = &code
	}

	tp.mu.Lock()
	tp.exitStatus = status
	tp.mu.Unlock()
}

func outputLimitOrDefault(limit *int) int {
	if limit == nil {
		return 128 * 1024
	}
	return *limit
}

func truncateToLastBytes(input string, limit int) string {
	if limit <= 0 || len(input) <= limit {
		return input
	}
	bytes := []byte(input)
	if len(bytes) <= limit {
		return input
	}
	bytes = bytes[len(bytes)-limit:]
	for !utf8.Valid(bytes) && len(bytes) > 0 {
		bytes = bytes[1:]
	}
	return string(bytes)
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string{}, base...)
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key := entry
		if index := strings.IndexRune(entry, '='); index >= 0 {
			key = entry[:index]
		}
		if value, ok := overrides[key]; ok {
			result = append(result, key+"="+value)
			seen[key] = struct{}{}
			continue
		}
		result = append(result, entry)
		seen[key] = struct{}{}
	}
	for key, value := range overrides {
		if _, ok := seen[key]; ok {
			continue
		}
		result = append(result, key+"="+value)
	}
	return result
}

func nullableInt(status *terminalExitStatus) *int {
	if status == nil {
		return nil
	}
	return status.ExitCode
}

func nullableSignal(status *terminalExitStatus) *string {
	if status == nil || status.Signal == "" {
		return nil
	}
	signal := status.Signal
	return &signal
}
