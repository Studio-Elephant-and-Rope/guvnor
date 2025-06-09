package domain

import (
	"testing"
	"time"
)

func TestRotationType_String(t *testing.T) {
	tests := []struct {
		name     string
		rotation RotationType
		expected string
	}{
		{
			name:     "weekly rotation",
			rotation: RotationTypeWeekly,
			expected: "weekly",
		},
		{
			name:     "daily rotation",
			rotation: RotationTypeDaily,
			expected: "daily",
		},
		{
			name:     "custom rotation",
			rotation: RotationTypeCustom,
			expected: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rotation.String()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestRotationType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		rotation RotationType
		expected bool
	}{
		{
			name:     "valid weekly",
			rotation: RotationTypeWeekly,
			expected: true,
		},
		{
			name:     "valid daily",
			rotation: RotationTypeDaily,
			expected: true,
		},
		{
			name:     "valid custom",
			rotation: RotationTypeCustom,
			expected: true,
		},
		{
			name:     "invalid rotation type",
			rotation: RotationType("invalid"),
			expected: false,
		},
		{
			name:     "empty rotation type",
			rotation: RotationType(""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rotation.IsValid()
			if result != tt.expected {
				t.Errorf("Expected %t, got %t", tt.expected, result)
			}
		})
	}
}

func TestRotation_Validate(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		rotation Rotation
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid weekly rotation",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob", "charlie"},
				StartDate:    baseTime,
			},
			wantErr: false,
		},
		{
			name: "valid daily rotation",
			rotation: Rotation{
				Type:         RotationTypeDaily,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			wantErr: false,
		},
		{
			name: "valid custom rotation",
			rotation: Rotation{
				Type:         RotationTypeCustom,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
				Duration:     48 * time.Hour,
			},
			wantErr: false,
		},
		{
			name: "invalid rotation type",
			rotation: Rotation{
				Type:         RotationType("invalid"),
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			wantErr: true,
			errMsg:  "invalid rotation type",
		},
		{
			name: "no participants",
			rotation: Rotation{
				Type:      RotationTypeWeekly,
				StartDate: baseTime,
			},
			wantErr: true,
			errMsg:  "rotation must have at least one participant",
		},
		{
			name: "empty participant ID",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "", "bob"},
				StartDate:    baseTime,
			},
			wantErr: true,
			errMsg:  "participant ID cannot be empty",
		},
		{
			name: "duplicate participants",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob", "alice"},
				StartDate:    baseTime,
			},
			wantErr: true,
			errMsg:  "duplicate participant",
		},
		{
			name: "zero start date",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob"},
			},
			wantErr: true,
			errMsg:  "start date is required",
		},
		{
			name: "custom rotation without duration",
			rotation: Rotation{
				Type:         RotationTypeCustom,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			wantErr: true,
			errMsg:  "custom rotation type requires a positive duration",
		},
		{
			name: "custom rotation with negative duration",
			rotation: Rotation{
				Type:         RotationTypeCustom,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
				Duration:     -24 * time.Hour,
			},
			wantErr: true,
			errMsg:  "custom rotation type requires a positive duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rotation.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate() error = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestRotation_GetCurrentOnCall(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // Monday

	tests := []struct {
		name     string
		rotation Rotation
		at       time.Time
		timezone string
		expected string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "weekly rotation - first week",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob", "charlie"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(3 * 24 * time.Hour), // Thursday
			timezone: "UTC",
			expected: "alice",
			wantErr:  false,
		},
		{
			name: "weekly rotation - second week",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob", "charlie"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(7 * 24 * time.Hour), // Next Monday
			timezone: "UTC",
			expected: "bob",
			wantErr:  false,
		},
		{
			name: "weekly rotation - third week",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob", "charlie"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(14 * 24 * time.Hour), // Two weeks later
			timezone: "UTC",
			expected: "charlie",
			wantErr:  false,
		},
		{
			name: "weekly rotation - cycle back to first",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob", "charlie"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(21 * 24 * time.Hour), // Three weeks later
			timezone: "UTC",
			expected: "alice",
			wantErr:  false,
		},
		{
			name: "daily rotation - first day",
			rotation: Rotation{
				Type:         RotationTypeDaily,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(12 * time.Hour), // Same day
			timezone: "UTC",
			expected: "alice",
			wantErr:  false,
		},
		{
			name: "daily rotation - second day",
			rotation: Rotation{
				Type:         RotationTypeDaily,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(24 * time.Hour), // Next day
			timezone: "UTC",
			expected: "bob",
			wantErr:  false,
		},
		{
			name: "daily rotation - cycle back",
			rotation: Rotation{
				Type:         RotationTypeDaily,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(48 * time.Hour), // Two days later
			timezone: "UTC",
			expected: "alice",
			wantErr:  false,
		},
		{
			name: "custom rotation - 48 hours",
			rotation: Rotation{
				Type:         RotationTypeCustom,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
				Duration:     48 * time.Hour,
			},
			at:       baseTime.Add(24 * time.Hour), // One day later
			timezone: "UTC",
			expected: "alice",
			wantErr:  false,
		},
		{
			name: "custom rotation - switch after 48 hours",
			rotation: Rotation{
				Type:         RotationTypeCustom,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
				Duration:     48 * time.Hour,
			},
			at:       baseTime.Add(48 * time.Hour), // Two days later
			timezone: "UTC",
			expected: "bob",
			wantErr:  false,
		},
		{
			name: "before start time",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(-24 * time.Hour), // Day before start
			timezone: "UTC",
			wantErr:  true,
			errMsg:   "schedule has not started yet",
		},
		{
			name: "invalid timezone",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(24 * time.Hour),
			timezone: "Invalid/Timezone",
			wantErr:  true,
			errMsg:   "invalid timezone",
		},
		{
			name: "timezone conversion",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob"},
				StartDate:    time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC), // 9 AM UTC
			},
			at:       time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), // 10 AM UTC = 5 AM EST
			timezone: "America/New_York",
			expected: "alice",
			wantErr:  false,
		},
		{
			name: "invalid rotation",
			rotation: Rotation{
				Type:      RotationType("invalid"),
				StartDate: baseTime,
			},
			at:       baseTime.Add(24 * time.Hour),
			timezone: "UTC",
			wantErr:  true,
			errMsg:   "invalid rotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.rotation.GetCurrentOnCall(tt.at, tt.timezone)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCurrentOnCall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("GetCurrentOnCall() error = %v, should contain %v", err.Error(), tt.errMsg)
				}
			} else {
				if result != tt.expected {
					t.Errorf("GetCurrentOnCall() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestRotation_GetNextRotation(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // Monday

	tests := []struct {
		name           string
		rotation       Rotation
		at             time.Time
		timezone       string
		expectedTime   time.Time
		expectedPerson string
		wantErr        bool
		errMsg         string
	}{
		{
			name: "weekly rotation - during first week",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob", "charlie"},
				StartDate:    baseTime,
			},
			at:             baseTime.Add(3 * 24 * time.Hour), // Thursday
			timezone:       "UTC",
			expectedTime:   baseTime.Add(7 * 24 * time.Hour), // Next Monday
			expectedPerson: "bob",
			wantErr:        false,
		},
		{
			name: "daily rotation - during first day",
			rotation: Rotation{
				Type:         RotationTypeDaily,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			at:             baseTime.Add(12 * time.Hour), // Same day
			timezone:       "UTC",
			expectedTime:   baseTime.Add(24 * time.Hour), // Next day
			expectedPerson: "bob",
			wantErr:        false,
		},
		{
			name: "before start time",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			at:             baseTime.Add(-24 * time.Hour), // Day before start
			timezone:       "UTC",
			expectedTime:   baseTime, // Start time
			expectedPerson: "alice",
			wantErr:        false,
		},
		{
			name: "custom rotation",
			rotation: Rotation{
				Type:         RotationTypeCustom,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
				Duration:     48 * time.Hour,
			},
			at:             baseTime.Add(24 * time.Hour), // One day later
			timezone:       "UTC",
			expectedTime:   baseTime.Add(48 * time.Hour), // Two days after start
			expectedPerson: "bob",
			wantErr:        false,
		},
		{
			name: "invalid timezone",
			rotation: Rotation{
				Type:         RotationTypeWeekly,
				Participants: []string{"alice", "bob"},
				StartDate:    baseTime,
			},
			at:       baseTime.Add(24 * time.Hour),
			timezone: "Invalid/Timezone",
			wantErr:  true,
			errMsg:   "invalid timezone",
		},
		{
			name: "invalid rotation",
			rotation: Rotation{
				Type:      RotationType("invalid"),
				StartDate: baseTime,
			},
			at:       baseTime.Add(24 * time.Hour),
			timezone: "UTC",
			wantErr:  true,
			errMsg:   "invalid rotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextTime, nextPerson, err := tt.rotation.GetNextRotation(tt.at, tt.timezone)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNextRotation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("GetNextRotation() error = %v, should contain %v", err.Error(), tt.errMsg)
				}
			} else {
				if !nextTime.Equal(tt.expectedTime) {
					t.Errorf("GetNextRotation() time = %v, want %v", nextTime, tt.expectedTime)
				}
				if nextPerson != tt.expectedPerson {
					t.Errorf("GetNextRotation() person = %v, want %v", nextPerson, tt.expectedPerson)
				}
			}
		})
	}
}

func TestSchedule_Validate(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		schedule Schedule
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid schedule",
			schedule: Schedule{
				ID:       "platform-team",
				Name:     "Platform Team Schedule",
				TeamID:   "platform",
				Timezone: "Europe/London",
				Rotation: Rotation{
					Type:         RotationTypeWeekly,
					Participants: []string{"alice", "bob"},
					StartDate:    baseTime,
				},
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "empty ID",
			schedule: Schedule{
				Name:     "Platform Team Schedule",
				TeamID:   "platform",
				Timezone: "Europe/London",
				Rotation: Rotation{
					Type:         RotationTypeWeekly,
					Participants: []string{"alice", "bob"},
					StartDate:    baseTime,
				},
			},
			wantErr: true,
			errMsg:  "schedule ID is required",
		},
		{
			name: "empty name",
			schedule: Schedule{
				ID:       "platform-team",
				TeamID:   "platform",
				Timezone: "Europe/London",
				Rotation: Rotation{
					Type:         RotationTypeWeekly,
					Participants: []string{"alice", "bob"},
					StartDate:    baseTime,
				},
			},
			wantErr: true,
			errMsg:  "schedule name is required",
		},
		{
			name: "empty team ID",
			schedule: Schedule{
				ID:       "platform-team",
				Name:     "Platform Team Schedule",
				Timezone: "Europe/London",
				Rotation: Rotation{
					Type:         RotationTypeWeekly,
					Participants: []string{"alice", "bob"},
					StartDate:    baseTime,
				},
			},
			wantErr: true,
			errMsg:  "team ID is required",
		},
		{
			name: "empty timezone",
			schedule: Schedule{
				ID:     "platform-team",
				Name:   "Platform Team Schedule",
				TeamID: "platform",
				Rotation: Rotation{
					Type:         RotationTypeWeekly,
					Participants: []string{"alice", "bob"},
					StartDate:    baseTime,
				},
			},
			wantErr: true,
			errMsg:  "timezone is required",
		},
		{
			name: "invalid timezone",
			schedule: Schedule{
				ID:       "platform-team",
				Name:     "Platform Team Schedule",
				TeamID:   "platform",
				Timezone: "Invalid/Timezone",
				Rotation: Rotation{
					Type:         RotationTypeWeekly,
					Participants: []string{"alice", "bob"},
					StartDate:    baseTime,
				},
			},
			wantErr: true,
			errMsg:  "invalid timezone",
		},
		{
			name: "invalid rotation",
			schedule: Schedule{
				ID:       "platform-team",
				Name:     "Platform Team Schedule",
				TeamID:   "platform",
				Timezone: "Europe/London",
				Rotation: Rotation{
					Type:      RotationType("invalid"),
					StartDate: baseTime,
				},
			},
			wantErr: true,
			errMsg:  "invalid rotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schedule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate() error = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestSchedule_GetCurrentOnCall(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		schedule Schedule
		at       time.Time
		expected string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "enabled schedule",
			schedule: Schedule{
				ID:       "platform-team",
				Name:     "Platform Team Schedule",
				TeamID:   "platform",
				Timezone: "UTC",
				Rotation: Rotation{
					Type:         RotationTypeWeekly,
					Participants: []string{"alice", "bob"},
					StartDate:    baseTime,
				},
				Enabled: true,
			},
			at:       baseTime.Add(24 * time.Hour),
			expected: "alice",
			wantErr:  false,
		},
		{
			name: "disabled schedule",
			schedule: Schedule{
				ID:       "platform-team",
				Name:     "Platform Team Schedule",
				TeamID:   "platform",
				Timezone: "UTC",
				Rotation: Rotation{
					Type:         RotationTypeWeekly,
					Participants: []string{"alice", "bob"},
					StartDate:    baseTime,
				},
				Enabled: false,
			},
			at:      baseTime.Add(24 * time.Hour),
			wantErr: true,
			errMsg:  "schedule platform-team is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.schedule.GetCurrentOnCall(tt.at)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCurrentOnCall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("GetCurrentOnCall() error = %v, should contain %v", err.Error(), tt.errMsg)
				}
			} else {
				if result != tt.expected {
					t.Errorf("GetCurrentOnCall() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestSchedule_IsActive(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		schedule Schedule
		at       time.Time
		expected bool
	}{
		{
			name: "enabled and after start time",
			schedule: Schedule{
				Timezone: "UTC",
				Rotation: Rotation{
					StartDate: baseTime,
				},
				Enabled: true,
			},
			at:       baseTime.Add(24 * time.Hour),
			expected: true,
		},
		{
			name: "enabled but before start time",
			schedule: Schedule{
				Timezone: "UTC",
				Rotation: Rotation{
					StartDate: baseTime,
				},
				Enabled: true,
			},
			at:       baseTime.Add(-24 * time.Hour),
			expected: false,
		},
		{
			name: "disabled",
			schedule: Schedule{
				Timezone: "UTC",
				Rotation: Rotation{
					StartDate: baseTime,
				},
				Enabled: false,
			},
			at:       baseTime.Add(24 * time.Hour),
			expected: false,
		},
		{
			name: "invalid timezone",
			schedule: Schedule{
				Timezone: "Invalid/Timezone",
				Rotation: Rotation{
					StartDate: baseTime,
				},
				Enabled: true,
			},
			at:       baseTime.Add(24 * time.Hour),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.schedule.IsActive(tt.at)
			if result != tt.expected {
				t.Errorf("IsActive() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSchedule_Clone(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)

	original := &Schedule{
		ID:        "platform-team",
		Name:      "Platform Team Schedule",
		TeamID:    "platform",
		Timezone:  "Europe/London",
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Enabled:   true,
		Rotation: Rotation{
			Type:         RotationTypeWeekly,
			Participants: []string{"alice", "bob", "charlie"},
			StartDate:    baseTime,
			Duration:     48 * time.Hour,
		},
	}

	clone := original.Clone()

	// Verify all fields are copied
	if clone.ID != original.ID {
		t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
	}
	if clone.Name != original.Name {
		t.Errorf("Clone Name = %v, want %v", clone.Name, original.Name)
	}
	if clone.TeamID != original.TeamID {
		t.Errorf("Clone TeamID = %v, want %v", clone.TeamID, original.TeamID)
	}
	if clone.Timezone != original.Timezone {
		t.Errorf("Clone Timezone = %v, want %v", clone.Timezone, original.Timezone)
	}
	if !clone.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("Clone CreatedAt = %v, want %v", clone.CreatedAt, original.CreatedAt)
	}
	if !clone.UpdatedAt.Equal(original.UpdatedAt) {
		t.Errorf("Clone UpdatedAt = %v, want %v", clone.UpdatedAt, original.UpdatedAt)
	}
	if clone.Enabled != original.Enabled {
		t.Errorf("Clone Enabled = %v, want %v", clone.Enabled, original.Enabled)
	}

	// Verify rotation is cloned
	if clone.Rotation.Type != original.Rotation.Type {
		t.Errorf("Clone Rotation.Type = %v, want %v", clone.Rotation.Type, original.Rotation.Type)
	}
	if !clone.Rotation.StartDate.Equal(original.Rotation.StartDate) {
		t.Errorf("Clone Rotation.StartDate = %v, want %v", clone.Rotation.StartDate, original.Rotation.StartDate)
	}
	if clone.Rotation.Duration != original.Rotation.Duration {
		t.Errorf("Clone Rotation.Duration = %v, want %v", clone.Rotation.Duration, original.Rotation.Duration)
	}

	// Verify participants slice is copied (not just referenced)
	if len(clone.Rotation.Participants) != len(original.Rotation.Participants) {
		t.Errorf("Clone participants length = %v, want %v", len(clone.Rotation.Participants), len(original.Rotation.Participants))
	}
	for i, participant := range original.Rotation.Participants {
		if clone.Rotation.Participants[i] != participant {
			t.Errorf("Clone participant[%d] = %v, want %v", i, clone.Rotation.Participants[i], participant)
		}
	}

	// Verify it's a deep copy - modifying clone shouldn't affect original
	clone.Rotation.Participants[0] = "modified"
	if original.Rotation.Participants[0] == "modified" {
		t.Error("Clone is not a deep copy - modifying clone affected original")
	}

	// Verify they're different objects
	if &original.Rotation.Participants == &clone.Rotation.Participants {
		t.Error("Clone shares the same participants slice reference")
	}
}

func TestSchedule_String(t *testing.T) {
	schedule := &Schedule{
		ID:     "platform-team",
		Name:   "Platform Team Schedule",
		TeamID: "platform",
		Rotation: Rotation{
			Type:         RotationTypeWeekly,
			Participants: []string{"alice", "bob", "charlie"},
		},
	}

	result := schedule.String()
	expected := "Schedule{ID: platform-team, Name: Platform Team Schedule, Team: platform, Type: weekly, Participants: [alice, bob, charlie]}"

	if result != expected {
		t.Errorf("String() = %v, want %v", result, expected)
	}
}
