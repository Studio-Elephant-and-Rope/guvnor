package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
)

func TestLoadSchedulesFromYAML_BasicSchedule(t *testing.T) {
	yamlData := `
schedules:
  platform-team:
    timezone: Europe/London
    rotation:
      type: weekly
      participants:
        - alice@example.com
        - bob@example.com
      start_date: "2024-01-01T09:00:00Z"
`

	schedules, err := LoadSchedulesFromYAML([]byte(yamlData))
	if err != nil {
		t.Fatalf("LoadSchedulesFromYAML() error = %v", err)
	}

	if len(schedules) != 1 {
		t.Fatalf("Expected 1 schedule, got %d", len(schedules))
	}

	schedule := schedules[0]
	if schedule.ID != "platform-team" {
		t.Errorf("Expected ID 'platform-team', got '%s'", schedule.ID)
	}
	if schedule.Name != "platform-team" {
		t.Errorf("Expected name 'platform-team', got '%s'", schedule.Name)
	}
	if schedule.TeamID != "platform-team" {
		t.Errorf("Expected teamID 'platform-team', got '%s'", schedule.TeamID)
	}
	if schedule.Timezone != "Europe/London" {
		t.Errorf("Expected timezone 'Europe/London', got '%s'", schedule.Timezone)
	}
	if schedule.Rotation.Type != domain.RotationTypeWeekly {
		t.Errorf("Expected rotation type weekly, got %s", schedule.Rotation.Type)
	}
	if len(schedule.Rotation.Participants) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(schedule.Rotation.Participants))
	}
	if schedule.Rotation.Participants[0] != "alice@example.com" {
		t.Errorf("Expected first participant 'alice@example.com', got '%s'", schedule.Rotation.Participants[0])
	}
	if schedule.Rotation.Participants[1] != "bob@example.com" {
		t.Errorf("Expected second participant 'bob@example.com', got '%s'", schedule.Rotation.Participants[1])
	}
	if !schedule.Enabled {
		t.Error("Expected schedule to be enabled by default")
	}

	expectedStartDate := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	if !schedule.Rotation.StartDate.Equal(expectedStartDate) {
		t.Errorf("Expected start date %v, got %v", expectedStartDate, schedule.Rotation.StartDate)
	}
}

func TestLoadSchedulesFromYAML_MultipleSchedules(t *testing.T) {
	yamlData := `
schedules:
  backend-team:
    name: "Backend Team On-Call"
    team_id: "backend"
    timezone: America/New_York
    rotation:
      type: daily
      participants:
        - dev1@company.com
        - dev2@company.com
        - dev3@company.com
  frontend-team:
    timezone: UTC
    rotation:
      type: weekly
      participants:
        - ui1@company.com
        - ui2@company.com
    enabled: false
`

	schedules, err := LoadSchedulesFromYAML([]byte(yamlData))
	if err != nil {
		t.Fatalf("LoadSchedulesFromYAML() error = %v", err)
	}

	if len(schedules) != 2 {
		t.Fatalf("Expected 2 schedules, got %d", len(schedules))
	}

	// Find schedules by ID
	var backendSchedule, frontendSchedule *domain.Schedule
	for _, schedule := range schedules {
		switch schedule.ID {
		case "backend-team":
			backendSchedule = schedule
		case "frontend-team":
			frontendSchedule = schedule
		}
	}

	if backendSchedule == nil {
		t.Fatal("Backend schedule not found")
	}
	if frontendSchedule == nil {
		t.Fatal("Frontend schedule not found")
	}

	// Test backend schedule
	if backendSchedule.Name != "Backend Team On-Call" {
		t.Errorf("Expected backend name 'Backend Team On-Call', got '%s'", backendSchedule.Name)
	}
	if backendSchedule.TeamID != "backend" {
		t.Errorf("Expected backend teamID 'backend', got '%s'", backendSchedule.TeamID)
	}
	if backendSchedule.Timezone != "America/New_York" {
		t.Errorf("Expected backend timezone 'America/New_York', got '%s'", backendSchedule.Timezone)
	}
	if backendSchedule.Rotation.Type != domain.RotationTypeDaily {
		t.Errorf("Expected backend rotation type daily, got %s", backendSchedule.Rotation.Type)
	}
	if len(backendSchedule.Rotation.Participants) != 3 {
		t.Errorf("Expected 3 backend participants, got %d", len(backendSchedule.Rotation.Participants))
	}
	if !backendSchedule.Enabled {
		t.Error("Expected backend schedule to be enabled by default")
	}

	// Test frontend schedule
	if frontendSchedule.Name != "frontend-team" {
		t.Errorf("Expected frontend name 'frontend-team', got '%s'", frontendSchedule.Name)
	}
	if frontendSchedule.TeamID != "frontend-team" {
		t.Errorf("Expected frontend teamID 'frontend-team', got '%s'", frontendSchedule.TeamID)
	}
	if frontendSchedule.Timezone != "UTC" {
		t.Errorf("Expected frontend timezone 'UTC', got '%s'", frontendSchedule.Timezone)
	}
	if frontendSchedule.Rotation.Type != domain.RotationTypeWeekly {
		t.Errorf("Expected frontend rotation type weekly, got %s", frontendSchedule.Rotation.Type)
	}
	if frontendSchedule.Enabled {
		t.Error("Expected frontend schedule to be disabled")
	}
}

func TestLoadSchedulesFromYAML_CustomRotation(t *testing.T) {
	yamlData := `
schedules:
  sre-team:
    timezone: UTC
    rotation:
      type: custom
      participants:
        - sre1@company.com
        - sre2@company.com
      start_date: "2024-01-01T00:00:00Z"
      duration: "72h"
`

	schedules, err := LoadSchedulesFromYAML([]byte(yamlData))
	if err != nil {
		t.Fatalf("LoadSchedulesFromYAML() error = %v", err)
	}

	if len(schedules) != 1 {
		t.Fatalf("Expected 1 schedule, got %d", len(schedules))
	}

	schedule := schedules[0]
	if schedule.Rotation.Type != domain.RotationTypeCustom {
		t.Errorf("Expected rotation type custom, got %s", schedule.Rotation.Type)
	}
	if schedule.Rotation.Duration != 72*time.Hour {
		t.Errorf("Expected duration 72h, got %v", schedule.Rotation.Duration)
	}
}

func TestLoadSchedulesFromYAML_DateFormats(t *testing.T) {
	tests := []struct {
		name      string
		startDate string
		wantYear  int
		wantMonth time.Month
		wantDay   int
	}{
		{
			name:      "RFC3339 format",
			startDate: "2024-01-15T10:30:00Z",
			wantYear:  2024,
			wantMonth: time.January,
			wantDay:   15,
		},
		{
			name:      "ISO 8601 format",
			startDate: "2024-01-15T10:30:00",
			wantYear:  2024,
			wantMonth: time.January,
			wantDay:   15,
		},
		{
			name:      "Date only format",
			startDate: "2024-01-15",
			wantYear:  2024,
			wantMonth: time.January,
			wantDay:   15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlData := `
schedules:
  test-team:
    timezone: UTC
    rotation:
      type: weekly
      participants:
        - test@example.com
      start_date: "` + tt.startDate + `"
`

			schedules, err := LoadSchedulesFromYAML([]byte(yamlData))
			if err != nil {
				t.Fatalf("LoadSchedulesFromYAML() error = %v", err)
			}

			schedule := schedules[0]
			startDate := schedule.Rotation.StartDate
			if startDate.Year() != tt.wantYear {
				t.Errorf("Expected year %d, got %d", tt.wantYear, startDate.Year())
			}
			if startDate.Month() != tt.wantMonth {
				t.Errorf("Expected month %s, got %s", tt.wantMonth, startDate.Month())
			}
			if startDate.Day() != tt.wantDay {
				t.Errorf("Expected day %d, got %d", tt.wantDay, startDate.Day())
			}
		})
	}
}

func TestLoadSchedulesFromYAML_ErrorCases(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
		wantErr  string
	}{
		{
			name:     "invalid YAML",
			yamlData: `invalid: yaml: [unclosed`,
			wantErr:  "failed to parse YAML",
		},
		{
			name: "missing timezone",
			yamlData: `
schedules:
  test-team:
    rotation:
      type: weekly
      participants:
        - test@example.com
`,
			wantErr: "timezone is required",
		},
		{
			name: "missing rotation type",
			yamlData: `
schedules:
  test-team:
    timezone: UTC
    rotation:
      participants:
        - test@example.com
`,
			wantErr: "rotation type is required",
		},
		{
			name: "empty participants",
			yamlData: `
schedules:
  test-team:
    timezone: UTC
    rotation:
      type: weekly
      participants: []
`,
			wantErr: "rotation must have at least one participant",
		},
		{
			name: "invalid rotation type",
			yamlData: `
schedules:
  test-team:
    timezone: UTC
    rotation:
      type: invalid
      participants:
        - test@example.com
`,
			wantErr: "invalid rotation type: invalid",
		},
		{
			name: "invalid timezone",
			yamlData: `
schedules:
  test-team:
    timezone: Invalid/Timezone
    rotation:
      type: weekly
      participants:
        - test@example.com
`,
			wantErr: "invalid timezone Invalid/Timezone",
		},
		{
			name: "invalid start date",
			yamlData: `
schedules:
  test-team:
    timezone: UTC
    rotation:
      type: weekly
      participants:
        - test@example.com
      start_date: "invalid-date"
`,
			wantErr: "invalid start date format",
		},
		{
			name: "custom rotation without duration",
			yamlData: `
schedules:
  test-team:
    timezone: UTC
    rotation:
      type: custom
      participants:
        - test@example.com
`,
			wantErr: "custom rotation type requires duration to be specified",
		},
		{
			name: "invalid duration format",
			yamlData: `
schedules:
  test-team:
    timezone: UTC
    rotation:
      type: custom
      participants:
        - test@example.com
      duration: "invalid"
`,
			wantErr: "invalid duration format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadSchedulesFromYAML([]byte(tt.yamlData))
			if err == nil {
				t.Fatal("Expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.wantErr, err.Error())
			}
		})
	}
}

func TestLoadSchedulesFromFile(t *testing.T) {
	// Create temporary file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "schedules.yaml")

	yamlContent := `
schedules:
  test-team:
    timezone: UTC
    rotation:
      type: weekly
      participants:
        - test1@example.com
        - test2@example.com
`

	err := os.WriteFile(configFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	schedules, err := LoadSchedulesFromFile(configFile)
	if err != nil {
		t.Fatalf("LoadSchedulesFromFile() error = %v", err)
	}

	if len(schedules) != 1 {
		t.Fatalf("Expected 1 schedule, got %d", len(schedules))
	}

	schedule := schedules[0]
	if schedule.ID != "test-team" {
		t.Errorf("Expected ID 'test-team', got '%s'", schedule.ID)
	}
}

func TestLoadSchedulesFromFile_NonExistentFile(t *testing.T) {
	_, err := LoadSchedulesFromFile("/nonexistent/file.yaml")
	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to read schedule config file") {
		t.Errorf("Expected file read error, got: %v", err)
	}
}

func TestWriteSchedulesToYAML(t *testing.T) {
	// Create test schedules
	schedules := []*domain.Schedule{
		{
			ID:       "team-1",
			Name:     "Team 1 Schedule",
			TeamID:   "team-1",
			Timezone: "America/New_York",
			Rotation: domain.Rotation{
				Type:         domain.RotationTypeWeekly,
				Participants: []string{"user1@example.com", "user2@example.com"},
				StartDate:    time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC),
			},
			Enabled: true,
		},
		{
			ID:       "team-2",
			Name:     "team-2", // Same as ID, should be omitted
			TeamID:   "team-2", // Same as ID, should be omitted
			Timezone: "UTC",
			Rotation: domain.Rotation{
				Type:         domain.RotationTypeCustom,
				Participants: []string{"user3@example.com"},
				StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Duration:     48 * time.Hour,
			},
			Enabled: false,
		},
	}

	yamlData, err := WriteSchedulesToYAML(schedules)
	if err != nil {
		t.Fatalf("WriteSchedulesToYAML() error = %v", err)
	}

	// Parse it back to verify round-trip
	parsedSchedules, err := LoadSchedulesFromYAML(yamlData)
	if err != nil {
		t.Fatalf("Failed to parse generated YAML: %v", err)
	}

	if len(parsedSchedules) != len(schedules) {
		t.Fatalf("Expected %d schedules, got %d", len(schedules), len(parsedSchedules))
	}

	// Find schedules by ID for comparison
	scheduleMap := make(map[string]*domain.Schedule)
	for _, schedule := range parsedSchedules {
		scheduleMap[schedule.ID] = schedule
	}

	// Verify team-1
	team1 := scheduleMap["team-1"]
	if team1 == nil {
		t.Fatal("team-1 schedule not found")
	}
	if team1.Name != "Team 1 Schedule" {
		t.Errorf("Expected name 'Team 1 Schedule', got '%s'", team1.Name)
	}
	if team1.Timezone != "America/New_York" {
		t.Errorf("Expected timezone 'America/New_York', got '%s'", team1.Timezone)
	}
	if team1.Rotation.Type != domain.RotationTypeWeekly {
		t.Errorf("Expected weekly rotation, got %s", team1.Rotation.Type)
	}
	if !team1.Enabled {
		t.Error("Expected team-1 to be enabled")
	}

	// Verify team-2
	team2 := scheduleMap["team-2"]
	if team2 == nil {
		t.Fatal("team-2 schedule not found")
	}
	if team2.Name != "team-2" {
		t.Errorf("Expected name 'team-2', got '%s'", team2.Name)
	}
	if team2.Rotation.Type != domain.RotationTypeCustom {
		t.Errorf("Expected custom rotation, got %s", team2.Rotation.Type)
	}
	if team2.Rotation.Duration != 48*time.Hour {
		t.Errorf("Expected 48h duration, got %v", team2.Rotation.Duration)
	}
	if team2.Enabled {
		t.Error("Expected team-2 to be disabled")
	}
}

func TestWriteSchedulesToFile(t *testing.T) {
	// Create test schedule
	schedule := &domain.Schedule{
		ID:       "test-team",
		Name:     "Test Team",
		TeamID:   "test",
		Timezone: "UTC",
		Rotation: domain.Rotation{
			Type:         domain.RotationTypeDaily,
			Participants: []string{"test@example.com"},
			StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Enabled: true,
	}

	// Write to temporary file
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "output.yaml")

	err := WriteSchedulesToFile(outputFile, []*domain.Schedule{schedule})
	if err != nil {
		t.Fatalf("WriteSchedulesToFile() error = %v", err)
	}

	// Verify file exists and can be read back
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatal("Output file was not created")
	}

	// Read and parse the file
	schedules, err := LoadSchedulesFromFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read back written file: %v", err)
	}

	if len(schedules) != 1 {
		t.Fatalf("Expected 1 schedule, got %d", len(schedules))
	}

	readSchedule := schedules[0]
	if readSchedule.ID != schedule.ID {
		t.Errorf("Expected ID '%s', got '%s'", schedule.ID, readSchedule.ID)
	}
	if readSchedule.Name != schedule.Name {
		t.Errorf("Expected name '%s', got '%s'", schedule.Name, readSchedule.Name)
	}
}

func TestDefaultValues(t *testing.T) {
	yamlData := `
schedules:
  minimal-schedule:
    timezone: UTC
    rotation:
      type: weekly
      participants:
        - test@example.com
`

	schedules, err := LoadSchedulesFromYAML([]byte(yamlData))
	if err != nil {
		t.Fatalf("LoadSchedulesFromYAML() error = %v", err)
	}

	schedule := schedules[0]

	// Check defaults
	if schedule.Name != "minimal-schedule" {
		t.Errorf("Expected name to default to ID 'minimal-schedule', got '%s'", schedule.Name)
	}
	if schedule.TeamID != "minimal-schedule" {
		t.Errorf("Expected teamID to default to ID 'minimal-schedule', got '%s'", schedule.TeamID)
	}
	if !schedule.Enabled {
		t.Error("Expected schedule to be enabled by default")
	}
	if schedule.Rotation.StartDate.IsZero() {
		t.Error("Expected start date to be set to current time by default")
	}
}

func TestValidationIntegration(t *testing.T) {
	// Test that domain validation is applied during YAML loading
	yamlData := `
schedules:
  invalid-schedule:
    timezone: UTC
    rotation:
      type: weekly
      participants:
        - ""  # Empty participant should fail validation
`

	_, err := LoadSchedulesFromYAML([]byte(yamlData))
	if err == nil {
		t.Fatal("Expected validation error for empty participant")
	}
	if !strings.Contains(err.Error(), "schedule validation failed") {
		t.Errorf("Expected schedule validation error, got: %v", err)
	}
}
