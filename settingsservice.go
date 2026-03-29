package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type SettingsService struct {
	ctx context.Context
}

type agentModeCatalogFile struct {
	Agents map[string]agentModeCatalogEntry `json:"agents"`
}

type agentModeCatalogEntry struct {
	ConfigHash    string               `json:"configHash"`
	CurrentModeID string               `json:"currentModeId"`
	ModelOption   *SessionConfigOption `json:"modelOption,omitempty"`
	Modes         []SessionMode        `json:"modes"`
	DiscoveredAt  string               `json:"discoveredAt"`
}

func NewSettingsService() *SettingsService {
	return &SettingsService{}
}

func (s *SettingsService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.ctx = ctx
	path, err := settingsPath()
	if err != nil {
		return err
	}

	if _, err := readJSONFile(path, defaultSettings()); err != nil {
		return err
	}

	catalogPath, err := agentCatalogPath()
	if err != nil {
		return err
	}

	if _, err := readJSONFile(catalogPath, defaultAgentCatalog()); err != nil {
		return err
	}

	return nil
}

func (s *SettingsService) ServiceShutdown() error {
	return nil
}

func (s *SettingsService) GetSettings() (ChiselSettings, error) {
	path, err := settingsPath()
	if err != nil {
		return ChiselSettings{}, err
	}

	settings, err := readJSONFile(path, defaultSettings())
	if err != nil {
		return ChiselSettings{}, err
	}

	return normalizeSettings(settings), nil
}

func (s *SettingsService) SaveSettings(settings ChiselSettings) error {
	normalized := normalizeSettings(settings)
	for name, config := range normalized.Agents {
		if config.Command == "" {
			return fmt.Errorf("agent %q is missing a command", name)
		}
	}

	path, err := settingsPath()
	if err != nil {
		return err
	}

	return writeJSONFile(path, normalized)
}

func (s *SettingsService) ListAgents() ([]AgentDescriptor, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}

	catalog, err := s.readAgentCatalog()
	if err != nil {
		return nil, err
	}

	names := sortedAgentNames(settings)
	result := make([]AgentDescriptor, 0, len(names))
	for _, name := range names {
		config := settings.Agents[name]
		entry, ok, err := s.cachedAgentCatalog(name, config, catalog)
		if err != nil {
			return nil, err
		}

		descriptor := AgentDescriptor{
			Name:    name,
			Command: config.Command,
			Args:    append([]string{}, config.Args...),
			Env:     copyStringMap(config.Env),
		}
		if ok {
			descriptor.CurrentModeID = entry.CurrentModeID
			descriptor.ModelOption = copySessionConfigOption(entry.ModelOption)
			descriptor.Modes = copySessionModes(entry.Modes)
			descriptor.ModesDiscoveredAt = entry.DiscoveredAt
		}

		result = append(result, descriptor)
	}

	return result, nil
}

func defaultAgentCatalog() agentModeCatalogFile {
	return agentModeCatalogFile{
		Agents: map[string]agentModeCatalogEntry{},
	}
}

func copySessionModes(modes []SessionMode) []SessionMode {
	if len(modes) == 0 {
		return nil
	}

	result := make([]SessionMode, 0, len(modes))
	for _, mode := range modes {
		result = append(result, SessionMode{
			ID:          mode.ID,
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	return result
}

func copySessionConfigValues(values []SessionConfigValue) []SessionConfigValue {
	if len(values) == 0 {
		return nil
	}

	result := make([]SessionConfigValue, 0, len(values))
	for _, value := range values {
		result = append(result, SessionConfigValue{
			Value:       value.Value,
			Name:        value.Name,
			Description: value.Description,
			GroupID:     value.GroupID,
			GroupName:   value.GroupName,
		})
	}
	return result
}

func copySessionConfigOption(option *SessionConfigOption) *SessionConfigOption {
	if option == nil {
		return nil
	}

	return &SessionConfigOption{
		ID:           option.ID,
		Name:         option.Name,
		Description:  option.Description,
		Category:     option.Category,
		Type:         option.Type,
		CurrentValue: option.CurrentValue,
		Values:       copySessionConfigValues(option.Values),
	}
}

func (s *SettingsService) readAgentCatalog() (agentModeCatalogFile, error) {
	path, err := agentCatalogPath()
	if err != nil {
		return agentModeCatalogFile{}, err
	}

	catalog, err := readJSONFile(path, defaultAgentCatalog())
	if err != nil {
		return agentModeCatalogFile{}, err
	}
	if catalog.Agents == nil {
		catalog.Agents = map[string]agentModeCatalogEntry{}
	}
	return catalog, nil
}

func (s *SettingsService) cachedAgentCatalog(agentName string, config AgentConfig, catalog agentModeCatalogFile) (agentModeCatalogEntry, bool, error) {
	entry, ok := catalog.Agents[agentName]
	if !ok {
		return agentModeCatalogEntry{}, false, nil
	}

	configHash, err := agentConfigHash(config)
	if err != nil {
		return agentModeCatalogEntry{}, false, err
	}
	if entry.ConfigHash != configHash {
		return agentModeCatalogEntry{}, false, nil
	}

	entry.Modes = copySessionModes(entry.Modes)
	entry.ModelOption = copySessionConfigOption(entry.ModelOption)
	return entry, true, nil
}

func (s *SettingsService) persistAgentCatalog(agentName string, config AgentConfig, currentModeID string, modes []SessionMode, modelOption *SessionConfigOption) error {
	if agentName == "" {
		return nil
	}

	catalog, err := s.readAgentCatalog()
	if err != nil {
		return err
	}

	configHash, err := agentConfigHash(config)
	if err != nil {
		return err
	}

	existing := catalog.Agents[agentName]
	nextModes := copySessionModes(modes)
	if len(nextModes) == 0 {
		nextModes = copySessionModes(existing.Modes)
	}
	nextModelOption := copySessionConfigOption(modelOption)
	if nextModelOption == nil {
		nextModelOption = copySessionConfigOption(existing.ModelOption)
	}
	if currentModeID == "" {
		currentModeID = existing.CurrentModeID
	}

	catalog.Agents[agentName] = agentModeCatalogEntry{
		ConfigHash:    configHash,
		CurrentModeID: currentModeID,
		ModelOption:   nextModelOption,
		Modes:         nextModes,
		DiscoveredAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}

	path, err := agentCatalogPath()
	if err != nil {
		return err
	}
	return writeJSONFile(path, catalog)
}

func agentConfigHash(config AgentConfig) (string, error) {
	normalized := normalizeSettings(ChiselSettings{
		Agents: map[string]AgentConfig{
			"agent": config,
		},
	})

	payload, err := json.Marshal(normalized.Agents["agent"])
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
