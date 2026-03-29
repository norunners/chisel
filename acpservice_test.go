package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

type testACPAgent struct {
	mu sync.Mutex

	conn *acp.AgentSideConnection

	initResponse        acp.InitializeResponse
	newSessionResponse  acp.NewSessionResponse
	loadSessionResponse acp.LoadSessionResponse
	setConfigResponse   acp.SetSessionConfigOptionResponse
	promptResponse      acp.PromptResponse

	authenticateRequests []acp.AuthenticateRequest
	newSessionRequests   []acp.NewSessionRequest
	loadSessionRequests  []acp.LoadSessionRequest
	setConfigRequests    []acp.SetSessionConfigOptionRequest
	setModeRequests      []acp.SetSessionModeRequest
	setModelRequests     []acp.UnstableSetSessionModelRequest
	promptRequests       []acp.PromptRequest

	onPrompt    func(context.Context, acp.PromptRequest) (acp.PromptResponse, error)
	onSetMode   func(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error)
	onSetConfig func(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error)
	onSetModel  func(context.Context, acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error)
}

func (a *testACPAgent) Authenticate(ctx context.Context, params acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	a.mu.Lock()
	a.authenticateRequests = append(a.authenticateRequests, params)
	a.mu.Unlock()
	return acp.AuthenticateResponse{}, nil
}

func (a *testACPAgent) Initialize(ctx context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
	response := a.initResponse
	if response.ProtocolVersion == 0 {
		response.ProtocolVersion = acp.ProtocolVersion(acp.ProtocolVersionNumber)
	}
	if response.AgentInfo == nil {
		response.AgentInfo = &acp.Implementation{
			Name:    "test-agent",
			Title:   acp.Ptr("Test Agent"),
			Version: "1.0.0",
		}
	}
	return response, nil
}

func (a *testACPAgent) Cancel(ctx context.Context, params acp.CancelNotification) error {
	return nil
}

func (a *testACPAgent) NewSession(ctx context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	a.mu.Lock()
	a.newSessionRequests = append(a.newSessionRequests, params)
	a.mu.Unlock()

	response := a.newSessionResponse
	if response.SessionId == "" {
		response.SessionId = acp.SessionId("session-1")
	}
	return response, nil
}

func (a *testACPAgent) LoadSession(ctx context.Context, params acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	a.mu.Lock()
	a.loadSessionRequests = append(a.loadSessionRequests, params)
	a.mu.Unlock()
	return a.loadSessionResponse, nil
}

func (a *testACPAgent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	a.mu.Lock()
	a.promptRequests = append(a.promptRequests, params)
	a.mu.Unlock()

	if a.onPrompt != nil {
		return a.onPrompt(ctx, params)
	}
	if a.promptResponse.StopReason == "" {
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}
	return a.promptResponse, nil
}

func (a *testACPAgent) SetSessionConfigOption(ctx context.Context, params acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	a.mu.Lock()
	a.setConfigRequests = append(a.setConfigRequests, params)
	a.mu.Unlock()

	if a.onSetConfig != nil {
		return a.onSetConfig(ctx, params)
	}
	return a.setConfigResponse, nil
}

func (a *testACPAgent) SetSessionMode(ctx context.Context, params acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	a.mu.Lock()
	a.setModeRequests = append(a.setModeRequests, params)
	a.mu.Unlock()

	if a.onSetMode != nil {
		return a.onSetMode(ctx, params)
	}
	return acp.SetSessionModeResponse{}, nil
}

func (a *testACPAgent) UnstableSetSessionModel(ctx context.Context, params acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error) {
	a.mu.Lock()
	a.setModelRequests = append(a.setModelRequests, params)
	a.mu.Unlock()

	if a.onSetModel != nil {
		return a.onSetModel(ctx, params)
	}
	return acp.UnstableSetSessionModelResponse{}, nil
}

func newConnectedACPClient(t *testing.T, agent *testACPAgent, workspaceRoot string, onEvent func(SessionEvent)) (*acpClient, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()

	client := newACPClient(
		"Test Agent",
		AgentConfig{Command: "test-agent"},
		workspaceRoot,
		NewPermissionService(),
		NewTerminalManager(),
		onEvent,
		ctx,
		cancel,
		nil,
		clientToAgentWriter,
		agentToClientReader,
	)

	agent.conn = acp.NewAgentSideConnection(agent, agentToClientWriter, clientToAgentReader)
	if err := client.initialize(ctx); err != nil {
		t.Fatalf("initialize client: %v", err)
	}

	cleanup := func() {
		client.close()
		_ = agentToClientWriter.Close()
		_ = agentToClientReader.Close()
		_ = clientToAgentWriter.Close()
		_ = clientToAgentReader.Close()
	}

	return client, cleanup
}

func makeModeState(current string, modes ...string) *acp.SessionModeState {
	available := make([]acp.SessionMode, 0, len(modes))
	for _, mode := range modes {
		available = append(available, acp.SessionMode{
			Id:          acp.SessionModeId(mode),
			Name:        strings.ToUpper(mode),
			Description: acp.Ptr(mode + " mode"),
		})
	}
	return &acp.SessionModeState{
		CurrentModeId:  acp.SessionModeId(current),
		AvailableModes: available,
	}
}

func makeModelState(current string, models ...string) *acp.SessionModelState {
	available := make([]acp.ModelInfo, 0, len(models))
	for _, model := range models {
		available = append(available, acp.ModelInfo{
			ModelId: acp.ModelId(model),
			Name:    strings.ToUpper(model),
		})
	}

	return &acp.SessionModelState{
		CurrentModelId:  acp.ModelId(current),
		AvailableModels: available,
	}
}

func makeSelectOption(id, current string, values ...string) acp.SessionConfigOption {
	options := make(acp.SessionConfigSelectOptionsUngrouped, 0, len(values))
	for _, value := range values {
		options = append(options, acp.SessionConfigSelectOption{
			Value: acp.SessionConfigValueId(value),
			Name:  strings.ToUpper(value),
		})
	}

	return acp.SessionConfigOption{
		Select: &acp.SessionConfigOptionSelect{
			Id:           acp.SessionConfigId(id),
			Name:         strings.Title(id),
			CurrentValue: acp.SessionConfigValueId(current),
			Type:         "select",
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &options,
			},
		},
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestACPClientInitializeAndAuthenticate(t *testing.T) {
	agent := &testACPAgent{
		initResponse: acp.InitializeResponse{
			ProtocolVersion: acp.ProtocolVersion(acp.ProtocolVersionNumber),
			AgentCapabilities: acp.AgentCapabilities{
				LoadSession: true,
				PromptCapabilities: acp.PromptCapabilities{
					Image:           true,
					Audio:           true,
					EmbeddedContext: true,
				},
			},
			AgentInfo: &acp.Implementation{
				Name:    "test-agent",
				Title:   acp.Ptr("Test Agent"),
				Version: "1.2.3",
			},
			AuthMethods: []acp.AuthMethod{
				{
					Id:          "browser",
					Name:        "Browser",
					Description: acp.Ptr("Open the browser to sign in"),
				},
			},
		},
	}

	client, cleanup := newConnectedACPClient(t, agent, t.TempDir(), nil)
	defer cleanup()

	if client.agentTitle != "Test Agent" {
		t.Fatalf("unexpected agent title: %q", client.agentTitle)
	}
	if client.agentVersion() != "1.2.3" {
		t.Fatalf("unexpected agent version: %q", client.agentVersion())
	}
	if !client.loadSessionSupported {
		t.Fatal("expected loadSession support")
	}
	if !client.promptCaps.Image || !client.promptCaps.Audio || !client.promptCaps.EmbeddedContext {
		t.Fatalf("unexpected prompt capabilities: %+v", client.promptCaps)
	}
	if len(client.authMethods) != 1 || client.authMethods[0].ID != "browser" {
		t.Fatalf("unexpected auth methods: %+v", client.authMethods)
	}

	if err := client.authenticate(context.Background(), "browser"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	if len(agent.authenticateRequests) != 1 || agent.authenticateRequests[0].MethodId != "browser" {
		t.Fatalf("unexpected authenticate requests: %+v", agent.authenticateRequests)
	}
}

func TestACPClientSessionRequestsIncludeMCPServersAndNormalizeState(t *testing.T) {
	agent := &testACPAgent{
		initResponse: acp.InitializeResponse{
			ProtocolVersion: acp.ProtocolVersion(acp.ProtocolVersionNumber),
			AgentCapabilities: acp.AgentCapabilities{
				LoadSession: true,
			},
		},
		newSessionResponse: acp.NewSessionResponse{
			SessionId:     acp.SessionId("session-1"),
			ConfigOptions: []acp.SessionConfigOption{makeSelectOption("model", "gpt-4o", "gpt-4o", "gpt-5")},
			Modes:         makeModeState("code", "code", "plan"),
		},
		loadSessionResponse: acp.LoadSessionResponse{
			ConfigOptions: []acp.SessionConfigOption{makeSelectOption("model", "gpt-5", "gpt-4o", "gpt-5")},
			Modes:         makeModeState("plan", "code", "plan"),
		},
	}

	client, cleanup := newConnectedACPClient(t, agent, t.TempDir(), nil)
	defer cleanup()

	result, err := client.createSession(context.Background(), "/tmp/workspace")
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	if string(result.SessionId) != "session-1" {
		t.Fatalf("unexpected session id: %q", result.SessionId)
	}
	if len(agent.newSessionRequests) != 1 {
		t.Fatalf("expected one new session request, got %d", len(agent.newSessionRequests))
	}
	if agent.newSessionRequests[0].McpServers == nil || len(agent.newSessionRequests[0].McpServers) != 0 {
		t.Fatalf("expected empty MCP server list, got %+v", agent.newSessionRequests[0].McpServers)
	}
	if client.currentMode() != "code" {
		t.Fatalf("unexpected current mode after create: %q", client.currentMode())
	}
	if len(client.sessionModes()) != 2 {
		t.Fatalf("unexpected session modes after create: %+v", client.sessionModes())
	}
	if got := client.sessionConfigOptions()[0].CurrentValue; got != "gpt-4o" {
		t.Fatalf("unexpected config option after create: %q", got)
	}

	if _, err := client.loadSession(context.Background(), "session-1", "/tmp/workspace"); err != nil {
		t.Fatalf("loadSession: %v", err)
	}
	if len(agent.loadSessionRequests) != 1 {
		t.Fatalf("expected one load session request, got %d", len(agent.loadSessionRequests))
	}
	if agent.loadSessionRequests[0].McpServers == nil || len(agent.loadSessionRequests[0].McpServers) != 0 {
		t.Fatalf("expected empty load MCP server list, got %+v", agent.loadSessionRequests[0].McpServers)
	}
	if client.currentMode() != "plan" {
		t.Fatalf("unexpected current mode after load: %q", client.currentMode())
	}
	if got := client.sessionConfigOptions()[0].CurrentValue; got != "gpt-5" {
		t.Fatalf("unexpected config option after load: %q", got)
	}
}

func TestACPClientSetConfigOptionUsesConfigIDAndUpdatesState(t *testing.T) {
	agent := &testACPAgent{
		initResponse: acp.InitializeResponse{
			ProtocolVersion: acp.ProtocolVersion(acp.ProtocolVersionNumber),
			AgentCapabilities: acp.AgentCapabilities{
				LoadSession: true,
			},
		},
		newSessionResponse: acp.NewSessionResponse{
			SessionId:     acp.SessionId("session-1"),
			ConfigOptions: []acp.SessionConfigOption{makeSelectOption("model", "gpt-4o", "gpt-4o", "gpt-5")},
			Modes:         makeModeState("code", "code", "plan"),
		},
		setConfigResponse: acp.SetSessionConfigOptionResponse{
			ConfigOptions: []acp.SessionConfigOption{makeSelectOption("model", "gpt-5", "gpt-4o", "gpt-5")},
		},
	}

	client, cleanup := newConnectedACPClient(t, agent, t.TempDir(), nil)
	defer cleanup()

	if _, err := client.createSession(context.Background(), "/tmp/workspace"); err != nil {
		t.Fatalf("createSession: %v", err)
	}
	if err := client.setConfigOption(context.Background(), "session-1", "model", "gpt-5"); err != nil {
		t.Fatalf("setConfigOption: %v", err)
	}

	if len(agent.setConfigRequests) != 1 {
		t.Fatalf("expected one config request, got %d", len(agent.setConfigRequests))
	}
	if agent.setConfigRequests[0].ConfigId != acp.SessionConfigId("model") {
		t.Fatalf("unexpected config id: %q", agent.setConfigRequests[0].ConfigId)
	}
	if got := client.sessionConfigOptions()[0].CurrentValue; got != "gpt-5" {
		t.Fatalf("unexpected current value: %q", got)
	}
}

func TestACPClientSynthesizesModelOptionFromACPModelState(t *testing.T) {
	agent := &testACPAgent{
		newSessionResponse: acp.NewSessionResponse{
			SessionId: "session-1",
			Models:    makeModelState("gpt-5.4", "gpt-5.4", "gpt-5.4-mini"),
		},
	}

	client, cleanup := newConnectedACPClient(t, agent, t.TempDir(), nil)
	defer cleanup()

	if _, err := client.createSession(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("create session: %v", err)
	}

	options := client.sessionConfigOptions()
	if len(options) != 1 {
		t.Fatalf("expected one synthetic model option, got %+v", options)
	}
	if options[0].ID != syntheticModelConfigID {
		t.Fatalf("unexpected synthetic option id: %q", options[0].ID)
	}
	if options[0].CurrentValue != "gpt-5.4" {
		t.Fatalf("unexpected current model: %q", options[0].CurrentValue)
	}

	if err := client.setConfigOption(context.Background(), "session-1", syntheticModelConfigID, "gpt-5.4-mini"); err != nil {
		t.Fatalf("set synthetic model option: %v", err)
	}

	if len(agent.setModelRequests) != 1 {
		t.Fatalf("expected unstable set model request, got %d", len(agent.setModelRequests))
	}
	if agent.setModelRequests[0].ModelId != acp.UnstableModelId("gpt-5.4-mini") {
		t.Fatalf("unexpected model id: %q", agent.setModelRequests[0].ModelId)
	}
	if len(agent.setConfigRequests) != 0 {
		t.Fatalf("expected no config option requests, got %d", len(agent.setConfigRequests))
	}

	options = client.sessionConfigOptions()
	if options[0].CurrentValue != "gpt-5.4-mini" {
		t.Fatalf("expected current model to update, got %q", options[0].CurrentValue)
	}
}

func TestACPClientSetModeUsesSDKStateUpdate(t *testing.T) {
	agent := &testACPAgent{
		initResponse: acp.InitializeResponse{
			ProtocolVersion: acp.ProtocolVersion(acp.ProtocolVersionNumber),
			AgentCapabilities: acp.AgentCapabilities{
				LoadSession: true,
			},
		},
		newSessionResponse: acp.NewSessionResponse{
			SessionId:     acp.SessionId("session-1"),
			ConfigOptions: []acp.SessionConfigOption{makeSelectOption("model", "gpt-4o", "gpt-4o", "gpt-5")},
			Modes:         makeModeState("code", "code", "plan"),
		},
	}
	agent.onSetMode = func(ctx context.Context, params acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
		if err := agent.conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update: acp.SessionUpdate{
				CurrentModeUpdate: &acp.SessionCurrentModeUpdate{
					SessionUpdate: "current_mode_update",
					CurrentModeId: params.ModeId,
				},
			},
		}); err != nil {
			return acp.SetSessionModeResponse{}, err
		}
		return acp.SetSessionModeResponse{}, nil
	}

	client, cleanup := newConnectedACPClient(t, agent, t.TempDir(), nil)
	defer cleanup()

	if _, err := client.createSession(context.Background(), "/tmp/workspace"); err != nil {
		t.Fatalf("createSession: %v", err)
	}
	if err := client.setMode(context.Background(), "session-1", "plan"); err != nil {
		t.Fatalf("setMode: %v", err)
	}

	if len(agent.setModeRequests) != 1 || agent.setModeRequests[0].ModeId != acp.SessionModeId("plan") {
		t.Fatalf("unexpected set mode requests: %+v", agent.setModeRequests)
	}
	if client.currentMode() != "plan" {
		t.Fatalf("unexpected current mode: %q", client.currentMode())
	}
}

func TestACPClientAdapterReadTextFileSupportsLineAndLimit(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\nline4\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	client := &acpClient{
		workspaceRoot: workspace,
		permissions:   NewPermissionService(),
		terminals:     NewTerminalManager(),
	}
	adapter := &acpClientAdapter{client: client}

	response, err := adapter.ReadTextFile(context.Background(), acp.ReadTextFileRequest{
		SessionId: acp.SessionId("session-1"),
		Path:      path,
		Line:      acp.Ptr(2),
		Limit:     acp.Ptr(2),
	})
	if err != nil {
		t.Fatalf("ReadTextFile: %v", err)
	}

	if response.Content != "line2\nline3" {
		t.Fatalf("unexpected sliced content: %q", response.Content)
	}
}

func TestACPClientAdapterPermissionSelectionAndCancellation(t *testing.T) {
	t.Run("selected", func(t *testing.T) {
		client := &acpClient{
			workspaceRoot: t.TempDir(),
			permissions:   NewPermissionService(),
			terminals:     NewTerminalManager(),
		}
		adapter := &acpClientAdapter{client: client}

		done := make(chan acp.RequestPermissionResponse, 1)
		errs := make(chan error, 1)
		go func() {
			response, err := adapter.RequestPermission(context.Background(), acp.RequestPermissionRequest{
				SessionId: acp.SessionId("session-1"),
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: acp.ToolCallId("tool-1"),
					Title:      acp.Ptr("Edit file"),
				},
				Options: []acp.PermissionOption{
					{OptionId: acp.PermissionOptionId("allow"), Name: "Allow", Kind: acp.PermissionOptionKindAllowOnce},
				},
			})
			if err != nil {
				errs <- err
				return
			}
			done <- response
		}()

		waitFor(t, func() bool {
			return len(client.permissions.ListPendingPermissions()) == 1
		})

		pending := client.permissions.ListPendingPermissions()[0]
		if err := client.permissions.ResolvePermission(pending.RequestID, "allow", false); err != nil {
			t.Fatalf("ResolvePermission: %v", err)
		}

		select {
		case err := <-errs:
			t.Fatalf("RequestPermission returned error: %v", err)
		case response := <-done:
			if response.Outcome.Selected == nil || response.Outcome.Selected.OptionId != acp.PermissionOptionId("allow") {
				t.Fatalf("unexpected selected outcome: %+v", response.Outcome)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for selected permission response")
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		client := &acpClient{
			workspaceRoot: t.TempDir(),
			permissions:   NewPermissionService(),
			terminals:     NewTerminalManager(),
		}
		adapter := &acpClientAdapter{client: client}

		done := make(chan acp.RequestPermissionResponse, 1)
		errs := make(chan error, 1)
		go func() {
			response, err := adapter.RequestPermission(context.Background(), acp.RequestPermissionRequest{
				SessionId: acp.SessionId("session-1"),
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: acp.ToolCallId("tool-1"),
					Title:      acp.Ptr("Edit file"),
				},
				Options: []acp.PermissionOption{
					{OptionId: acp.PermissionOptionId("allow"), Name: "Allow", Kind: acp.PermissionOptionKindAllowOnce},
				},
			})
			if err != nil {
				errs <- err
				return
			}
			done <- response
		}()

		waitFor(t, func() bool {
			return len(client.permissions.ListPendingPermissions()) == 1
		})

		pending := client.permissions.ListPendingPermissions()[0]
		if err := client.permissions.ResolvePermission(pending.RequestID, "", true); err != nil {
			t.Fatalf("ResolvePermission cancel: %v", err)
		}

		select {
		case err := <-errs:
			t.Fatalf("RequestPermission returned error: %v", err)
		case response := <-done:
			if response.Outcome.Cancelled == nil {
				t.Fatalf("expected cancelled outcome, got %+v", response.Outcome)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for cancelled permission response")
		}
	})
}

func TestACPClientAdapterTerminalLifecycle(t *testing.T) {
	client := &acpClient{
		ctx:           context.Background(),
		workspaceRoot: t.TempDir(),
		permissions:   NewPermissionService(),
		terminals:     NewTerminalManager(),
	}
	adapter := &acpClientAdapter{client: client}

	createResponse, err := adapter.CreateTerminal(context.Background(), acp.CreateTerminalRequest{
		SessionId: acp.SessionId("session-1"),
		Command:   "sh",
		Args:      []string{"-c", "printf hello"},
	})
	if err != nil {
		t.Fatalf("CreateTerminal: %v", err)
	}
	if createResponse.TerminalId == "" {
		t.Fatal("expected terminal id")
	}

	if _, err := adapter.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{
		SessionId:  acp.SessionId("session-1"),
		TerminalId: createResponse.TerminalId,
	}); err != nil {
		t.Fatalf("WaitForTerminalExit: %v", err)
	}

	outputResponse, err := adapter.TerminalOutput(context.Background(), acp.TerminalOutputRequest{
		SessionId:  acp.SessionId("session-1"),
		TerminalId: createResponse.TerminalId,
	})
	if err != nil {
		t.Fatalf("TerminalOutput: %v", err)
	}
	if !strings.Contains(outputResponse.Output, "hello") {
		t.Fatalf("unexpected terminal output: %q", outputResponse.Output)
	}

	if _, err := adapter.ReleaseTerminal(context.Background(), acp.ReleaseTerminalRequest{
		SessionId:  acp.SessionId("session-1"),
		TerminalId: createResponse.TerminalId,
	}); err != nil {
		t.Fatalf("ReleaseTerminal: %v", err)
	}
}

func TestACPClientSessionUpdateNormalization(t *testing.T) {
	var (
		mu     sync.Mutex
		events []SessionEvent
	)

	client := &acpClient{
		workspaceRoot: t.TempDir(),
		permissions:   NewPermissionService(),
		terminals:     NewTerminalManager(),
		onEvent: func(event SessionEvent) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
		},
	}
	adapter := &acpClientAdapter{client: client}

	if err := adapter.SessionUpdate(context.Background(), acp.SessionNotification{
		SessionId: acp.SessionId("session-1"),
		Update:    acp.UpdateAgentMessageText("Hello"),
	}); err != nil {
		t.Fatalf("agent message update: %v", err)
	}

	if err := adapter.SessionUpdate(context.Background(), acp.SessionNotification{
		SessionId: acp.SessionId("session-1"),
		Update: acp.SessionUpdate{
			CurrentModeUpdate: &acp.SessionCurrentModeUpdate{
				SessionUpdate: "current_mode_update",
				CurrentModeId: acp.SessionModeId("plan"),
			},
		},
	}); err != nil {
		t.Fatalf("mode update: %v", err)
	}

	if err := adapter.SessionUpdate(context.Background(), acp.SessionNotification{
		SessionId: acp.SessionId("session-1"),
		Update: acp.SessionUpdate{
			ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
				SessionUpdate: "config_option_update",
				ConfigOptions: []acp.SessionConfigOption{makeSelectOption("model", "gpt-5", "gpt-4o", "gpt-5")},
			},
		},
	}); err != nil {
		t.Fatalf("config update: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) != 3 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	if events[0].Kind != "message_chunk" || events[0].Text != "Hello" {
		t.Fatalf("unexpected message event: %+v", events[0])
	}
	if events[1].Kind != "mode_update" || events[1].CurrentModeID != "plan" {
		t.Fatalf("unexpected mode event: %+v", events[1])
	}
	if events[2].Kind != "config_options_update" {
		t.Fatalf("unexpected config event: %+v", events[2])
	}
	if got := client.currentMode(); got != "plan" {
		t.Fatalf("unexpected client mode state: %q", got)
	}
	if got := client.sessionConfigOptions()[0].CurrentValue; got != "gpt-5" {
		t.Fatalf("unexpected client config state: %q", got)
	}
}

func TestACPClientCancelPromptEmitsCancelledTurn(t *testing.T) {
	var (
		mu     sync.Mutex
		events []SessionEvent
	)

	agent := &testACPAgent{
		initResponse: acp.InitializeResponse{
			ProtocolVersion: acp.ProtocolVersion(acp.ProtocolVersionNumber),
			AgentCapabilities: acp.AgentCapabilities{
				LoadSession: true,
			},
		},
	}
	agent.onPrompt = func(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
		<-ctx.Done()
		return acp.PromptResponse{}, ctx.Err()
	}

	client, cleanup := newConnectedACPClient(t, agent, t.TempDir(), func(event SessionEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	defer cleanup()

	client.promptAsync("session-1", "cancel me")
	time.Sleep(50 * time.Millisecond)

	if err := client.cancelPrompt("session-1"); err != nil {
		t.Fatalf("cancelPrompt: %v", err)
	}

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, event := range events {
			if event.Kind == "turn_end" && event.StopReason == "cancelled" {
				return true
			}
		}
		return false
	})
}

func TestWrapAuthHintAddsAuthenticateGuidance(t *testing.T) {
	err := wrapAuthHint(&acp.RequestError{
		Code:    -32000,
		Message: "Authentication required",
	}, []AuthMethod{{ID: "browser", Name: "Browser"}})

	if err == nil || !strings.Contains(err.Error(), "advertised auth methods") {
		t.Fatalf("unexpected wrapped auth hint: %v", err)
	}
}

func TestIsACPRequestCancelled(t *testing.T) {
	if !isACPRequestCancelled(&acp.RequestError{Code: -32800, Message: "Request cancelled"}) {
		t.Fatal("expected request cancelled error to be detected")
	}
	if isACPRequestCancelled(errors.New("boom")) {
		t.Fatal("did not expect generic error to be detected as request cancelled")
	}
}
