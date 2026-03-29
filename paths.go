package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func chiselHomeDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("CHISEL_HOME")); override != "" {
		return filepath.Clean(override), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".chisel"), nil
}

func settingsPath() (string, error) {
	root, err := chiselHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "settings.json"), nil
}

func agentCatalogPath() (string, error) {
	root, err := chiselHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "agent-mode-catalog.json"), nil
}

func sessionsDir() (string, error) {
	root, err := chiselHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "sessions"), nil
}

func sessionIndexPath() (string, error) {
	root, err := sessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "index.json"), nil
}

func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func defaultSettings() ChiselSettings {
	return ChiselSettings{
		Agents: map[string]AgentConfig{
			"Kiro CLI": {
				Command: "kiro-cli",
				Args:    []string{"acp"},
				Env:     map[string]string{},
			},
		},
	}
}

func readJSONFile[T any](path string, fallback T) (T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fallback, nil
		}
		return fallback, err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return fallback, nil
	}

	result := fallback
	if err := json.Unmarshal(data, &result); err != nil {
		return fallback, err
	}

	return result, nil
}

func writeJSONFile(path string, value any) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func normalizeSettings(input ChiselSettings) ChiselSettings {
	result := ChiselSettings{
		Agents: map[string]AgentConfig{},
	}

	for name, config := range input.Agents {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			continue
		}

		args := append([]string{}, config.Args...)
		env := map[string]string{}
		for key, value := range config.Env {
			if strings.TrimSpace(key) == "" {
				continue
			}
			env[key] = value
		}

		result.Agents[trimmedName] = AgentConfig{
			Command: strings.TrimSpace(config.Command),
			Args:    args,
			Env:     env,
		}
	}

	if result.Agents == nil {
		result.Agents = map[string]AgentConfig{}
	}

	return result
}

func sortedAgentNames(settings ChiselSettings) []string {
	names := make([]string, 0, len(settings.Agents))
	for name := range settings.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ensurePathWithinRoot(root, target string) error {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	if cleanRoot == cleanTarget {
		return nil
	}

	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, "..") || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return errors.New("path is outside the active workspace")
	}
	return nil
}

func makeSessionTitle(titleHint string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(titleHint)), " ")
	if normalized == "" {
		return "Untitled Session"
	}
	if len(normalized) > 48 {
		return normalized[:45] + "..."
	}
	return normalized
}
