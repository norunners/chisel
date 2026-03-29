package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type SessionService struct {
	settings    *SettingsService
	acp         *ACPService
	permissions *PermissionService

	mu            sync.Mutex
	index         []SessionSummary
	activeSession string
}

func NewSessionService(settings *SettingsService, acp *ACPService, permissions *PermissionService) *SessionService {
	return &SessionService{
		settings:    settings,
		acp:         acp,
		permissions: permissions,
		index:       []SessionSummary{},
	}
}

func (s *SessionService) ServiceStartup(context.Context, application.ServiceOptions) error {
	indexPath, err := sessionIndexPath()
	if err != nil {
		return err
	}

	index, err := readJSONFile(indexPath, []SessionSummary{})
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.index = index
	s.sortLocked()
	s.mu.Unlock()
	return nil
}

func (s *SessionService) ServiceShutdown() error {
	s.acp.closeActive()
	return nil
}

func (s *SessionService) ListSessions() ([]SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := append([]SessionSummary{}, s.index...)
	s.sortLocked()
	return result, nil
}

func (s *SessionService) ListRecentWorkspaces() ([]WorkspaceSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := map[string]WorkspaceSummary{}
	for _, entry := range s.index {
		current, ok := seen[entry.Cwd]
		if !ok || current.LastOpenedAt < entry.LastOpenedAt {
			seen[entry.Cwd] = WorkspaceSummary{
				Path:         entry.Cwd,
				LastOpenedAt: entry.LastOpenedAt,
			}
		}
	}

	result := make([]WorkspaceSummary, 0, len(seen))
	for _, workspace := range seen {
		result = append(result, workspace)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastOpenedAt > result[j].LastOpenedAt
	})
	return result, nil
}

func (s *SessionService) PickWorkspace(initialDir string) (string, error) {
	if !supportsNativeWorkspacePicker() {
		return "", errors.New("workspace picker is only available in the native desktop app; in browser preview or server mode, enter an absolute workspace path instead")
	}

	dialog := application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Select workspace")

	if strings.TrimSpace(initialDir) != "" {
		dialog.SetDirectory(initialDir)
	}

	selection, err := dialog.PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if selection == "" {
		return "", nil
	}
	return filepath.Clean(selection), nil
}

func (s *SessionService) CreateSession(request CreateSessionRequest) (SessionLoadResult, error) {
	settings, err := s.settings.GetSettings()
	if err != nil {
		return SessionLoadResult{}, err
	}

	config, ok := settings.Agents[request.AgentName]
	if !ok {
		return SessionLoadResult{}, errors.New("selected agent is not configured")
	}

	cwd, err := normalizeWorkspacePath(request.Cwd)
	if err != nil {
		return SessionLoadResult{}, err
	}

	client, err := s.acp.openAgent(request.AgentName, config, cwd, s.onSessionEvent)
	if err != nil {
		return SessionLoadResult{}, err
	}

	result, err := client.createSession(context.Background(), cwd)
	if err != nil {
		return SessionLoadResult{}, err
	}

	titleHint := request.TitleHint
	if strings.TrimSpace(titleHint) == "" {
		titleHint = filepath.Base(cwd)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	entry := SessionSummary{
		SessionID:    string(result.SessionId),
		AgentName:    request.AgentName,
		Cwd:          cwd,
		Title:        makeSessionTitle(titleHint),
		CreatedAt:    now,
		UpdatedAt:    now,
		LastOpenedAt: now,
	}

	s.mu.Lock()
	s.activeSession = entry.SessionID
	s.upsertLocked(entry)
	if err := s.saveIndexLocked(); err != nil {
		s.mu.Unlock()
		return SessionLoadResult{}, err
	}
	s.mu.Unlock()

	_ = s.settings.persistAgentCatalog(request.AgentName, config, client.currentMode(), client.sessionModes(), client.currentModelOption())

	return SessionLoadResult{
		Session:            entry,
		ConfigOptions:      client.sessionConfigOptions(),
		CurrentModeID:      client.currentMode(),
		Modes:              client.sessionModes(),
		PromptCapabilities: client.promptCaps,
	}, nil
}

func (s *SessionService) LoadSession(sessionID string) (SessionLoadResult, error) {
	entry, err := s.sessionByID(sessionID)
	if err != nil {
		return SessionLoadResult{}, err
	}

	settings, err := s.settings.GetSettings()
	if err != nil {
		return SessionLoadResult{}, err
	}
	config, ok := settings.Agents[entry.AgentName]
	if !ok {
		return SessionLoadResult{}, errors.New("session agent is no longer configured")
	}

	emitSessionEvent(SessionEvent{
		SessionID: sessionID,
		Kind:      "history_reset",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})

	client, err := s.acp.openAgent(entry.AgentName, config, entry.Cwd, s.onSessionEvent)
	if err != nil {
		return SessionLoadResult{}, err
	}

	if _, err := client.loadSession(context.Background(), entry.SessionID, entry.Cwd); err != nil {
		return SessionLoadResult{}, err
	}

	entry.LastOpenedAt = time.Now().UTC().Format(time.RFC3339Nano)
	entry.UpdatedAt = entry.LastOpenedAt

	s.mu.Lock()
	s.activeSession = sessionID
	s.upsertLocked(entry)
	if err := s.saveIndexLocked(); err != nil {
		s.mu.Unlock()
		return SessionLoadResult{}, err
	}
	s.mu.Unlock()

	_ = s.settings.persistAgentCatalog(entry.AgentName, config, client.currentMode(), client.sessionModes(), client.currentModelOption())

	return SessionLoadResult{
		Session:            entry,
		ConfigOptions:      client.sessionConfigOptions(),
		CurrentModeID:      client.currentMode(),
		Modes:              client.sessionModes(),
		PromptCapabilities: client.promptCaps,
	}, nil
}

func (s *SessionService) ForgetSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.index[:0]
	for _, entry := range s.index {
		if entry.SessionID == sessionID {
			continue
		}
		filtered = append(filtered, entry)
	}
	s.index = append([]SessionSummary{}, filtered...)
	if s.activeSession == sessionID {
		s.activeSession = ""
		s.acp.closeActive()
	}
	return s.saveIndexLocked()
}

func (s *SessionService) SendPrompt(sessionID, prompt string) error {
	client := s.acp.activeClient()
	if client == nil || s.activeSessionID() != sessionID {
		return errors.New("session is not active")
	}
	if strings.TrimSpace(prompt) == "" {
		return errors.New("prompt cannot be empty")
	}

	s.mu.Lock()
	for index, entry := range s.index {
		if entry.SessionID != sessionID {
			continue
		}
		if entry.Title == "Untitled Session" {
			entry.Title = makeSessionTitle(prompt)
		}
		entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.index[index] = entry
		break
	}
	_ = s.saveIndexLocked()
	s.mu.Unlock()

	client.promptAsync(sessionID, prompt)
	return nil
}

func (s *SessionService) CancelPrompt(sessionID string) error {
	client := s.acp.activeClient()
	if client == nil || s.activeSessionID() != sessionID {
		return errors.New("session is not active")
	}
	return client.cancelPrompt(sessionID)
}

func (s *SessionService) SetConfigOption(sessionID, optionID, value string) (SessionLoadResult, error) {
	client := s.acp.activeClient()
	if client == nil || s.activeSessionID() != sessionID {
		return SessionLoadResult{}, errors.New("session is not active")
	}
	if err := client.setConfigOption(context.Background(), sessionID, optionID, value); err != nil {
		return SessionLoadResult{}, err
	}
	entry, err := s.sessionByID(sessionID)
	if err != nil {
		return SessionLoadResult{}, err
	}
	_ = s.settings.persistAgentCatalog(client.agentName, client.config, client.currentMode(), client.sessionModes(), client.currentModelOption())
	return SessionLoadResult{
		Session:            entry,
		ConfigOptions:      client.sessionConfigOptions(),
		CurrentModeID:      client.currentMode(),
		Modes:              client.sessionModes(),
		PromptCapabilities: client.promptCaps,
	}, nil
}

func (s *SessionService) SetMode(sessionID, modeID string) (SessionLoadResult, error) {
	client := s.acp.activeClient()
	if client == nil || s.activeSessionID() != sessionID {
		return SessionLoadResult{}, errors.New("session is not active")
	}
	if err := client.setMode(context.Background(), sessionID, modeID); err != nil {
		return SessionLoadResult{}, err
	}
	entry, err := s.sessionByID(sessionID)
	if err != nil {
		return SessionLoadResult{}, err
	}
	_ = s.settings.persistAgentCatalog(client.agentName, client.config, client.currentMode(), client.sessionModes(), client.currentModelOption())
	return SessionLoadResult{
		Session:            entry,
		ConfigOptions:      client.sessionConfigOptions(),
		CurrentModeID:      client.currentMode(),
		Modes:              client.sessionModes(),
		PromptCapabilities: client.promptCaps,
	}, nil
}

func (s *SessionService) onSessionEvent(event SessionEvent) {
	if event.SessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for index, entry := range s.index {
		if entry.SessionID != event.SessionID {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		entry.UpdatedAt = now
		if event.Kind == "session_info_update" && strings.TrimSpace(event.Title) != "" {
			entry.Title = event.Title
		}
		s.index[index] = entry
		break
	}
	_ = s.saveIndexLocked()
}

func (s *SessionService) sessionByID(sessionID string) (SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entry := range s.index {
		if entry.SessionID == sessionID {
			return entry, nil
		}
	}
	return SessionSummary{}, errors.New("session not found")
}

func (s *SessionService) activeSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeSession
}

func (s *SessionService) upsertLocked(entry SessionSummary) {
	for index, current := range s.index {
		if current.SessionID == entry.SessionID {
			s.index[index] = entry
			s.sortLocked()
			return
		}
	}
	s.index = append(s.index, entry)
	s.sortLocked()
}

func (s *SessionService) sortLocked() {
	sort.Slice(s.index, func(i, j int) bool {
		left := firstNonEmpty(s.index[i].LastOpenedAt, s.index[i].UpdatedAt, s.index[i].CreatedAt)
		right := firstNonEmpty(s.index[j].LastOpenedAt, s.index[j].UpdatedAt, s.index[j].CreatedAt)
		return left > right
	})
}

func (s *SessionService) saveIndexLocked() error {
	path, err := sessionIndexPath()
	if err != nil {
		return err
	}
	return writeJSONFile(path, s.index)
}

func normalizeWorkspacePath(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", errors.New("workspace is required")
	}
	absolute, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("workspace must be a directory")
	}
	return absolute, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
