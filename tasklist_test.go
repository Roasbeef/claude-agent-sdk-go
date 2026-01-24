package claudeagent

import (
	"encoding/json"
	"testing"
)

func TestTaskListItemIsBlocked(t *testing.T) {
	tests := []struct {
		name      string
		blockedBy []string
		want      bool
	}{
		{"nil blockedBy", nil, false},
		{"empty blockedBy", []string{}, false},
		{"one blocker", []string{"1"}, true},
		{"multiple blockers", []string{"1", "2", "3"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &TaskListItem{BlockedBy: tt.blockedBy}
			if got := task.IsBlocked(); got != tt.want {
				t.Errorf("IsBlocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskListItemIsClaimed(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		want  bool
	}{
		{"no owner", "", false},
		{"has owner", "agent-1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &TaskListItem{Owner: tt.owner}
			if got := task.IsClaimed(); got != tt.want {
				t.Errorf("IsClaimed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskListItemIsAvailable(t *testing.T) {
	tests := []struct {
		name   string
		status TaskListStatus
		owner  string
		blocks []string
		want   bool
	}{
		{"pending, unclaimed, unblocked", TaskListStatusPending, "", nil, true},
		{"pending, claimed", TaskListStatusPending, "agent-1", nil, false},
		{"pending, blocked", TaskListStatusPending, "", []string{"1"}, false},
		{"in progress", TaskListStatusInProgress, "", nil, false},
		{"completed", TaskListStatusCompleted, "", nil, false},
		{"pending, claimed, blocked", TaskListStatusPending, "agent-1", []string{"1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &TaskListItem{
				Status:    tt.status,
				Owner:     tt.owner,
				BlockedBy: tt.blocks,
			}
			if got := task.IsAvailable(); got != tt.want {
				t.Errorf("IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskListItemJSON(t *testing.T) {
	task := TaskListItem{
		ID:          "1",
		Subject:     "Test task",
		Description: "A test task description",
		ActiveForm:  "Testing",
		Owner:       "agent-1",
		Status:      TaskListStatusInProgress,
		Blocks:      []string{"2", "3"},
		BlockedBy:   []string{"0"},
		Metadata:    map[string]any{"priority": "high", "estimate": "2h"},
	}

	// Marshal to JSON.
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Unmarshal back.
	var decoded TaskListItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify fields.
	if decoded.ID != task.ID {
		t.Errorf("ID = %v, want %v", decoded.ID, task.ID)
	}
	if decoded.Subject != task.Subject {
		t.Errorf("Subject = %v, want %v", decoded.Subject, task.Subject)
	}
	if decoded.Description != task.Description {
		t.Errorf("Description = %v, want %v", decoded.Description, task.Description)
	}
	if decoded.ActiveForm != task.ActiveForm {
		t.Errorf("ActiveForm = %v, want %v", decoded.ActiveForm, task.ActiveForm)
	}
	if decoded.Owner != task.Owner {
		t.Errorf("Owner = %v, want %v", decoded.Owner, task.Owner)
	}
	if decoded.Status != task.Status {
		t.Errorf("Status = %v, want %v", decoded.Status, task.Status)
	}
	if len(decoded.Blocks) != len(task.Blocks) {
		t.Errorf("Blocks length = %v, want %v", len(decoded.Blocks), len(task.Blocks))
	}
	if len(decoded.BlockedBy) != len(task.BlockedBy) {
		t.Errorf("BlockedBy length = %v, want %v", len(decoded.BlockedBy), len(task.BlockedBy))
	}
	if decoded.Metadata["priority"] != task.Metadata["priority"] {
		t.Errorf("Metadata[priority] = %v, want %v", decoded.Metadata["priority"], task.Metadata["priority"])
	}
}

func TestTaskListItemJSONOmitEmpty(t *testing.T) {
	task := TaskListItem{
		ID:          "1",
		Subject:     "Test",
		Description: "Desc",
		Status:      TaskListStatusPending,
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Verify omitempty fields are not present.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if _, ok := raw["activeForm"]; ok {
		t.Error("activeForm should be omitted when empty")
	}
	if _, ok := raw["owner"]; ok {
		t.Error("owner should be omitted when empty")
	}
	if _, ok := raw["blocks"]; ok {
		t.Error("blocks should be omitted when nil")
	}
	if _, ok := raw["blockedBy"]; ok {
		t.Error("blockedBy should be omitted when nil")
	}
	if _, ok := raw["metadata"]; ok {
		t.Error("metadata should be omitted when nil")
	}
}

func TestTaskUpdateInputJSON(t *testing.T) {
	input := TaskUpdateInput{
		TaskID:       "1",
		Subject:      "Updated subject",
		Status:       TaskListStatusInProgress,
		Owner:        "agent-1",
		AddBlocks:    []string{"2"},
		AddBlockedBy: []string{"0"},
		Metadata:     map[string]any{"priority": "low"},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded TaskUpdateInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.TaskID != input.TaskID {
		t.Errorf("TaskID = %v, want %v", decoded.TaskID, input.TaskID)
	}
	if decoded.Status != input.Status {
		t.Errorf("Status = %v, want %v", decoded.Status, input.Status)
	}
	if len(decoded.AddBlocks) != 1 || decoded.AddBlocks[0] != "2" {
		t.Errorf("AddBlocks = %v, want [2]", decoded.AddBlocks)
	}
}

func TestTaskCreateInputJSON(t *testing.T) {
	input := TaskCreateInput{
		Subject:     "New task",
		Description: "Task description",
		ActiveForm:  "Creating task",
		Metadata:    map[string]any{"priority": "high"},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded TaskCreateInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Subject != input.Subject {
		t.Errorf("Subject = %v, want %v", decoded.Subject, input.Subject)
	}
	if decoded.Description != input.Description {
		t.Errorf("Description = %v, want %v", decoded.Description, input.Description)
	}
}

func TestTaskEventJSON(t *testing.T) {
	task := &TaskListItem{
		ID:      "1",
		Subject: "Test",
		Status:  TaskListStatusPending,
	}
	event := TaskEvent{
		Type:    TaskEventCreated,
		ListID:  "my-list",
		TaskID:  "1",
		Task:    task,
		AgentID: "agent-1",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded TaskEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Type != event.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, event.Type)
	}
	if decoded.ListID != event.ListID {
		t.Errorf("ListID = %v, want %v", decoded.ListID, event.ListID)
	}
	if decoded.Task == nil {
		t.Error("Task should not be nil")
	}
}

func TestErrTaskNotFoundError(t *testing.T) {
	err := &ErrTaskNotFound{TaskID: "123"}
	want := "task not found: 123"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

func TestErrTaskAlreadyExistsError(t *testing.T) {
	err := &ErrTaskAlreadyExists{TaskID: "123"}
	want := "task already exists: 123"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

func TestErrInvalidTaskStatusError(t *testing.T) {
	err := &ErrInvalidTaskStatus{
		TaskID: "123",
		From:   TaskListStatusCompleted,
		To:     TaskListStatusPending,
	}
	want := "invalid status transition for task 123: completed -> pending"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

func TestErrTaskBlockedError(t *testing.T) {
	err := &ErrTaskBlocked{TaskID: "123", BlockedBy: []string{"1", "2"}}
	want := "task 123 is blocked by other tasks"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

func TestTaskListStatusValues(t *testing.T) {
	// Verify status values match expected strings.
	if TaskListStatusPending != "pending" {
		t.Errorf("TaskListStatusPending = %v, want pending", TaskListStatusPending)
	}
	if TaskListStatusInProgress != "in_progress" {
		t.Errorf("TaskListStatusInProgress = %v, want in_progress", TaskListStatusInProgress)
	}
	if TaskListStatusCompleted != "completed" {
		t.Errorf("TaskListStatusCompleted = %v, want completed", TaskListStatusCompleted)
	}
}

func TestTaskEventTypeValues(t *testing.T) {
	// Verify event type values match expected strings.
	if TaskEventCreated != "created" {
		t.Errorf("TaskEventCreated = %v, want created", TaskEventCreated)
	}
	if TaskEventUpdated != "updated" {
		t.Errorf("TaskEventUpdated = %v, want updated", TaskEventUpdated)
	}
	if TaskEventDeleted != "deleted" {
		t.Errorf("TaskEventDeleted = %v, want deleted", TaskEventDeleted)
	}
	if TaskEventClaimed != "claimed" {
		t.Errorf("TaskEventClaimed = %v, want claimed", TaskEventClaimed)
	}
	if TaskEventCompleted != "completed" {
		t.Errorf("TaskEventCompleted = %v, want completed", TaskEventCompleted)
	}
	if TaskEventUnblocked != "unblocked" {
		t.Errorf("TaskEventUnblocked = %v, want unblocked", TaskEventUnblocked)
	}
}
