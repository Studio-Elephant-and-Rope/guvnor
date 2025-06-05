package ports

import (
	"errors"
	"testing"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
)

func TestListFilter_Validate(t *testing.T) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	tests := []struct {
		name    string
		filter  ListFilter
		wantErr bool
		errType error
		// Expected values after validation
		expectLimit     int
		expectSortBy    string
		expectSortOrder string
	}{
		{
			name:            "valid empty filter sets defaults",
			filter:          ListFilter{},
			wantErr:         false,
			expectLimit:     50,
			expectSortBy:    "created_at",
			expectSortOrder: "desc",
		},
		{
			name: "valid filter with all fields",
			filter: ListFilter{
				TeamID:        "team-1",
				Status:        []domain.Status{domain.StatusTriggered, domain.StatusAcknowledged},
				Severity:      []domain.Severity{domain.SeverityHigh, domain.SeverityCritical},
				AssigneeID:    "user-1",
				ServiceID:     "service-1",
				Labels:        map[string]string{"env": "production"},
				CreatedAfter:  &yesterday,
				CreatedBefore: &now,
				UpdatedAfter:  &yesterday,
				UpdatedBefore: &now,
				Search:        "database error",
				Limit:         25,
				Offset:        100,
				SortBy:        "severity",
				SortOrder:     "asc",
			},
			wantErr:         false,
			expectLimit:     25,
			expectSortBy:    "severity",
			expectSortOrder: "asc",
		},
		{
			name: "zero limit gets default",
			filter: ListFilter{
				Limit: 0,
			},
			wantErr:     false,
			expectLimit: 50,
		},
		{
			name: "negative limit gets default",
			filter: ListFilter{
				Limit: -5,
			},
			wantErr:     false,
			expectLimit: 50,
		},
		{
			name: "limit too high",
			filter: ListFilter{
				Limit: 1001,
			},
			wantErr: true,
			errType: ErrInvalidInput,
		},
		{
			name: "negative offset",
			filter: ListFilter{
				Offset: -1,
			},
			wantErr: true,
			errType: ErrInvalidInput,
		},
		{
			name: "invalid created time range",
			filter: ListFilter{
				CreatedAfter:  &tomorrow,
				CreatedBefore: &yesterday,
			},
			wantErr: true,
			errType: ErrInvalidInput,
		},
		{
			name: "invalid updated time range",
			filter: ListFilter{
				UpdatedAfter:  &tomorrow,
				UpdatedBefore: &yesterday,
			},
			wantErr: true,
			errType: ErrInvalidInput,
		},
		{
			name: "invalid sort field",
			filter: ListFilter{
				SortBy: "invalid_field",
			},
			wantErr: true,
			errType: ErrInvalidInput,
		},
		{
			name: "invalid sort order",
			filter: ListFilter{
				SortOrder: "invalid_order",
			},
			wantErr: true,
			errType: ErrInvalidInput,
		},
		{
			name: "invalid status",
			filter: ListFilter{
				Status: []domain.Status{"invalid_status"},
			},
			wantErr: true,
			errType: ErrInvalidInput,
		},
		{
			name: "invalid severity",
			filter: ListFilter{
				Severity: []domain.Severity{"invalid_severity"},
			},
			wantErr: true,
			errType: ErrInvalidInput,
		},
		{
			name: "valid custom sort parameters",
			filter: ListFilter{
				SortBy:    "updated_at",
				SortOrder: "asc",
			},
			wantErr:         false,
			expectSortBy:    "updated_at",
			expectSortOrder: "asc",
		},
		{
			name: "empty sort order gets default",
			filter: ListFilter{
				SortBy: "severity",
			},
			wantErr:         false,
			expectSortBy:    "severity",
			expectSortOrder: "desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("ListFilter.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check specific error type
			if tt.wantErr && tt.errType != nil {
				if !errors.Is(err, tt.errType) {
					t.Errorf("ListFilter.Validate() error = %v, want %v", err, tt.errType)
				}
				return
			}

			// Check expected values after validation
			if !tt.wantErr {
				if tt.expectLimit > 0 && tt.filter.Limit != tt.expectLimit {
					t.Errorf("Expected limit %d, got %d", tt.expectLimit, tt.filter.Limit)
				}
				if tt.expectSortBy != "" && tt.filter.SortBy != tt.expectSortBy {
					t.Errorf("Expected sort_by %s, got %s", tt.expectSortBy, tt.filter.SortBy)
				}
				if tt.expectSortOrder != "" && tt.filter.SortOrder != tt.expectSortOrder {
					t.Errorf("Expected sort_order %s, got %s", tt.expectSortOrder, tt.filter.SortOrder)
				}
			}
		})
	}
}

func TestListFilter_ValidateEdgeCases(t *testing.T) {
	t.Run("maximum valid limit", func(t *testing.T) {
		filter := ListFilter{Limit: 1000}
		err := filter.Validate()
		if err != nil {
			t.Errorf("Expected no error for limit 1000, got %v", err)
		}
	})

	t.Run("zero offset is valid", func(t *testing.T) {
		filter := ListFilter{Offset: 0}
		err := filter.Validate()
		if err != nil {
			t.Errorf("Expected no error for offset 0, got %v", err)
		}
	})

	t.Run("same time for before and after is valid", func(t *testing.T) {
		now := time.Now()
		filter := ListFilter{
			CreatedAfter:  &now,
			CreatedBefore: &now,
		}
		err := filter.Validate()
		if err != nil {
			t.Errorf("Expected no error for same before/after times, got %v", err)
		}
	})

	t.Run("mixed valid and invalid status", func(t *testing.T) {
		filter := ListFilter{
			Status: []domain.Status{domain.StatusTriggered, "invalid"},
		}
		err := filter.Validate()
		if err == nil {
			t.Error("Expected error for mixed valid/invalid status")
		}
	})

	t.Run("empty search string is valid", func(t *testing.T) {
		filter := ListFilter{Search: ""}
		err := filter.Validate()
		if err != nil {
			t.Errorf("Expected no error for empty search, got %v", err)
		}
	})
}

func TestRepositoryErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "not found error",
			err:  ErrNotFound,
			want: "resource not found",
		},
		{
			name: "already exists error",
			err:  ErrAlreadyExists,
			want: "resource already exists",
		},
		{
			name: "conflict error",
			err:  ErrConflict,
			want: "resource conflict",
		},
		{
			name: "invalid input error",
			err:  ErrInvalidInput,
			want: "invalid input",
		},
		{
			name: "connection failed error",
			err:  ErrConnectionFailed,
			want: "storage connection failed",
		},
		{
			name: "timeout error",
			err:  ErrTimeout,
			want: "operation timed out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.want {
				t.Errorf("Error message = %v, want %v", tt.err.Error(), tt.want)
			}
		})
	}
}

func TestListResult(t *testing.T) {
	t.Run("list result structure", func(t *testing.T) {
		now := time.Now()
		incidents := []*domain.Incident{
			{
				ID:        "incident-1",
				Title:     "Test incident",
				Status:    domain.StatusTriggered,
				Severity:  domain.SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: now,
				UpdatedAt: now,
			},
		}

		result := &ListResult{
			Incidents: incidents,
			Total:     100,
			Limit:     50,
			Offset:    0,
			HasMore:   true,
		}

		if len(result.Incidents) != 1 {
			t.Errorf("Expected 1 incident, got %d", len(result.Incidents))
		}
		if result.Total != 100 {
			t.Errorf("Expected total 100, got %d", result.Total)
		}
		if result.Limit != 50 {
			t.Errorf("Expected limit 50, got %d", result.Limit)
		}
		if result.Offset != 0 {
			t.Errorf("Expected offset 0, got %d", result.Offset)
		}
		if !result.HasMore {
			t.Error("Expected HasMore to be true")
		}
	})
}

func TestListFilter_DefaultBehaviour(t *testing.T) {
	t.Run("default values are applied correctly", func(t *testing.T) {
		filter := ListFilter{}
		err := filter.Validate()
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}

		// Check all defaults
		if filter.Limit != 50 {
			t.Errorf("Expected default limit 50, got %d", filter.Limit)
		}
		if filter.SortBy != "created_at" {
			t.Errorf("Expected default sort_by 'created_at', got %s", filter.SortBy)
		}
		if filter.SortOrder != "desc" {
			t.Errorf("Expected default sort_order 'desc', got %s", filter.SortOrder)
		}
		if filter.Offset != 0 {
			t.Errorf("Expected default offset 0, got %d", filter.Offset)
		}
	})
}

// BenchmarkListFilterValidate tests the performance of filter validation
func BenchmarkListFilterValidate(b *testing.B) {
	now := time.Now()
	filter := ListFilter{
		TeamID:        "team-1",
		Status:        []domain.Status{domain.StatusTriggered},
		Severity:      []domain.Severity{domain.SeverityHigh},
		AssigneeID:    "user-1",
		ServiceID:     "service-1",
		Labels:        map[string]string{"env": "production"},
		CreatedAfter:  &now,
		CreatedBefore: &now,
		Search:        "test",
		Limit:         100,
		Offset:        50,
		SortBy:        "created_at",
		SortOrder:     "desc",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Validate()
	}
}
