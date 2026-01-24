package claudeagent

import (
	"context"
	"testing"
)

func TestNewTaskManager(t *testing.T) {
	tm, err := NewTaskManager("test-list")
	if err != nil {
		t.Fatalf("NewTaskManager() error = %v", err)
	}
	if tm.ListID() != "test-list" {
		t.Errorf("ListID() = %v, want test-list", tm.ListID())
	}
	if tm.Store() == nil {
		t.Error("Store() should not be nil")
	}
}

func TestNewTaskManagerWithStore(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)

	if tm.ListID() != "test-list" {
		t.Errorf("ListID() = %v, want test-list", tm.ListID())
	}
	if tm.Store() != store {
		t.Error("Store() should return provided store")
	}
}

func TestTaskManagerCreate(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	task, err := tm.Create(ctx, "Test task", "Description")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if task.Subject != "Test task" {
		t.Errorf("Subject = %v, want Test task", task.Subject)
	}
	if task.Description != "Description" {
		t.Errorf("Description = %v, want Description", task.Description)
	}
	if task.Status != TaskListStatusPending {
		t.Errorf("Status = %v, want pending", task.Status)
	}
}

func TestTaskManagerCreateWithOptions(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	// Note: WithMetadata should be applied first if used with WithPriority/WithEstimate,
	// or just include priority/estimate in the WithMetadata call directly.
	task, err := tm.Create(ctx, "Test task", "Description",
		WithActiveForm("Testing"),
		WithMetadata(map[string]any{"tags": []string{"test"}}),
		WithPriority("high"),
		WithEstimate("2h"),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if task.ActiveForm != "Testing" {
		t.Errorf("ActiveForm = %v, want Testing", task.ActiveForm)
	}
	if task.Metadata["priority"] != "high" {
		t.Errorf("Metadata[priority] = %v, want high", task.Metadata["priority"])
	}
	if task.Metadata["estimate"] != "2h" {
		t.Errorf("Metadata[estimate] = %v, want 2h", task.Metadata["estimate"])
	}
}

func TestTaskManagerGet(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	created, _ := tm.Create(ctx, "Test", "Desc")
	got, err := tm.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %v, want %v", got.ID, created.ID)
	}
}

func TestTaskManagerUpdate(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	task, _ := tm.Create(ctx, "Original", "Desc")

	err := tm.Update(ctx, task.ID,
		UpdateSubject("Updated"),
		UpdateDescription("New desc"),
		UpdateActiveForm("Working"),
		UpdateStatus(TaskListStatusInProgress),
		UpdateOwner("agent-1"),
		AddBlocks("2"),
		AddBlockedBy("0"),
		UpdateMetadata(map[string]any{"priority": "high"}),
	)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	updated, _ := tm.Get(ctx, task.ID)
	if updated.Subject != "Updated" {
		t.Errorf("Subject = %v, want Updated", updated.Subject)
	}
	if updated.Description != "New desc" {
		t.Errorf("Description = %v, want New desc", updated.Description)
	}
	if updated.ActiveForm != "Working" {
		t.Errorf("ActiveForm = %v, want Working", updated.ActiveForm)
	}
	if updated.Status != TaskListStatusInProgress {
		t.Errorf("Status = %v, want in_progress", updated.Status)
	}
	if updated.Owner != "agent-1" {
		t.Errorf("Owner = %v, want agent-1", updated.Owner)
	}
}

func TestTaskManagerList(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	tm.Create(ctx, "Task 1", "D1")
	tm.Create(ctx, "Task 2", "D2")
	tm.Create(ctx, "Task 3", "D3")

	tasks, err := tm.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("List() length = %v, want 3", len(tasks))
	}
}

func TestTaskManagerListPending(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	t1, _ := tm.Create(ctx, "Task 1", "D1")
	tm.Create(ctx, "Task 2", "D2")

	// Mark one as in progress.
	tm.Update(ctx, t1.ID, UpdateStatus(TaskListStatusInProgress))

	pending, err := tm.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending() error = %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("ListPending() length = %v, want 1", len(pending))
	}
}

func TestTaskManagerListInProgress(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	t1, _ := tm.Create(ctx, "Task 1", "D1")
	tm.Create(ctx, "Task 2", "D2")

	// Mark one as in progress.
	tm.Update(ctx, t1.ID, UpdateStatus(TaskListStatusInProgress))

	inProgress, err := tm.ListInProgress(ctx)
	if err != nil {
		t.Fatalf("ListInProgress() error = %v", err)
	}
	if len(inProgress) != 1 {
		t.Errorf("ListInProgress() length = %v, want 1", len(inProgress))
	}
	if inProgress[0].ID != t1.ID {
		t.Errorf("ListInProgress()[0].ID = %v, want %v", inProgress[0].ID, t1.ID)
	}
}

func TestTaskManagerListByOwner(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	t1, _ := tm.Create(ctx, "Task 1", "D1")
	tm.Create(ctx, "Task 2", "D2")

	// Assign owner.
	tm.Update(ctx, t1.ID, UpdateOwner("agent-1"))

	owned, err := tm.ListByOwner(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListByOwner() error = %v", err)
	}
	if len(owned) != 1 {
		t.Errorf("ListByOwner() length = %v, want 1", len(owned))
	}
}

func TestTaskManagerListUnblocked(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	t1, _ := tm.Create(ctx, "Blocker", "D1")
	t2, _ := tm.Create(ctx, "Blocked", "D2")

	// Block t2 on t1.
	tm.Update(ctx, t2.ID, AddBlockedBy(t1.ID))

	unblocked, err := tm.ListUnblocked(ctx)
	if err != nil {
		t.Fatalf("ListUnblocked() error = %v", err)
	}
	if len(unblocked) != 1 {
		t.Errorf("ListUnblocked() length = %v, want 1", len(unblocked))
	}
	if unblocked[0].ID != t1.ID {
		t.Errorf("ListUnblocked()[0].ID = %v, want %v", unblocked[0].ID, t1.ID)
	}
}

func TestTaskManagerClaim(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	task, _ := tm.Create(ctx, "Test", "D")

	err := tm.Claim(ctx, task.ID, "agent-1")
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}

	claimed, _ := tm.Get(ctx, task.ID)
	if claimed.Owner != "agent-1" {
		t.Errorf("Owner = %v, want agent-1", claimed.Owner)
	}
	if claimed.Status != TaskListStatusInProgress {
		t.Errorf("Status = %v, want in_progress", claimed.Status)
	}
}

func TestTaskManagerComplete(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	task, _ := tm.Create(ctx, "Test", "D")

	err := tm.Complete(ctx, task.ID)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	completed, _ := tm.Get(ctx, task.ID)
	if completed.Status != TaskListStatusCompleted {
		t.Errorf("Status = %v, want completed", completed.Status)
	}
}

func TestTaskManagerCompleteUnblocks(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	blocker, _ := tm.Create(ctx, "Blocker", "D1")
	blocked, _ := tm.Create(ctx, "Blocked", "D2")

	// Block.
	tm.Update(ctx, blocked.ID, AddBlockedBy(blocker.ID))

	// Verify blocked.
	task, _ := tm.Get(ctx, blocked.ID)
	if !task.IsBlocked() {
		t.Error("task should be blocked")
	}

	// Complete blocker.
	tm.Complete(ctx, blocker.ID)

	// Verify unblocked.
	task, _ = tm.Get(ctx, blocked.ID)
	if task.IsBlocked() {
		t.Error("task should be unblocked after blocker completed")
	}
}

func TestTaskManagerDelete(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	task, _ := tm.Create(ctx, "Test", "D")

	err := tm.Delete(ctx, task.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = tm.Get(ctx, task.ID)
	if err == nil {
		t.Error("Get() should return error after delete")
	}
}

func TestTaskManagerClear(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	tm.Create(ctx, "Task 1", "D1")
	tm.Create(ctx, "Task 2", "D2")

	err := tm.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	tasks, _ := tm.List(ctx)
	if len(tasks) != 0 {
		t.Errorf("List() length = %v, want 0", len(tasks))
	}
}

func TestTaskManagerWatch(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := tm.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if ch == nil {
		t.Error("Watch() should return non-nil channel")
	}

	// Create and verify event.
	tm.Create(ctx, "Test", "D")
	event := <-ch
	if event.Type != TaskEventCreated {
		t.Errorf("event.Type = %v, want created", event.Type)
	}
}

func TestTaskManagerNextAvailable(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	// No tasks.
	next, err := tm.NextAvailable(ctx)
	if err != nil {
		t.Fatalf("NextAvailable() error = %v", err)
	}
	if next != nil {
		t.Error("NextAvailable() should return nil when no tasks")
	}

	// Create tasks.
	t1, _ := tm.Create(ctx, "Task 1", "D1")
	tm.Create(ctx, "Task 2", "D2")

	next, err = tm.NextAvailable(ctx)
	if err != nil {
		t.Fatalf("NextAvailable() error = %v", err)
	}
	if next == nil {
		t.Fatal("NextAvailable() should return a task")
	}
	if next.ID != t1.ID {
		t.Errorf("NextAvailable().ID = %v, want %v", next.ID, t1.ID)
	}
}

func TestTaskManagerClaimNext(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	// No tasks.
	claimed, err := tm.ClaimNext(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ClaimNext() error = %v", err)
	}
	if claimed != nil {
		t.Error("ClaimNext() should return nil when no tasks")
	}

	// Create tasks.
	t1, _ := tm.Create(ctx, "Task 1", "D1")

	claimed, err = tm.ClaimNext(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ClaimNext() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimNext() should return a task")
	}
	if claimed.ID != t1.ID {
		t.Errorf("ClaimNext().ID = %v, want %v", claimed.ID, t1.ID)
	}
	if claimed.Owner != "agent-1" {
		t.Errorf("ClaimNext().Owner = %v, want agent-1", claimed.Owner)
	}
	if claimed.Status != TaskListStatusInProgress {
		t.Errorf("ClaimNext().Status = %v, want in_progress", claimed.Status)
	}
}

func TestTaskManagerStats(t *testing.T) {
	store := NewMemoryTaskStore()
	tm := NewTaskManagerWithStore("test-list", store)
	ctx := context.Background()

	// Create mix of tasks.
	t1, _ := tm.Create(ctx, "Pending 1", "D")
	t2, _ := tm.Create(ctx, "Pending 2", "D")
	t3, _ := tm.Create(ctx, "Blocked", "D")

	tm.Update(ctx, t1.ID, UpdateStatus(TaskListStatusInProgress))
	tm.Update(ctx, t2.ID, UpdateStatus(TaskListStatusCompleted))
	tm.Update(ctx, t3.ID, AddBlockedBy(t1.ID))

	stats, err := tm.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Total != 3 {
		t.Errorf("Total = %v, want 3", stats.Total)
	}
	if stats.Pending != 1 {
		t.Errorf("Pending = %v, want 1", stats.Pending)
	}
	if stats.InProgress != 1 {
		t.Errorf("InProgress = %v, want 1", stats.InProgress)
	}
	if stats.Completed != 1 {
		t.Errorf("Completed = %v, want 1", stats.Completed)
	}
	if stats.Blocked != 1 {
		t.Errorf("Blocked = %v, want 1", stats.Blocked)
	}
	// Only t2 is pending and unblocked, but t2 is completed.
	// t3 is pending but blocked.
	// No unblocked tasks after t2 completed.
	if stats.Unblocked != 0 {
		t.Errorf("Unblocked = %v, want 0", stats.Unblocked)
	}
}

func TestTaskOptions(t *testing.T) {
	t.Run("WithActiveForm", func(t *testing.T) {
		task := &TaskListItem{}
		WithActiveForm("Testing")(task)
		if task.ActiveForm != "Testing" {
			t.Errorf("ActiveForm = %v, want Testing", task.ActiveForm)
		}
	})

	t.Run("WithMetadata", func(t *testing.T) {
		task := &TaskListItem{}
		WithMetadata(map[string]any{"key": "value"})(task)
		if task.Metadata["key"] != "value" {
			t.Errorf("Metadata[key] = %v, want value", task.Metadata["key"])
		}
	})

	t.Run("WithPriority", func(t *testing.T) {
		task := &TaskListItem{}
		WithPriority("high")(task)
		if task.Metadata["priority"] != "high" {
			t.Errorf("Metadata[priority] = %v, want high", task.Metadata["priority"])
		}
	})

	t.Run("WithEstimate", func(t *testing.T) {
		task := &TaskListItem{}
		WithEstimate("2h")(task)
		if task.Metadata["estimate"] != "2h" {
			t.Errorf("Metadata[estimate] = %v, want 2h", task.Metadata["estimate"])
		}
	})
}

func TestUpdateOptions(t *testing.T) {
	t.Run("UpdateSubject", func(t *testing.T) {
		input := &TaskUpdateInput{}
		UpdateSubject("New subject")(input)
		if input.Subject != "New subject" {
			t.Errorf("Subject = %v, want New subject", input.Subject)
		}
	})

	t.Run("UpdateDescription", func(t *testing.T) {
		input := &TaskUpdateInput{}
		UpdateDescription("New desc")(input)
		if input.Description != "New desc" {
			t.Errorf("Description = %v, want New desc", input.Description)
		}
	})

	t.Run("UpdateActiveForm", func(t *testing.T) {
		input := &TaskUpdateInput{}
		UpdateActiveForm("Working")(input)
		if input.ActiveForm != "Working" {
			t.Errorf("ActiveForm = %v, want Working", input.ActiveForm)
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		input := &TaskUpdateInput{}
		UpdateStatus(TaskListStatusCompleted)(input)
		if input.Status != TaskListStatusCompleted {
			t.Errorf("Status = %v, want completed", input.Status)
		}
	})

	t.Run("UpdateOwner", func(t *testing.T) {
		input := &TaskUpdateInput{}
		UpdateOwner("agent-1")(input)
		if input.Owner != "agent-1" {
			t.Errorf("Owner = %v, want agent-1", input.Owner)
		}
	})

	t.Run("AddBlocks", func(t *testing.T) {
		input := &TaskUpdateInput{}
		AddBlocks("1", "2")(input)
		if len(input.AddBlocks) != 2 {
			t.Errorf("AddBlocks length = %v, want 2", len(input.AddBlocks))
		}
	})

	t.Run("AddBlockedBy", func(t *testing.T) {
		input := &TaskUpdateInput{}
		AddBlockedBy("1", "2")(input)
		if len(input.AddBlockedBy) != 2 {
			t.Errorf("AddBlockedBy length = %v, want 2", len(input.AddBlockedBy))
		}
	})

	t.Run("UpdateMetadata", func(t *testing.T) {
		input := &TaskUpdateInput{}
		UpdateMetadata(map[string]any{"key": "value"})(input)
		if input.Metadata["key"] != "value" {
			t.Errorf("Metadata[key] = %v, want value", input.Metadata["key"])
		}
	})
}
