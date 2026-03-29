package main

import "testing"

func TestSettingsServiceListAgentsIncludesCachedModes(t *testing.T) {
	t.Setenv("CHISEL_HOME", t.TempDir())

	service := NewSettingsService()
	settings := ChiselSettings{
		Agents: map[string]AgentConfig{
			"Kiro CLI": {
				Command: "kiro-cli",
				Args:    []string{"acp"},
				Env:     map[string]string{},
			},
		},
	}

	if err := service.SaveSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	modes := []SessionMode{
		{ID: "kiro_default", Name: "kiro_default", Description: "Default mode"},
		{ID: "kiro_planning", Name: "kiro_planning", Description: "Planning mode"},
	}
	modelOption := &SessionConfigOption{
		ID:           "model",
		Name:         "Model",
		Category:     "model",
		Type:         "select",
		CurrentValue: "gpt-5.4",
		Values: []SessionConfigValue{
			{Value: "gpt-5.4", Name: "GPT-5.4"},
			{Value: "gpt-5.4-mini", Name: "GPT-5.4 Mini"},
		},
	}
	if err := service.persistAgentCatalog("Kiro CLI", settings.Agents["Kiro CLI"], "kiro_default", modes, modelOption); err != nil {
		t.Fatalf("persist agent catalog: %v", err)
	}

	agents, err := service.ListAgents()
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("unexpected agent count: %d", len(agents))
	}
	if agents[0].CurrentModeID != "kiro_default" {
		t.Fatalf("unexpected current mode id: %q", agents[0].CurrentModeID)
	}
	if len(agents[0].Modes) != 2 {
		t.Fatalf("unexpected mode count: %d", len(agents[0].Modes))
	}
	if agents[0].ModelOption == nil {
		t.Fatal("expected cached model option to be included")
	}
	if agents[0].ModelOption.CurrentValue != "gpt-5.4" {
		t.Fatalf("unexpected cached model current value: %q", agents[0].ModelOption.CurrentValue)
	}
	if agents[0].Modes[1].ID != "kiro_planning" {
		t.Fatalf("unexpected second mode id: %q", agents[0].Modes[1].ID)
	}
	if agents[0].ModesDiscoveredAt == "" {
		t.Fatal("expected modes discovery timestamp to be set")
	}
}

func TestSettingsServiceListAgentsSkipsStaleCachedModes(t *testing.T) {
	t.Setenv("CHISEL_HOME", t.TempDir())

	service := NewSettingsService()
	original := ChiselSettings{
		Agents: map[string]AgentConfig{
			"Kiro CLI": {
				Command: "kiro-cli",
				Args:    []string{"acp"},
				Env:     map[string]string{},
			},
		},
	}

	if err := service.SaveSettings(original); err != nil {
		t.Fatalf("save original settings: %v", err)
	}
	if err := service.persistAgentCatalog("Kiro CLI", original.Agents["Kiro CLI"], "kiro_default", []SessionMode{
		{ID: "kiro_default", Name: "kiro_default", Description: "Default mode"},
	}, &SessionConfigOption{
		ID:           "model",
		Name:         "Model",
		Category:     "model",
		Type:         "select",
		CurrentValue: "gpt-5.4",
	}); err != nil {
		t.Fatalf("persist agent catalog: %v", err)
	}

	updated := ChiselSettings{
		Agents: map[string]AgentConfig{
			"Kiro CLI": {
				Command: "kiro-cli",
				Args:    []string{"acp", "--experimental-modes"},
				Env:     map[string]string{},
			},
		},
	}
	if err := service.SaveSettings(updated); err != nil {
		t.Fatalf("save updated settings: %v", err)
	}

	agents, err := service.ListAgents()
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("unexpected agent count: %d", len(agents))
	}
	if agents[0].CurrentModeID != "" {
		t.Fatalf("expected stale cached current mode to be ignored, got %q", agents[0].CurrentModeID)
	}
	if len(agents[0].Modes) != 0 {
		t.Fatalf("expected stale cached modes to be ignored, got %d modes", len(agents[0].Modes))
	}
	if agents[0].ModelOption != nil {
		t.Fatalf("expected stale cached model option to be ignored, got %+v", agents[0].ModelOption)
	}
	if agents[0].ModesDiscoveredAt != "" {
		t.Fatalf("expected stale discovery timestamp to be ignored, got %q", agents[0].ModesDiscoveredAt)
	}
}
