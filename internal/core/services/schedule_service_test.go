package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// MockScheduleRepository is a mock implementation of ports.ScheduleRepository for testing.
type MockScheduleRepository struct {
	schedules map[string]*domain.Schedule
	createErr error
	getErr    error
	listErr   error
	updateErr error
	deleteErr error
	mu        sync.RWMutex
}

// NewMockScheduleRepository creates a new mock schedule repository.
func NewMockScheduleRepository() *MockScheduleRepository {
	return &MockScheduleRepository{
		schedules: make(map[string]*domain.Schedule),
	}
}

// SetCreateError sets the error to return from Create calls.
func (m *MockScheduleRepository) SetCreateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createErr = err
}

// SetGetError sets the error to return from Get calls.
func (m *MockScheduleRepository) SetGetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getErr = err
}

// SetListError sets the error to return from List calls.
func (m *MockScheduleRepository) SetListError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listErr = err
}

// SetUpdateError sets the error to return from Update calls.
func (m *MockScheduleRepository) SetUpdateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateErr = err
}

// SetDeleteError sets the error to return from Delete calls.
func (m *MockScheduleRepository) SetDeleteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteErr = err
}

// Create implements ports.ScheduleRepository.
func (m *MockScheduleRepository) Create(ctx context.Context, schedule *domain.Schedule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return m.createErr
	}

	if _, exists := m.schedules[schedule.ID]; exists {
		return ports.ErrAlreadyExists
	}

	// Create a copy to avoid external modifications
	copied := *schedule
	m.schedules[schedule.ID] = &copied
	return nil
}

// Get implements ports.ScheduleRepository.
func (m *MockScheduleRepository) Get(ctx context.Context, id string) (*domain.Schedule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	schedule, exists := m.schedules[id]
	if !exists {
		return nil, ports.ErrNotFound
	}

	// Create a copy to avoid external modifications
	copied := *schedule
	copied.Rotation.Participants = make([]string, len(schedule.Rotation.Participants))
	copy(copied.Rotation.Participants, schedule.Rotation.Participants)
	return &copied, nil
}

// GetByTeamID implements ports.ScheduleRepository.
func (m *MockScheduleRepository) GetByTeamID(ctx context.Context, teamID string) ([]*domain.Schedule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	result := make([]*domain.Schedule, 0)
	for _, schedule := range m.schedules {
		if schedule.TeamID == teamID {
			copied := *schedule
			copied.Rotation.Participants = make([]string, len(schedule.Rotation.Participants))
			copy(copied.Rotation.Participants, schedule.Rotation.Participants)
			result = append(result, &copied)
		}
	}

	return result, nil
}

// List implements ports.ScheduleRepository.
func (m *MockScheduleRepository) List(ctx context.Context, enabledOnly bool) ([]*domain.Schedule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.listErr != nil {
		return nil, m.listErr
	}

	result := make([]*domain.Schedule, 0)
	for _, schedule := range m.schedules {
		if !enabledOnly || schedule.Enabled {
			copied := *schedule
			copied.Rotation.Participants = make([]string, len(schedule.Rotation.Participants))
			copy(copied.Rotation.Participants, schedule.Rotation.Participants)
			result = append(result, &copied)
		}
	}

	return result, nil
}

// Update implements ports.ScheduleRepository.
func (m *MockScheduleRepository) Update(ctx context.Context, schedule *domain.Schedule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.updateErr != nil {
		return m.updateErr
	}

	if _, exists := m.schedules[schedule.ID]; !exists {
		return ports.ErrNotFound
	}

	// Create a copy to avoid external modifications
	copied := *schedule
	m.schedules[schedule.ID] = &copied
	return nil
}

// Delete implements ports.ScheduleRepository.
func (m *MockScheduleRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteErr != nil {
		return m.deleteErr
	}

	if _, exists := m.schedules[id]; !exists {
		return ports.ErrNotFound
	}

	delete(m.schedules, id)
	return nil
}

// AddSchedule is a helper method for setting up test data.
func (m *MockScheduleRepository) AddSchedule(schedule *domain.Schedule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *schedule
	m.schedules[schedule.ID] = &copied
}

// createScheduleTestLogger creates a logger specifically for schedule service tests.
func createScheduleTestLogger() *logging.Logger {
	config := logging.Config{
		Environment: logging.Test,
		Level:       -8, // Debug level for comprehensive testing
		AddSource:   false,
	}
	logger, _ := logging.NewLogger(config)
	return logger
}

// createTestScheduleService creates a service with mocked dependencies for testing.
func createTestScheduleService() (*ScheduleService, *MockScheduleRepository) {
	repo := NewMockScheduleRepository()
	logger := createScheduleTestLogger()
	service, _ := NewScheduleService(repo, logger)
	return service, repo
}

// createTestSchedule creates a valid test schedule.
func createTestSchedule(id, teamID string) *domain.Schedule {
	return &domain.Schedule{
		ID:       id,
		Name:     "Test Schedule " + id,
		TeamID:   teamID,
		Timezone: "UTC",
		Rotation: domain.Rotation{
			Type:         domain.RotationTypeWeekly,
			Participants: []string{"user1@example.com", "user2@example.com"},
			StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Enabled:   true,
	}
}

func TestNewScheduleService(t *testing.T) {
	tests := []struct {
		name        string
		repo        ports.ScheduleRepository
		logger      *logging.Logger
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid dependencies",
			repo:        NewMockScheduleRepository(),
			logger:      createScheduleTestLogger(),
			expectError: false,
		},
		{
			name:        "nil repository",
			repo:        nil,
			logger:      createScheduleTestLogger(),
			expectError: true,
			errorMsg:    "schedule repository is required",
		},
		{
			name:        "nil logger",
			repo:        NewMockScheduleRepository(),
			logger:      nil,
			expectError: true,
			errorMsg:    "logger is required",
		},
		{
			name:        "both nil",
			repo:        nil,
			logger:      nil,
			expectError: true,
			errorMsg:    "schedule repository is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewScheduleService(tt.repo, tt.logger)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if service != nil {
					t.Errorf("Expected nil service on error, got %v", service)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if service == nil {
					t.Errorf("Expected valid service but got nil")
				}
			}
		})
	}
}

func TestScheduleService_CreateSchedule(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	tests := []struct {
		name        string
		schedule    *domain.Schedule
		setupRepo   func(*MockScheduleRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid schedule",
			schedule:    createTestSchedule("valid-schedule", "team-1"),
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: false,
		},
		{
			name:        "nil schedule",
			schedule:    nil,
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule is required",
		},
		{
			name: "invalid schedule - empty ID",
			schedule: &domain.Schedule{
				Name:     "Test Schedule",
				TeamID:   "team-1",
				Timezone: "UTC",
				Rotation: domain.Rotation{
					Type:         domain.RotationTypeWeekly,
					Participants: []string{"user1@example.com"},
					StartDate:    time.Now(),
				},
				Enabled: true,
			},
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule validation failed",
		},
		{
			name:     "repository error",
			schedule: createTestSchedule("repo-error", "team-1"),
			setupRepo: func(r *MockScheduleRepository) {
				r.SetCreateError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to create schedule",
		},
		{
			name:     "already exists",
			schedule: createTestSchedule("existing-schedule", "team-1"),
			setupRepo: func(r *MockScheduleRepository) {
				r.AddSchedule(createTestSchedule("existing-schedule", "team-1"))
			},
			expectError: true,
			errorMsg:    "failed to create schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset repository
			repo = NewMockScheduleRepository()
			service, _ = NewScheduleService(repo, createScheduleTestLogger())
			tt.setupRepo(repo)

			// Test create schedule
			result, err := service.CreateSchedule(ctx, tt.schedule)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if result != nil {
					t.Errorf("Expected nil result on error, got %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if result == nil {
					t.Errorf("Expected valid result but got nil")
				}
				if result != nil {
					// Verify timestamps were set
					if result.CreatedAt.IsZero() {
						t.Errorf("CreatedAt timestamp was not set")
					}
					if result.UpdatedAt.IsZero() {
						t.Errorf("UpdatedAt timestamp was not set")
					}
					if !result.CreatedAt.Equal(result.UpdatedAt) {
						t.Errorf("CreatedAt and UpdatedAt should be equal for new schedule")
					}
				}
			}
		})
	}
}

func TestScheduleService_GetSchedule(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	// Setup test data
	testSchedule := createTestSchedule("test-schedule", "team-1")
	repo.AddSchedule(testSchedule)

	tests := []struct {
		name        string
		scheduleID  string
		setupRepo   func(*MockScheduleRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:        "existing schedule",
			scheduleID:  "test-schedule",
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: false,
		},
		{
			name:        "empty schedule ID",
			scheduleID:  "",
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule ID is required",
		},
		{
			name:        "non-existent schedule",
			scheduleID:  "non-existent",
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "failed to retrieve schedule",
		},
		{
			name:       "repository error",
			scheduleID: "test-schedule",
			setupRepo: func(r *MockScheduleRepository) {
				r.SetGetError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to retrieve schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRepo(repo)

			result, err := service.GetSchedule(ctx, tt.scheduleID)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if result != nil {
					t.Errorf("Expected nil result on error, got %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if result == nil {
					t.Errorf("Expected valid result but got nil")
				}
				if result != nil && result.ID != tt.scheduleID {
					t.Errorf("Expected schedule ID %s, got %s", tt.scheduleID, result.ID)
				}
			}

			// Reset repo error state
			repo.SetGetError(nil)
		})
	}
}

func TestScheduleService_GetSchedulesByTeam(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	// Setup test data
	repo.AddSchedule(createTestSchedule("team1-schedule1", "team-1"))
	repo.AddSchedule(createTestSchedule("team1-schedule2", "team-1"))
	repo.AddSchedule(createTestSchedule("team2-schedule1", "team-2"))

	tests := []struct {
		name          string
		teamID        string
		setupRepo     func(*MockScheduleRepository)
		expectError   bool
		errorMsg      string
		expectedCount int
	}{
		{
			name:          "team with schedules",
			teamID:        "team-1",
			setupRepo:     func(r *MockScheduleRepository) {},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name:          "team with one schedule",
			teamID:        "team-2",
			setupRepo:     func(r *MockScheduleRepository) {},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name:          "team with no schedules",
			teamID:        "team-3",
			setupRepo:     func(r *MockScheduleRepository) {},
			expectError:   false,
			expectedCount: 0,
		},
		{
			name:        "empty team ID",
			teamID:      "",
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "team ID is required",
		},
		{
			name:   "repository error",
			teamID: "team-1",
			setupRepo: func(r *MockScheduleRepository) {
				r.SetGetError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to retrieve schedules for team",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRepo(repo)

			result, err := service.GetSchedulesByTeam(ctx, tt.teamID)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if result != nil {
					t.Errorf("Expected nil result on error, got %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if result == nil {
					t.Errorf("Expected valid result but got nil")
				}
				if len(result) != tt.expectedCount {
					t.Errorf("Expected %d schedules, got %d", tt.expectedCount, len(result))
				}

				// Verify all schedules belong to the correct team
				if result != nil {
					for _, schedule := range result {
						if schedule.TeamID != tt.teamID {
							t.Errorf("Expected schedule to belong to team %s, got %s", tt.teamID, schedule.TeamID)
						}
					}
				}
			}

			// Reset repo error state
			repo.SetGetError(nil)
		})
	}
}

func TestScheduleService_ListSchedules(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	// Setup test data
	enabledSchedule := createTestSchedule("enabled-schedule", "team-1")
	enabledSchedule.Enabled = true
	disabledSchedule := createTestSchedule("disabled-schedule", "team-2")
	disabledSchedule.Enabled = false

	repo.AddSchedule(enabledSchedule)
	repo.AddSchedule(disabledSchedule)

	tests := []struct {
		name          string
		enabledOnly   bool
		setupRepo     func(*MockScheduleRepository)
		expectError   bool
		errorMsg      string
		expectedCount int
	}{
		{
			name:          "all schedules",
			enabledOnly:   false,
			setupRepo:     func(r *MockScheduleRepository) {},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name:          "enabled schedules only",
			enabledOnly:   true,
			setupRepo:     func(r *MockScheduleRepository) {},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name:        "repository error",
			enabledOnly: false,
			setupRepo: func(r *MockScheduleRepository) {
				r.SetListError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to list schedules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRepo(repo)

			result, err := service.ListSchedules(ctx, tt.enabledOnly)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if result != nil {
					t.Errorf("Expected nil result on error, got %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if result == nil {
					t.Errorf("Expected valid result but got nil")
				}
				if len(result) != tt.expectedCount {
					t.Errorf("Expected %d schedules, got %d", tt.expectedCount, len(result))
				}

				// Verify enabled filter is applied correctly
				if tt.enabledOnly && result != nil {
					for _, schedule := range result {
						if !schedule.Enabled {
							t.Errorf("Expected only enabled schedules when enabledOnly=true, found disabled schedule %s", schedule.ID)
						}
					}
				}
			}

			// Reset repo error state
			repo.SetListError(nil)
		})
	}
}

func TestScheduleService_UpdateSchedule(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	// Setup test data
	existingSchedule := createTestSchedule("existing-schedule", "team-1")
	repo.AddSchedule(existingSchedule)

	tests := []struct {
		name        string
		schedule    *domain.Schedule
		setupRepo   func(*MockScheduleRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid update",
			schedule: &domain.Schedule{
				ID:       "existing-schedule",
				Name:     "Updated Schedule",
				TeamID:   "team-1",
				Timezone: "America/New_York",
				Rotation: domain.Rotation{
					Type:         domain.RotationTypeDaily,
					Participants: []string{"user1@example.com", "user2@example.com", "user3@example.com"},
					StartDate:    time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				},
				CreatedAt: existingSchedule.CreatedAt,
				UpdatedAt: existingSchedule.UpdatedAt,
				Enabled:   false,
			},
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: false,
		},
		{
			name:        "nil schedule",
			schedule:    nil,
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule is required",
		},
		{
			name: "invalid schedule",
			schedule: &domain.Schedule{
				ID:       "existing-schedule",
				Name:     "", // Invalid: empty name
				TeamID:   "team-1",
				Timezone: "UTC",
				Rotation: domain.Rotation{
					Type:         domain.RotationTypeWeekly,
					Participants: []string{"user1@example.com"},
					StartDate:    time.Now(),
				},
				Enabled: true,
			},
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule validation failed",
		},
		{
			name: "repository error",
			schedule: &domain.Schedule{
				ID:       "existing-schedule",
				Name:     "Updated Schedule",
				TeamID:   "team-1",
				Timezone: "UTC",
				Rotation: domain.Rotation{
					Type:         domain.RotationTypeWeekly,
					Participants: []string{"user1@example.com"},
					StartDate:    time.Now(),
				},
				Enabled: true,
			},
			setupRepo: func(r *MockScheduleRepository) {
				r.SetUpdateError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to update schedule",
		},
		{
			name: "non-existent schedule",
			schedule: &domain.Schedule{
				ID:       "non-existent",
				Name:     "Updated Schedule",
				TeamID:   "team-1",
				Timezone: "UTC",
				Rotation: domain.Rotation{
					Type:         domain.RotationTypeWeekly,
					Participants: []string{"user1@example.com"},
					StartDate:    time.Now(),
				},
				Enabled: true,
			},
			setupRepo: func(r *MockScheduleRepository) {
				r.SetUpdateError(ports.ErrNotFound)
			},
			expectError: true,
			errorMsg:    "failed to update schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRepo(repo)

			originalUpdatedAt := time.Time{}
			if tt.schedule != nil {
				originalUpdatedAt = tt.schedule.UpdatedAt
			}

			result, err := service.UpdateSchedule(ctx, tt.schedule)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if result != nil {
					t.Errorf("Expected nil result on error, got %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if result == nil {
					t.Errorf("Expected valid result but got nil")
				}
				if result != nil {
					// Verify UpdatedAt timestamp was updated
					if !result.UpdatedAt.After(originalUpdatedAt) {
						t.Errorf("UpdatedAt timestamp should be updated")
					}
				}
			}

			// Reset repo error state
			repo.SetUpdateError(nil)
		})
	}
}

func TestScheduleService_DeleteSchedule(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	// Setup test data
	existingSchedule := createTestSchedule("existing-schedule", "team-1")
	repo.AddSchedule(existingSchedule)

	tests := []struct {
		name        string
		scheduleID  string
		setupRepo   func(*MockScheduleRepository)
		expectError bool
		errorMsg    string
	}{
		{
			name:        "existing schedule",
			scheduleID:  "existing-schedule",
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: false,
		},
		{
			name:        "empty schedule ID",
			scheduleID:  "",
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule ID is required",
		},
		{
			name:        "non-existent schedule",
			scheduleID:  "non-existent",
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "failed to delete schedule",
		},
		{
			name:       "repository error",
			scheduleID: "existing-schedule",
			setupRepo: func(r *MockScheduleRepository) {
				r.SetDeleteError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to delete schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-add the schedule for each test
			repo.AddSchedule(existingSchedule)
			tt.setupRepo(repo)

			err := service.DeleteSchedule(ctx, tt.scheduleID)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}

			// Reset repo error state
			repo.SetDeleteError(nil)
		})
	}
}

func TestScheduleService_GetCurrentOnCall(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	// Setup test data with a known schedule
	testSchedule := &domain.Schedule{
		ID:       "test-schedule",
		Name:     "Test Schedule",
		TeamID:   "team-1",
		Timezone: "UTC",
		Rotation: domain.Rotation{
			Type:         domain.RotationTypeWeekly,
			Participants: []string{"user1@example.com", "user2@example.com"},
			StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), // Monday
		},
		CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Enabled:   true,
	}
	repo.AddSchedule(testSchedule)

	tests := []struct {
		name         string
		scheduleID   string
		atTime       time.Time
		setupRepo    func(*MockScheduleRepository)
		expectError  bool
		errorMsg     string
		expectedUser string
	}{
		{
			name:         "valid schedule and time - first week",
			scheduleID:   "test-schedule",
			atTime:       time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC), // Wednesday of first week
			setupRepo:    func(r *MockScheduleRepository) {},
			expectError:  false,
			expectedUser: "user1@example.com",
		},
		{
			name:         "valid schedule and time - second week",
			scheduleID:   "test-schedule",
			atTime:       time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC), // Wednesday of second week
			setupRepo:    func(r *MockScheduleRepository) {},
			expectError:  false,
			expectedUser: "user2@example.com",
		},
		{
			name:        "empty schedule ID",
			scheduleID:  "",
			atTime:      time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule ID is required",
		},
		{
			name:        "non-existent schedule",
			scheduleID:  "non-existent",
			atTime:      time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "failed to retrieve schedule",
		},
		{
			name:       "repository error",
			scheduleID: "test-schedule",
			atTime:     time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
			setupRepo: func(r *MockScheduleRepository) {
				r.SetGetError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to retrieve schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRepo(repo)

			result, err := service.GetCurrentOnCall(ctx, tt.scheduleID, tt.atTime)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if result != "" {
					t.Errorf("Expected empty result on error, got %s", result)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if result != tt.expectedUser {
					t.Errorf("Expected user %s, got %s", tt.expectedUser, result)
				}
			}

			// Reset repo error state
			repo.SetGetError(nil)
		})
	}
}

func TestScheduleService_GetNextRotation(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	// Setup test data with a known schedule
	testSchedule := &domain.Schedule{
		ID:       "test-schedule",
		Name:     "Test Schedule",
		TeamID:   "team-1",
		Timezone: "UTC",
		Rotation: domain.Rotation{
			Type:         domain.RotationTypeWeekly,
			Participants: []string{"user1@example.com", "user2@example.com"},
			StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), // Monday
		},
		CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Enabled:   true,
	}
	repo.AddSchedule(testSchedule)

	tests := []struct {
		name             string
		scheduleID       string
		atTime           time.Time
		setupRepo        func(*MockScheduleRepository)
		expectError      bool
		errorMsg         string
		expectedNextTime time.Time
		expectedUser     string
	}{
		{
			name:             "valid schedule and time - first week",
			scheduleID:       "test-schedule",
			atTime:           time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC), // Wednesday of first week
			setupRepo:        func(r *MockScheduleRepository) {},
			expectError:      false,
			expectedNextTime: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), // Next Monday
			expectedUser:     "user2@example.com",
		},
		{
			name:        "empty schedule ID",
			scheduleID:  "",
			atTime:      time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule ID is required",
		},
		{
			name:        "non-existent schedule",
			scheduleID:  "non-existent",
			atTime:      time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "failed to retrieve schedule",
		},
		{
			name:       "repository error",
			scheduleID: "test-schedule",
			atTime:     time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC),
			setupRepo: func(r *MockScheduleRepository) {
				r.SetGetError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to retrieve schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRepo(repo)

			nextTime, nextUser, err := service.GetNextRotation(ctx, tt.scheduleID, tt.atTime)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if !nextTime.IsZero() {
					t.Errorf("Expected zero time on error, got %v", nextTime)
				}
				if nextUser != "" {
					t.Errorf("Expected empty user on error, got %s", nextUser)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if !nextTime.Equal(tt.expectedNextTime) {
					t.Errorf("Expected next time %v, got %v", tt.expectedNextTime, nextTime)
				}
				if nextUser != tt.expectedUser {
					t.Errorf("Expected next user %s, got %s", tt.expectedUser, nextUser)
				}
			}

			// Reset repo error state
			repo.SetGetError(nil)
		})
	}
}

func TestScheduleService_IsScheduleActive(t *testing.T) {
	service, repo := createTestScheduleService()
	ctx := context.Background()

	// Setup test data
	activeSchedule := createTestSchedule("active-schedule", "team-1")
	activeSchedule.Enabled = true
	activeSchedule.Rotation.StartDate = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	disabledSchedule := createTestSchedule("disabled-schedule", "team-2")
	disabledSchedule.Enabled = false

	futureSchedule := createTestSchedule("future-schedule", "team-3")
	futureSchedule.Enabled = true
	futureSchedule.Rotation.StartDate = time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	repo.AddSchedule(activeSchedule)
	repo.AddSchedule(disabledSchedule)
	repo.AddSchedule(futureSchedule)

	tests := []struct {
		name           string
		scheduleID     string
		atTime         time.Time
		setupRepo      func(*MockScheduleRepository)
		expectError    bool
		errorMsg       string
		expectedActive bool
	}{
		{
			name:           "active schedule",
			scheduleID:     "active-schedule",
			atTime:         time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			setupRepo:      func(r *MockScheduleRepository) {},
			expectError:    false,
			expectedActive: true,
		},
		{
			name:           "disabled schedule",
			scheduleID:     "disabled-schedule",
			atTime:         time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			setupRepo:      func(r *MockScheduleRepository) {},
			expectError:    false,
			expectedActive: false,
		},
		{
			name:           "future schedule",
			scheduleID:     "future-schedule",
			atTime:         time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			setupRepo:      func(r *MockScheduleRepository) {},
			expectError:    false,
			expectedActive: false,
		},
		{
			name:        "empty schedule ID",
			scheduleID:  "",
			atTime:      time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "schedule ID is required",
		},
		{
			name:        "non-existent schedule",
			scheduleID:  "non-existent",
			atTime:      time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			setupRepo:   func(r *MockScheduleRepository) {},
			expectError: true,
			errorMsg:    "failed to retrieve schedule",
		},
		{
			name:       "repository error",
			scheduleID: "active-schedule",
			atTime:     time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			setupRepo: func(r *MockScheduleRepository) {
				r.SetGetError(errors.New("database error"))
			},
			expectError: true,
			errorMsg:    "failed to retrieve schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRepo(repo)

			result, err := service.IsScheduleActive(ctx, tt.scheduleID, tt.atTime)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if err != nil && !containsText(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				if result != false {
					t.Errorf("Expected false result on error, got %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if result != tt.expectedActive {
					t.Errorf("Expected active status %v, got %v", tt.expectedActive, result)
				}
			}

			// Reset repo error state
			repo.SetGetError(nil)
		})
	}
}

// Helper function to check if a string contains a substring (case-insensitive)
func containsText(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
