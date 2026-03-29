package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const syntheticModelConfigID = "__acp_model"

type acpClient struct {
	agentName         string
	agentTitle        string
	agentVersionValue string
	protocolVersion   int
	config            AgentConfig

	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
	stdin  io.WriteCloser

	conn *acp.ClientSideConnection

	workspaceRoot        string
	promptCaps           PromptCapabilities
	loadSessionSupported bool
	authMethods          []AuthMethod

	mu              sync.Mutex
	currentModeID   string
	modes           []SessionMode
	configOptions   []SessionConfigOption
	currentModelID  string
	availableModels []SessionConfigValue

	terminals   *TerminalManager
	permissions *PermissionService
	onEvent     func(SessionEvent)
}

type acpClientAdapter struct {
	client *acpClient
}

type ACPService struct {
	ctx         context.Context
	settings    *SettingsService
	permissions *PermissionService
	terminals   *TerminalManager

	mu     sync.Mutex
	active *acpClient
}

func NewACPService(settings *SettingsService, permissions *PermissionService) *ACPService {
	return &ACPService{
		settings:    settings,
		permissions: permissions,
		terminals:   NewTerminalManager(),
	}
}

func (s *ACPService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.ctx = ctx
	return nil
}

func (s *ACPService) ServiceShutdown() error {
	s.mu.Lock()
	active := s.active
	s.active = nil
	s.mu.Unlock()

	if active != nil {
		active.close()
	}
	s.terminals.shutdown()
	return nil
}

func (s *ACPService) TestAgent(agentName string) (AgentTestResult, error) {
	settings, err := s.settings.GetSettings()
	if err != nil {
		return AgentTestResult{}, err
	}

	config, ok := settings.Agents[agentName]
	if !ok {
		return AgentTestResult{}, fmt.Errorf("agent %q not found", agentName)
	}

	client, err := s.startClient(agentName, config, "", nil)
	if err != nil {
		return AgentTestResult{}, err
	}
	defer client.close()

	return AgentTestResult{
		AgentName:          agentName,
		AgentTitle:         client.agentTitle,
		AgentVersion:       client.agentVersion(),
		ProtocolVersion:    client.protocolVersion,
		PromptCapabilities: client.promptCaps,
		LoadSession:        client.loadSessionSupported,
		AuthMethods:        append([]AuthMethod{}, client.authMethods...),
	}, nil
}

func (s *ACPService) AuthenticateAgent(agentName, methodID string) (AgentTestResult, error) {
	settings, err := s.settings.GetSettings()
	if err != nil {
		return AgentTestResult{}, err
	}

	config, ok := settings.Agents[agentName]
	if !ok {
		return AgentTestResult{}, fmt.Errorf("agent %q not found", agentName)
	}

	client, err := s.startClient(agentName, config, "", nil)
	if err != nil {
		return AgentTestResult{}, err
	}
	defer client.close()

	if err := client.authenticate(context.Background(), methodID); err != nil {
		return AgentTestResult{}, err
	}

	return AgentTestResult{
		AgentName:          agentName,
		AgentTitle:         client.agentTitle,
		AgentVersion:       client.agentVersion(),
		ProtocolVersion:    client.protocolVersion,
		PromptCapabilities: client.promptCaps,
		LoadSession:        client.loadSessionSupported,
		AuthMethods:        append([]AuthMethod{}, client.authMethods...),
	}, nil
}

func (s *ACPService) DiscoverAgentModes(agentName, cwd string, force bool) (AgentDescriptor, error) {
	settings, err := s.settings.GetSettings()
	if err != nil {
		return AgentDescriptor{}, err
	}

	config, ok := settings.Agents[agentName]
	if !ok {
		return AgentDescriptor{}, fmt.Errorf("agent %q not found", agentName)
	}

	catalog, err := s.settings.readAgentCatalog()
	if err != nil {
		return AgentDescriptor{}, err
	}
	if !force {
		if entry, ok, err := s.settings.cachedAgentCatalog(agentName, config, catalog); err != nil {
			return AgentDescriptor{}, err
		} else if ok && (len(entry.Modes) > 0 || entry.ModelOption != nil) {
			return AgentDescriptor{
				Name:              agentName,
				Command:           config.Command,
				Args:              append([]string{}, config.Args...),
				Env:               copyStringMap(config.Env),
				CurrentModeID:     entry.CurrentModeID,
				ModelOption:       copySessionConfigOption(entry.ModelOption),
				Modes:             copySessionModes(entry.Modes),
				ModesDiscoveredAt: entry.DiscoveredAt,
			}, nil
		}
	}

	probeRoot := strings.TrimSpace(cwd)
	if probeRoot == "" {
		probeRoot, err = os.UserHomeDir()
		if err != nil {
			return AgentDescriptor{}, err
		}
	}

	probeRoot, err = normalizeWorkspacePath(probeRoot)
	if err != nil {
		return AgentDescriptor{}, err
	}

	client, err := s.startClient(agentName, config, probeRoot, nil)
	if err != nil {
		return AgentDescriptor{}, err
	}
	defer client.close()

	if _, err := client.createSession(context.Background(), probeRoot); err != nil {
		return AgentDescriptor{}, err
	}

	if err := s.settings.persistAgentCatalog(agentName, config, client.currentMode(), client.sessionModes(), client.currentModelOption()); err != nil {
		return AgentDescriptor{}, err
	}

	return AgentDescriptor{
		Name:              agentName,
		Command:           config.Command,
		Args:              append([]string{}, config.Args...),
		Env:               copyStringMap(config.Env),
		CurrentModeID:     client.currentMode(),
		ModelOption:       client.currentModelOption(),
		Modes:             client.sessionModes(),
		ModesDiscoveredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *ACPService) openAgent(agentName string, config AgentConfig, workspaceRoot string, onEvent func(SessionEvent)) (*acpClient, error) {
	s.mu.Lock()
	active := s.active
	s.active = nil
	s.mu.Unlock()

	if active != nil {
		active.close()
	}

	client, err := s.startClient(agentName, config, workspaceRoot, onEvent)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.active = client
	s.mu.Unlock()
	return client, nil
}

func (s *ACPService) activeClient() *acpClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *ACPService) closeActive() {
	s.mu.Lock()
	active := s.active
	s.active = nil
	s.mu.Unlock()
	if active != nil {
		active.close()
	}
}

func (s *ACPService) startClient(agentName string, config AgentConfig, workspaceRoot string, onEvent func(SessionEvent)) (*acpClient, error) {
	if strings.TrimSpace(config.Command) == "" {
		return nil, errors.New("agent command is required")
	}

	ctx, cancel := context.WithCancel(s.ctx)
	cmd := exec.CommandContext(ctx, config.Command, config.Args...)
	cmd.Env = mergeEnv(os.Environ(), config.Env)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	client := newACPClient(agentName, config, workspaceRoot, s.permissions, s.terminals, onEvent, ctx, cancel, cmd, stdin, stdout)
	if err := client.initialize(ctx); err != nil {
		client.close()
		if details := strings.TrimSpace(stderr.String()); details != "" {
			return nil, fmt.Errorf("%w: %s", err, details)
		}
		return nil, err
	}

	return client, nil
}

func newACPClient(agentName string, config AgentConfig, workspaceRoot string, permissions *PermissionService, terminals *TerminalManager, onEvent func(SessionEvent), ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, stdin io.WriteCloser, stdout io.Reader) *acpClient {
	client := &acpClient{
		agentName:     agentName,
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		cmd:           cmd,
		stdin:         stdin,
		workspaceRoot: workspaceRoot,
		permissions:   permissions,
		terminals:     terminals,
		onEvent:       onEvent,
	}

	adapter := &acpClientAdapter{client: client}
	client.conn = acp.NewClientSideConnection(adapter, stdin, stdout)
	return client
}

func (c *acpClient) initialize(ctx context.Context) error {
	response, err := c.conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersion(acp.ProtocolVersionNumber),
		ClientCapabilities: acp.ClientCapabilities{
			Fs: acp.FileSystemCapability{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
		ClientInfo: &acp.Implementation{
			Name:    "chisel",
			Title:   acp.Ptr("Chisel"),
			Version: "0.1.0",
		},
	})
	if err != nil {
		return err
	}

	if response.ProtocolVersion != acp.ProtocolVersion(acp.ProtocolVersionNumber) {
		return fmt.Errorf("agent negotiated unsupported ACP protocol version %d", response.ProtocolVersion)
	}

	c.protocolVersion = int(response.ProtocolVersion)
	c.promptCaps = toPromptCapabilities(response.AgentCapabilities.PromptCapabilities)
	c.loadSessionSupported = response.AgentCapabilities.LoadSession
	c.authMethods = toAuthMethods(response.AuthMethods)

	if response.AgentInfo != nil {
		c.agentTitle = firstNonEmpty(stringPointerValue(response.AgentInfo.Title), response.AgentInfo.Name, c.agentName)
		c.agentVersionValue = response.AgentInfo.Version
	}
	if c.agentTitle == "" {
		c.agentTitle = c.agentName
	}

	return nil
}

func (c *acpClient) authenticate(ctx context.Context, methodID string) error {
	if strings.TrimSpace(methodID) == "" {
		return errors.New("authentication method is required")
	}
	_, err := c.conn.Authenticate(ctx, acp.AuthenticateRequest{
		MethodId: methodID,
	})
	return wrapAuthHint(err, c.authMethods)
}

func (c *acpClient) agentVersion() string {
	return c.agentVersionValue
}

func (c *acpClient) close() {
	c.cancel()
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}

func (c *acpClient) createSession(ctx context.Context, cwd string) (acp.NewSessionResponse, error) {
	result, err := c.conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return acp.NewSessionResponse{}, wrapAuthHint(err, c.authMethods)
	}

	c.workspaceRoot = cwd
	c.replaceSessionState(result.ConfigOptions, result.Modes, result.Models)
	return result, nil
}

func (c *acpClient) loadSession(ctx context.Context, sessionID, cwd string) (acp.LoadSessionResponse, error) {
	if !c.loadSessionSupported {
		return acp.LoadSessionResponse{}, errors.New("agent does not support loading sessions")
	}

	result, err := c.conn.LoadSession(ctx, acp.LoadSessionRequest{
		SessionId:  acp.SessionId(sessionID),
		Cwd:        cwd,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return acp.LoadSessionResponse{}, wrapAuthHint(err, c.authMethods)
	}

	c.workspaceRoot = cwd
	c.replaceSessionState(result.ConfigOptions, result.Modes, result.Models)
	return result, nil
}

func (c *acpClient) promptAsync(sessionID, prompt string) {
	go func() {
		response, err := c.conn.Prompt(c.ctx, acp.PromptRequest{
			SessionId: acp.SessionId(sessionID),
			Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
		})
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			if isACPRequestCancelled(err) {
				c.emit(SessionEvent{
					SessionID:  sessionID,
					Kind:       "turn_end",
					StopReason: "cancelled",
				})
				return
			}
			c.emit(SessionEvent{
				SessionID: sessionID,
				Kind:      "error",
				Error:     wrapAuthHint(err, c.authMethods).Error(),
			})
			return
		}

		c.emit(SessionEvent{
			SessionID:  sessionID,
			Kind:       "turn_end",
			StopReason: string(response.StopReason),
		})
	}()
}

func (c *acpClient) cancelPrompt(sessionID string) error {
	c.permissions.cancelSession(sessionID)
	return c.conn.Cancel(context.Background(), acp.CancelNotification{
		SessionId: acp.SessionId(sessionID),
	})
}

func (c *acpClient) setConfigOption(ctx context.Context, sessionID, optionID, value string) error {
	if optionID == syntheticModelConfigID {
		return c.setModel(ctx, sessionID, value)
	}

	response, err := c.conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		SessionId: acp.SessionId(sessionID),
		ConfigId:  acp.SessionConfigId(optionID),
		Value:     acp.SessionConfigValueId(value),
	})
	if err != nil {
		return wrapAuthHint(err, c.authMethods)
	}

	c.applyConfigOptions(response.ConfigOptions)
	return nil
}

func (c *acpClient) setModel(ctx context.Context, sessionID, modelID string) error {
	if option := c.modelConfigOption(); option != nil {
		return c.setConfigOption(ctx, sessionID, option.ID, modelID)
	}

	c.mu.Lock()
	availableModels := copySessionConfigValues(c.availableModels)
	c.mu.Unlock()

	if len(availableModels) == 0 {
		return errors.New("agent does not expose selectable models for this session")
	}

	_, err := c.conn.UnstableSetSessionModel(ctx, acp.UnstableSetSessionModelRequest{
		SessionId: acp.SessionId(sessionID),
		ModelId:   acp.UnstableModelId(modelID),
	})
	if err != nil {
		return wrapAuthHint(err, c.authMethods)
	}

	c.applyCurrentModel(modelID)
	return nil
}

func (c *acpClient) setMode(ctx context.Context, sessionID, modeID string) error {
	previous := c.currentMode()
	_, err := c.conn.SetSessionMode(ctx, acp.SetSessionModeRequest{
		SessionId: acp.SessionId(sessionID),
		ModeId:    acp.SessionModeId(modeID),
	})
	if err != nil {
		return wrapAuthHint(err, c.authMethods)
	}

	c.mu.Lock()
	if c.currentModeID == "" || c.currentModeID == previous {
		c.currentModeID = modeID
	}
	c.mu.Unlock()
	return nil
}

func (c *acpClient) replaceSessionState(options []acp.SessionConfigOption, modes *acp.SessionModeState, models *acp.SessionModelState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.configOptions = normalizeConfigOptions(options)
	c.currentModelID, c.availableModels = normalizeSessionModels(models)
	if modes == nil {
		c.currentModeID = ""
		c.modes = nil
	} else {
		c.currentModeID = string(modes.CurrentModeId)
		c.modes = normalizeModes(modes.AvailableModes)
	}
}

func (c *acpClient) applyConfigOptions(options []acp.SessionConfigOption) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configOptions = normalizeConfigOptions(options)
}

func (c *acpClient) applyCurrentMode(modeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentModeID = modeID
}

func (c *acpClient) applyCurrentModel(modelID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentModelID = modelID
}

func (c *acpClient) currentMode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentModeID
}

func (c *acpClient) sessionModes() []SessionMode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]SessionMode{}, c.modes...)
}

func (c *acpClient) sessionConfigOptions() []SessionConfigOption {
	c.mu.Lock()
	defer c.mu.Unlock()
	options := append([]SessionConfigOption{}, c.configOptions...)
	if modelOption := modelOptionFromState(c.configOptions, c.availableModels, c.currentModelID); modelOption != nil {
		options = append(options, *modelOption)
	}
	return options
}

func (c *acpClient) currentModelOption() *SessionConfigOption {
	c.mu.Lock()
	defer c.mu.Unlock()
	return modelOptionFromState(c.configOptions, c.availableModels, c.currentModelID)
}

func (c *acpClient) modelConfigOption() *SessionConfigOption {
	c.mu.Lock()
	defer c.mu.Unlock()
	return findModelConfigOption(c.configOptions)
}

func (c *acpClient) emit(event SessionEvent) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if c.onEvent != nil {
		c.onEvent(event)
	}
	emitSessionEvent(event)
}

func (a *acpClientAdapter) ReadTextFile(ctx context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	if err := ensurePathWithinRoot(a.client.workspaceRoot, params.Path); err != nil {
		return acp.ReadTextFileResponse{}, err
	}

	content, err := os.ReadFile(params.Path)
	if err != nil {
		return acp.ReadTextFileResponse{}, err
	}

	return acp.ReadTextFileResponse{
		Content: sliceTextContent(string(content), params.Line, params.Limit),
	}, nil
}

func (a *acpClientAdapter) WriteTextFile(ctx context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	if err := ensurePathWithinRoot(a.client.workspaceRoot, params.Path); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	if err := os.MkdirAll(filepath.Dir(params.Path), 0o755); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0o644); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	return acp.WriteTextFileResponse{}, nil
}

func (a *acpClientAdapter) RequestPermission(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	options := make([]PermissionOption, 0, len(params.Options))
	for _, option := range params.Options {
		options = append(options, PermissionOption{
			ID:          string(option.OptionId),
			Name:        option.Name,
			Kind:        string(option.Kind),
			Description: "",
		})
	}

	title := strings.TrimSpace(stringPointerValue(params.ToolCall.Title))
	if title == "" {
		title = firstNonEmpty(stringPointerValue(toolCallTitleFromUpdate(params.ToolCall)), "Permission request")
	}
	description := buildPermissionDescription(params.ToolCall)

	decision, err := a.client.permissions.requestPermission(
		ctx,
		string(params.SessionId),
		string(params.ToolCall.ToolCallId),
		title,
		description,
		options,
	)
	if err != nil && !errors.Is(err, context.Canceled) {
		return acp.RequestPermissionResponse{}, err
	}
	if decision.cancelled || errors.Is(err, context.Canceled) {
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{
					Outcome: "cancelled",
				},
			},
		}, nil
	}

	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{
				Outcome:  "selected",
				OptionId: acp.PermissionOptionId(decision.optionID),
			},
		},
	}, nil
}

func (a *acpClientAdapter) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	event, ok := a.client.normalizeSessionNotification(params)
	if !ok {
		raw, _ := json.Marshal(params.Update)
		a.client.emit(SessionEvent{
			SessionID: string(params.SessionId),
			Kind:      "raw_update",
			Raw:       string(raw),
		})
		return nil
	}

	a.client.emit(event)
	return nil
}

func (a *acpClientAdapter) CreateTerminal(ctx context.Context, params acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	request := terminalCreateParams{
		SessionID:       string(params.SessionId),
		Command:         params.Command,
		Args:            append([]string{}, params.Args...),
		Cwd:             stringPointerValue(params.Cwd),
		Env:             envVariablesToMap(params.Env),
		OutputByteLimit: params.OutputByteLimit,
	}

	result, err := a.client.terminals.create(request, a.client.workspaceRoot)
	if err != nil {
		return acp.CreateTerminalResponse{}, err
	}

	terminalID, _ := result["terminalId"].(string)
	return acp.CreateTerminalResponse{TerminalId: terminalID}, nil
}

func (a *acpClientAdapter) KillTerminalCommand(ctx context.Context, params acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	_, err := a.client.terminals.kill(terminalIDParams{
		SessionID:  string(params.SessionId),
		TerminalID: params.TerminalId,
	})
	if err != nil {
		return acp.KillTerminalCommandResponse{}, err
	}
	return acp.KillTerminalCommandResponse{}, nil
}

func (a *acpClientAdapter) TerminalOutput(ctx context.Context, params acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	result, err := a.client.terminals.output(terminalIDParams{
		SessionID:  string(params.SessionId),
		TerminalID: params.TerminalId,
	})
	if err != nil {
		return acp.TerminalOutputResponse{}, err
	}

	response := acp.TerminalOutputResponse{
		Output:    stringMapValue(result, "output"),
		Truncated: boolMapValue(result, "truncated"),
	}
	if exitStatus, ok := result["exitStatus"].(*terminalExitStatus); ok && exitStatus != nil {
		response.ExitStatus = &acp.TerminalExitStatus{
			ExitCode: exitStatus.ExitCode,
		}
		if exitStatus.Signal != "" {
			response.ExitStatus.Signal = acp.Ptr(exitStatus.Signal)
		}
	}
	return response, nil
}

func (a *acpClientAdapter) ReleaseTerminal(ctx context.Context, params acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	_, err := a.client.terminals.release(terminalIDParams{
		SessionID:  string(params.SessionId),
		TerminalID: params.TerminalId,
	})
	if err != nil {
		return acp.ReleaseTerminalResponse{}, err
	}
	return acp.ReleaseTerminalResponse{}, nil
}

func (a *acpClientAdapter) WaitForTerminalExit(ctx context.Context, params acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	result, err := a.client.terminals.waitForExit(a.client.ctx, terminalIDParams{
		SessionID:  string(params.SessionId),
		TerminalID: params.TerminalId,
	})
	if err != nil {
		return acp.WaitForTerminalExitResponse{}, err
	}

	response := acp.WaitForTerminalExitResponse{}
	if exitCode, ok := result["exitCode"].(*int); ok {
		response.ExitCode = exitCode
	}
	if signal, ok := result["signal"].(*string); ok {
		response.Signal = signal
	}
	return response, nil
}

func (a *acpClientAdapter) HandleExtensionMethod(ctx context.Context, method string, params json.RawMessage) (any, error) {
	a.client.emit(SessionEvent{
		Kind:   "ext_notification",
		Method: method,
		Raw:    rawJSONToString(params),
	})
	return nil, acp.NewMethodNotFound(method)
}

func (c *acpClient) normalizeSessionNotification(notification acp.SessionNotification) (SessionEvent, bool) {
	sessionID := string(notification.SessionId)

	switch {
	case notification.Update.UserMessageChunk != nil:
		return SessionEvent{
			SessionID:   sessionID,
			Kind:        "message_chunk",
			MessageRole: "user",
			Text:        extractTextFromBlock(notification.Update.UserMessageChunk.Content),
		}, true
	case notification.Update.AgentMessageChunk != nil:
		return SessionEvent{
			SessionID:   sessionID,
			Kind:        "message_chunk",
			MessageRole: "assistant",
			Text:        extractTextFromBlock(notification.Update.AgentMessageChunk.Content),
		}, true
	case notification.Update.AgentThoughtChunk != nil:
		return SessionEvent{
			SessionID: sessionID,
			Kind:      "thought_chunk",
			Thought:   extractTextFromBlock(notification.Update.AgentThoughtChunk.Content),
		}, true
	case notification.Update.ToolCall != nil:
		record := c.normalizeToolCall(notification.Update.ToolCall)
		return SessionEvent{
			SessionID: sessionID,
			Kind:      "tool_call",
			ToolCall:  &record,
		}, true
	case notification.Update.ToolCallUpdate != nil:
		record := c.normalizeToolCallUpdate(notification.Update.ToolCallUpdate)
		return SessionEvent{
			SessionID: sessionID,
			Kind:      "tool_call_update",
			ToolCall:  &record,
		}, true
	case notification.Update.Plan != nil:
		return SessionEvent{
			SessionID: sessionID,
			Kind:      "plan_update",
			Plan:      normalizePlanEntries(notification.Update.Plan.Entries),
		}, true
	case notification.Update.SessionInfoUpdate != nil:
		return SessionEvent{
			SessionID: sessionID,
			Kind:      "session_info_update",
			Title:     stringPointerValue(notification.Update.SessionInfoUpdate.Title),
		}, true
	case notification.Update.ConfigOptionUpdate != nil:
		c.applyConfigOptions(notification.Update.ConfigOptionUpdate.ConfigOptions)
		return SessionEvent{
			SessionID:     sessionID,
			Kind:          "config_options_update",
			ConfigOptions: c.sessionConfigOptions(),
		}, true
	case notification.Update.CurrentModeUpdate != nil:
		modeID := string(notification.Update.CurrentModeUpdate.CurrentModeId)
		c.applyCurrentMode(modeID)
		return SessionEvent{
			SessionID:     sessionID,
			Kind:          "mode_update",
			CurrentModeID: modeID,
		}, true
	default:
		return SessionEvent{}, false
	}
}

func (c *acpClient) normalizeToolCall(update *acp.SessionUpdateToolCall) ToolCallRecord {
	record := ToolCallRecord{
		ToolCallID: string(update.ToolCallId),
		Title:      update.Title,
		Kind:       string(update.Kind),
		Status:     string(update.Status),
		Content:    normalizeToolCallContent(update.Content, c.terminals),
	}

	if len(record.Content) == 0 {
		record.Content = locationsToToolCallContent(update.Locations)
	}
	return record
}

func (c *acpClient) normalizeToolCallUpdate(update *acp.SessionToolCallUpdate) ToolCallRecord {
	record := ToolCallRecord{
		ToolCallID: string(update.ToolCallId),
		Content:    normalizeToolCallContent(update.Content, c.terminals),
	}
	if update.Title != nil {
		record.Title = *update.Title
	}
	if update.Kind != nil {
		record.Kind = string(*update.Kind)
	}
	if update.Status != nil {
		record.Status = string(*update.Status)
	}
	if len(record.Content) == 0 {
		record.Content = locationsToToolCallContent(update.Locations)
	}
	return record
}

func normalizeToolCallContent(items []acp.ToolCallContent, terminals *TerminalManager) []ToolCallContent {
	result := make([]ToolCallContent, 0, len(items))
	for _, item := range items {
		switch {
		case item.Content != nil:
			content := ToolCallContent{
				Type: textOrDefault(contentBlockType(item.Content.Content), "content"),
				Text: extractTextFromBlock(item.Content.Content),
			}
			switch {
			case item.Content.Content.ResourceLink != nil:
				content.URI = item.Content.Content.ResourceLink.Uri
				content.MimeType = stringPointerValue(item.Content.Content.ResourceLink.MimeType)
				content.Description = firstNonEmpty(stringPointerValue(item.Content.Content.ResourceLink.Description), item.Content.Content.ResourceLink.Name)
			case item.Content.Content.Resource != nil:
				if item.Content.Content.Resource.Resource.TextResourceContents != nil {
					content.Path = item.Content.Content.Resource.Resource.TextResourceContents.Uri
					content.MimeType = stringPointerValue(item.Content.Content.Resource.Resource.TextResourceContents.MimeType)
				} else if item.Content.Content.Resource.Resource.BlobResourceContents != nil {
					content.Path = item.Content.Content.Resource.Resource.BlobResourceContents.Uri
					content.MimeType = stringPointerValue(item.Content.Content.Resource.Resource.BlobResourceContents.MimeType)
				}
			case item.Content.Content.Image != nil:
				content.MimeType = item.Content.Content.Image.MimeType
				content.URI = stringPointerValue(item.Content.Content.Image.Uri)
			case item.Content.Content.Audio != nil:
				content.MimeType = item.Content.Content.Audio.MimeType
			}
			result = append(result, content)
		case item.Diff != nil:
			result = append(result, ToolCallContent{
				Type:        "diff",
				Text:        item.Diff.NewText,
				Path:        item.Diff.Path,
				Description: "Updated file",
			})
		case item.Terminal != nil:
			content := ToolCallContent{
				Type:       "terminal",
				TerminalID: item.Terminal.TerminalId,
			}
			if snapshot, ok := terminals.terminalSnapshot(item.Terminal.TerminalId); ok {
				content.Output = snapshot.Output
				content.ExitCode = snapshot.ExitCode
				content.ExitCodeSet = snapshot.ExitCodeSet
				content.IsTruncated = snapshot.IsTruncated
			}
			result = append(result, content)
		}
	}
	return result
}

func locationsToToolCallContent(locations []acp.ToolCallLocation) []ToolCallContent {
	if len(locations) == 0 {
		return nil
	}

	result := make([]ToolCallContent, 0, len(locations))
	for _, location := range locations {
		if location.Path == "" {
			continue
		}
		result = append(result, ToolCallContent{
			Type: "location",
			Path: location.Path,
		})
	}
	return result
}

func normalizePlanEntries(entries []acp.PlanEntry) []PlanEntry {
	if len(entries) == 0 {
		return nil
	}

	result := make([]PlanEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, PlanEntry{
			Content:  entry.Content,
			Status:   string(entry.Status),
			Priority: string(entry.Priority),
		})
	}
	return result
}

func normalizeModes(modes []acp.SessionMode) []SessionMode {
	if len(modes) == 0 {
		return nil
	}

	result := make([]SessionMode, 0, len(modes))
	for _, mode := range modes {
		result = append(result, SessionMode{
			ID:          string(mode.Id),
			Name:        mode.Name,
			Description: stringPointerValue(mode.Description),
		})
	}
	return result
}

func normalizeConfigOptions(options []acp.SessionConfigOption) []SessionConfigOption {
	if len(options) == 0 {
		return nil
	}

	result := make([]SessionConfigOption, 0, len(options))
	for _, option := range options {
		if option.Select == nil {
			continue
		}

		values := make([]SessionConfigValue, 0)
		switch {
		case option.Select.Options.Ungrouped != nil:
			for _, value := range *option.Select.Options.Ungrouped {
				values = append(values, SessionConfigValue{
					Value:       string(value.Value),
					Name:        value.Name,
					Description: stringPointerValue(value.Description),
				})
			}
		case option.Select.Options.Grouped != nil:
			for _, group := range *option.Select.Options.Grouped {
				for _, value := range group.Options {
					values = append(values, SessionConfigValue{
						Value:       string(value.Value),
						Name:        value.Name,
						Description: stringPointerValue(value.Description),
						GroupID:     string(group.Group),
						GroupName:   group.Name,
					})
				}
			}
		}

		result = append(result, SessionConfigOption{
			ID:           string(option.Select.Id),
			Name:         option.Select.Name,
			Description:  stringPointerValue(option.Select.Description),
			Category:     normalizeSessionConfigCategory(option.Select.Category),
			Type:         option.Select.Type,
			CurrentValue: string(option.Select.CurrentValue),
			Values:       values,
		})
	}
	return result
}

func normalizeSessionModels(models *acp.SessionModelState) (string, []SessionConfigValue) {
	if models == nil || len(models.AvailableModels) == 0 {
		return "", nil
	}

	result := make([]SessionConfigValue, 0, len(models.AvailableModels))
	for _, model := range models.AvailableModels {
		result = append(result, SessionConfigValue{
			Value:       string(model.ModelId),
			Name:        model.Name,
			Description: stringPointerValue(model.Description),
		})
	}

	return string(models.CurrentModelId), result
}

func modelOptionFromState(options []SessionConfigOption, availableModels []SessionConfigValue, currentModelID string) *SessionConfigOption {
	if option := findModelConfigOption(options); option != nil {
		return option
	}
	if len(availableModels) == 0 {
		return nil
	}

	return &SessionConfigOption{
		ID:           syntheticModelConfigID,
		Name:         "Model",
		Description:  "Choose the model for this session.",
		Category:     "model",
		Type:         "select",
		CurrentValue: currentModelID,
		Values:       copySessionConfigValues(availableModels),
	}
}

func findModelConfigOption(options []SessionConfigOption) *SessionConfigOption {
	for _, option := range options {
		if !isModelConfigOption(option) {
			continue
		}

		copied := option
		copied.Values = copySessionConfigValues(option.Values)
		return &copied
	}
	return nil
}

func isModelConfigOption(option SessionConfigOption) bool {
	if strings.EqualFold(option.Category, "model") {
		return true
	}

	label := strings.ToLower(strings.TrimSpace(option.ID + " " + option.Name))
	return strings.Contains(label, "model")
}

func normalizeSessionConfigCategory(category *acp.SessionConfigOptionCategory) string {
	if category == nil || category.Other == nil {
		return ""
	}
	return string(*category.Other)
}

func extractTextFromBlock(block acp.ContentBlock) string {
	switch {
	case block.Text != nil:
		return block.Text.Text
	case block.ResourceLink != nil:
		return firstNonEmpty(stringPointerValue(block.ResourceLink.Title), block.ResourceLink.Name, block.ResourceLink.Uri)
	case block.Resource != nil:
		if block.Resource.Resource.TextResourceContents != nil {
			return block.Resource.Resource.TextResourceContents.Text
		}
		if block.Resource.Resource.BlobResourceContents != nil {
			return block.Resource.Resource.BlobResourceContents.Uri
		}
	}
	return ""
}

func contentBlockType(block acp.ContentBlock) string {
	switch {
	case block.Text != nil:
		return block.Text.Type
	case block.Image != nil:
		return block.Image.Type
	case block.Audio != nil:
		return block.Audio.Type
	case block.ResourceLink != nil:
		return block.ResourceLink.Type
	case block.Resource != nil:
		return block.Resource.Type
	default:
		return ""
	}
}

func buildPermissionDescription(update acp.ToolCallUpdate) string {
	parts := make([]string, 0)
	for _, location := range update.Locations {
		if location.Path != "" {
			parts = append(parts, location.Path)
		}
	}
	return firstNonEmpty(cleanTextParts(parts), stringPointerValue(update.Title), "Choose how Chisel should respond.")
}

func toolCallTitleFromUpdate(update acp.ToolCallUpdate) *string {
	if update.Title != nil {
		return update.Title
	}
	if update.Kind != nil {
		value := strings.ReplaceAll(string(*update.Kind), "_", " ")
		value = strings.TrimSpace(strings.Title(value))
		return &value
	}
	return nil
}

func envVariablesToMap(input []acp.EnvVariable) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	result := make(map[string]string, len(input))
	for _, variable := range input {
		if strings.TrimSpace(variable.Name) == "" {
			continue
		}
		result[variable.Name] = variable.Value
	}
	return result
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func textOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func toPromptCapabilities(input acp.PromptCapabilities) PromptCapabilities {
	return PromptCapabilities{
		Image:           input.Image,
		Audio:           input.Audio,
		EmbeddedContext: input.EmbeddedContext,
	}
}

func toAuthMethods(methods []acp.AuthMethod) []AuthMethod {
	if len(methods) == 0 {
		return nil
	}

	result := make([]AuthMethod, 0, len(methods))
	for _, method := range methods {
		result = append(result, AuthMethod{
			ID:          method.Id,
			Name:        method.Name,
			Description: stringPointerValue(method.Description),
		})
	}
	return result
}

func sliceTextContent(content string, line, limit *int) string {
	if line == nil && limit == nil {
		return content
	}

	lines := strings.Split(content, "\n")
	start := 0
	if line != nil && *line > 0 {
		start = *line - 1
		if start > len(lines) {
			start = len(lines)
		}
	}

	end := len(lines)
	if limit != nil && *limit > 0 && start+*limit < end {
		end = start + *limit
	}
	if start > end {
		start = end
	}
	return strings.Join(lines[start:end], "\n")
}

func boolMapValue(input map[string]any, key string) bool {
	value, _ := input[key].(bool)
	return value
}

func stringMapValue(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

func isACPRequestCancelled(err error) bool {
	var requestErr *acp.RequestError
	return errors.As(err, &requestErr) && requestErr.Code == -32800
}

func wrapAuthHint(err error, authMethods []AuthMethod) error {
	if err == nil {
		return nil
	}

	if len(authMethods) == 0 {
		return err
	}

	var requestErr *acp.RequestError
	if errors.As(err, &requestErr) {
		message := strings.ToLower(requestErr.Message)
		if requestErr.Code == -32000 || strings.Contains(message, "auth") || strings.Contains(message, "access denied") {
			return fmt.Errorf("%s. Agent requires authentication; use one of the advertised auth methods and retry", err.Error())
		}
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "auth") || strings.Contains(message, "access denied") {
		return fmt.Errorf("%s. Agent requires authentication; use one of the advertised auth methods and retry", err.Error())
	}
	return err
}
