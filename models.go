package main

import "github.com/wailsapp/wails/v3/pkg/application"

func init() {
	application.RegisterEvent[SessionEvent]("acp:event")
}

type ChiselSettings struct {
	Agents map[string]AgentConfig `json:"agents"`
}

type AgentConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

type AgentDescriptor struct {
	Name              string               `json:"name"`
	Command           string               `json:"command"`
	Args              []string             `json:"args"`
	Env               map[string]string    `json:"env"`
	CurrentModeID     string               `json:"currentModeId"`
	ModelOption       *SessionConfigOption `json:"modelOption,omitempty"`
	Modes             []SessionMode        `json:"modes"`
	ModesDiscoveredAt string               `json:"modesDiscoveredAt"`
}

type AgentTestResult struct {
	AgentName          string             `json:"agentName"`
	AgentTitle         string             `json:"agentTitle"`
	AgentVersion       string             `json:"agentVersion"`
	ProtocolVersion    int                `json:"protocolVersion"`
	PromptCapabilities PromptCapabilities `json:"promptCapabilities"`
	LoadSession        bool               `json:"loadSession"`
	AuthMethods        []AuthMethod       `json:"authMethods"`
}

type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type SessionSummary struct {
	SessionID    string `json:"sessionId"`
	AgentName    string `json:"agentName"`
	Cwd          string `json:"cwd"`
	Title        string `json:"title"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	LastOpenedAt string `json:"lastOpenedAt"`
}

type WorkspaceSummary struct {
	Path         string `json:"path"`
	LastOpenedAt string `json:"lastOpenedAt"`
}

type CreateSessionRequest struct {
	AgentName string `json:"agentName"`
	Cwd       string `json:"cwd"`
	TitleHint string `json:"titleHint"`
}

type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SessionConfigValue struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description"`
	GroupID     string `json:"groupId"`
	GroupName   string `json:"groupName"`
}

type SessionConfigOption struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Category     string               `json:"category"`
	Type         string               `json:"type"`
	CurrentValue string               `json:"currentValue"`
	Values       []SessionConfigValue `json:"values"`
}

type SessionLoadResult struct {
	Session            SessionSummary        `json:"session"`
	ConfigOptions      []SessionConfigOption `json:"configOptions"`
	CurrentModeID      string                `json:"currentModeId"`
	Modes              []SessionMode         `json:"modes"`
	PromptCapabilities PromptCapabilities    `json:"promptCapabilities"`
}

type PermissionOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

type PermissionRequest struct {
	RequestID   string             `json:"requestId"`
	SessionID   string             `json:"sessionId"`
	ToolCallID  string             `json:"toolCallId"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Options     []PermissionOption `json:"options"`
	CreatedAt   string             `json:"createdAt"`
}

type ToolCallRecord struct {
	ToolCallID string            `json:"toolCallId"`
	Title      string            `json:"title"`
	Kind       string            `json:"kind"`
	Status     string            `json:"status"`
	Content    []ToolCallContent `json:"content"`
}

type ToolCallContent struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Path        string `json:"path"`
	URI         string `json:"uri"`
	MimeType    string `json:"mimeType"`
	TerminalID  string `json:"terminalId"`
	Output      string `json:"output"`
	ExitCode    int    `json:"exitCode"`
	ExitCodeSet bool   `json:"exitCodeSet"`
	IsTruncated bool   `json:"isTruncated"`
	Description string `json:"description"`
}

type PlanEntry struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

type SessionEvent struct {
	SessionID     string                `json:"sessionId"`
	Kind          string                `json:"kind"`
	Timestamp     string                `json:"timestamp"`
	MessageRole   string                `json:"messageRole,omitempty"`
	Text          string                `json:"text,omitempty"`
	Thought       string                `json:"thought,omitempty"`
	ToolCall      *ToolCallRecord       `json:"toolCall,omitempty"`
	Permission    *PermissionRequest    `json:"permission,omitempty"`
	StopReason    string                `json:"stopReason,omitempty"`
	Title         string                `json:"title,omitempty"`
	ConfigOptions []SessionConfigOption `json:"configOptions,omitempty"`
	CurrentModeID string                `json:"currentModeId,omitempty"`
	Modes         []SessionMode         `json:"modes,omitempty"`
	Plan          []PlanEntry           `json:"plan,omitempty"`
	Error         string                `json:"error,omitempty"`
	Method        string                `json:"method,omitempty"`
	Raw           string                `json:"raw,omitempty"`
}
