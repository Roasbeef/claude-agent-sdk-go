package claudeagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// testTaskStore runs a common test suite against any TaskStore implementation.
func testTaskStore(t *testing.T, store TaskStore, listID string) {
	ctx := context.Background()

	// Test Create and Get.
	t.Run("create and get", func(t *testing.T) {
		task := TaskListItem{
			Subject:     "Test task",
			Description: "A test task",
			ActiveForm:  "Testing",
		}

		id, err := store.Create(ctx, listID, task)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if id == "" {
			t.Error("Create() returned empty ID")
		}

		got, err := store.Get(ctx, listID, id)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.ID != id {
			t.Errorf("Get() ID = %v, want %v", got.ID, id)
		}
		if got.Subject != task.Subject {
			t.Errorf("Get() Subject = %v, want %v", got.Subject, task.Subject)
		}
		if got.Status != TaskListStatusPending {
			t.Errorf("Get() Status = %v, want pending", got.Status)
		}
	})

	// Test Get not found.
	t.Run("get not found", func(t *testing.T) {
		_, err := store.Get(ctx, listID, "nonexistent")
		if err == nil {
			t.Error("Get() expected error for nonexistent task")
		}
		var notFound *ErrTaskNotFound
		if !errors.As(err, &notFound) {
			t.Errorf("Get() error = %v, want ErrTaskNotFound", err)
		}
	})

	// Test Update.
	t.Run("update", func(t *testing.T) {
		task := TaskListItem{Subject: "Original", Description: "Desc"}
		id, _ := store.Create(ctx, listID, task)

		err := store.Update(ctx, listID, id, TaskUpdateInput{
			TaskID:  id,
			Subject: "Updated",
			Status:  TaskListStatusInProgress,
			Owner:   "agent-1",
		})
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		got, _ := store.Get(ctx, listID, id)
		if got.Subject != "Updated" {
			t.Errorf("Update() Subject = %v, want Updated", got.Subject)
		}
		if got.Status != TaskListStatusInProgress {
			t.Errorf("Update() Status = %v, want in_progress", got.Status)
		}
		if got.Owner != "agent-1" {
			t.Errorf("Update() Owner = %v, want agent-1", got.Owner)
		}
	})

	// Test Update not found.
	t.Run("update not found", func(t *testing.T) {
		err := store.Update(ctx, listID, "nonexistent", TaskUpdateInput{
			TaskID:  "nonexistent",
			Subject: "Updated",
		})
		if err == nil {
			t.Error("Update() expected error for nonexistent task")
		}
	})

	// Test Update with AddBlocks and AddBlockedBy.
	t.Run("update blocks", func(t *testing.T) {
		task := TaskListItem{Subject: "Blocked", Description: "Desc"}
		id, _ := store.Create(ctx, listID, task)

		err := store.Update(ctx, listID, id, TaskUpdateInput{
			TaskID:       id,
			AddBlocks:    []string{"99"},
			AddBlockedBy: []string{"100"},
		})
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		got, _ := store.Get(ctx, listID, id)
		if len(got.Blocks) != 1 || got.Blocks[0] != "99" {
			t.Errorf("Update() Blocks = %v, want [99]", got.Blocks)
		}
		if len(got.BlockedBy) != 1 || got.BlockedBy[0] != "100" {
			t.Errorf("Update() BlockedBy = %v, want [100]", got.BlockedBy)
		}

		// Add more, check no duplicates.
		err = store.Update(ctx, listID, id, TaskUpdateInput{
			TaskID:       id,
			AddBlocks:    []string{"99", "101"},
			AddBlockedBy: []string{"100", "102"},
		})
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		got, _ = store.Get(ctx, listID, id)
		if len(got.Blocks) != 2 {
			t.Errorf("Update() Blocks length = %v, want 2", len(got.Blocks))
		}
		if len(got.BlockedBy) != 2 {
			t.Errorf("Update() BlockedBy length = %v, want 2", len(got.BlockedBy))
		}
	})

	// Test Update with Metadata.
	t.Run("update metadata", func(t *testing.T) {
		task := TaskListItem{Subject: "Meta", Description: "Desc"}
		id, _ := store.Create(ctx, listID, task)

		// Add metadata.
		err := store.Update(ctx, listID, id, TaskUpdateInput{
			TaskID:   id,
			Metadata: map[string]any{"priority": "high", "estimate": "2h"},
		})
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		got, _ := store.Get(ctx, listID, id)
		if got.Metadata["priority"] != "high" {
			t.Errorf("Metadata[priority] = %v, want high", got.Metadata["priority"])
		}

		// Update and delete metadata.
		err = store.Update(ctx, listID, id, TaskUpdateInput{
			TaskID:   id,
			Metadata: map[string]any{"priority": "low", "estimate": nil},
		})
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		got, _ = store.Get(ctx, listID, id)
		if got.Metadata["priority"] != "low" {
			t.Errorf("Metadata[priority] = %v, want low", got.Metadata["priority"])
		}
		if _, ok := got.Metadata["estimate"]; ok {
			t.Error("Metadata[estimate] should be deleted")
		}
	})

	// Test List.
	t.Run("list", func(t *testing.T) {
		// Clear first.
		_ = store.Clear(ctx, listID)

		// Create multiple tasks.
		store.Create(ctx, listID, TaskListItem{Subject: "Task 1", Description: "D1"})
		store.Create(ctx, listID, TaskListItem{Subject: "Task 2", Description: "D2"})
		store.Create(ctx, listID, TaskListItem{Subject: "Task 3", Description: "D3"})

		tasks, err := store.List(ctx, listID)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(tasks) != 3 {
			t.Errorf("List() length = %v, want 3", len(tasks))
		}

		// Verify sorted by ID.
		for i := 1; i < len(tasks); i++ {
			if compareTaskIDs(tasks[i-1].ID, tasks[i].ID) >= 0 {
				t.Errorf("List() not sorted: %v >= %v", tasks[i-1].ID, tasks[i].ID)
			}
		}
	})

	// Test List empty.
	t.Run("list empty", func(t *testing.T) {
		tasks, err := store.List(ctx, "empty-list-"+listID)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("List() length = %v, want 0", len(tasks))
		}
	})

	// Test Delete.
	t.Run("delete", func(t *testing.T) {
		task := TaskListItem{Subject: "To delete", Description: "D"}
		id, _ := store.Create(ctx, listID, task)

		err := store.Delete(ctx, listID, id)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err = store.Get(ctx, listID, id)
		if err == nil {
			t.Error("Get() expected error after Delete")
		}
	})

	// Test Delete not found.
	t.Run("delete not found", func(t *testing.T) {
		err := store.Delete(ctx, listID, "nonexistent")
		if err == nil {
			t.Error("Delete() expected error for nonexistent task")
		}
	})

	// Test Clear.
	t.Run("clear", func(t *testing.T) {
		clearListID := "clear-test-" + listID
		store.Create(ctx, clearListID, TaskListItem{Subject: "T1", Description: "D"})
		store.Create(ctx, clearListID, TaskListItem{Subject: "T2", Description: "D"})

		err := store.Clear(ctx, clearListID)
		if err != nil {
			t.Fatalf("Clear() error = %v", err)
		}

		tasks, _ := store.List(ctx, clearListID)
		if len(tasks) != 0 {
			t.Errorf("Clear() list length = %v, want 0", len(tasks))
		}
	})
}

func TestMemoryTaskStore(t *testing.T) {
	store := NewMemoryTaskStore()
	testTaskStore(t, store, "memory-test")
}

func TestMemoryTaskStoreSubscribe(t *testing.T) {
	store := NewMemoryTaskStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := store.Subscribe(ctx, "sub-test")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Create a task and verify event.
	go func() {
		time.Sleep(10 * time.Millisecond)
		store.Create(ctx, "sub-test", TaskListItem{Subject: "Test", Description: "D"})
	}()

	select {
	case event := <-ch:
		if event.Type != TaskEventCreated {
			t.Errorf("event.Type = %v, want created", event.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("Subscribe() did not receive event")
	}
}

func TestMemoryTaskStoreUnblock(t *testing.T) {
	store := NewMemoryTaskStore()
	ctx := context.Background()

	// Create blocking task.
	blockerID, _ := store.Create(ctx, "unblock-test", TaskListItem{
		Subject:     "Blocker",
		Description: "D",
	})

	// Create blocked task.
	blockedID, _ := store.Create(ctx, "unblock-test", TaskListItem{
		Subject:     "Blocked",
		Description: "D",
		BlockedBy:   []string{blockerID},
	})

	// Verify blocked.
	blocked, _ := store.Get(ctx, "unblock-test", blockedID)
	if !blocked.IsBlocked() {
		t.Error("task should be blocked")
	}

	// Complete blocker.
	store.Update(ctx, "unblock-test", blockerID, TaskUpdateInput{
		TaskID: blockerID,
		Status: TaskListStatusCompleted,
	})

	// Verify unblocked.
	blocked, _ = store.Get(ctx, "unblock-test", blockedID)
	if blocked.IsBlocked() {
		t.Error("task should be unblocked after blocker completed")
	}
}

func TestMemoryTaskStoreConcurrent(t *testing.T) {
	store := NewMemoryTaskStore()
	ctx := context.Background()
	listID := "concurrent-test"

	var wg sync.WaitGroup
	numGoroutines := 10
	tasksPerGoroutine := 10

	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range tasksPerGoroutine {
				id, _ := store.Create(ctx, listID, TaskListItem{
					Subject:     "Task",
					Description: "D",
				})
				store.Update(ctx, listID, id, TaskUpdateInput{
					TaskID: id,
					Status: TaskListStatusInProgress,
				})
			}
		}()
	}

	wg.Wait()

	tasks, err := store.List(ctx, listID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	expectedTasks := numGoroutines * tasksPerGoroutine
	if len(tasks) != expectedTasks {
		t.Errorf("List() length = %v, want %v", len(tasks), expectedTasks)
	}
}

func TestTaskHelpers(t *testing.T) {
	t.Run("taskAppendUnique", func(t *testing.T) {
		slice := []string{"a", "b"}
		result := taskAppendUnique(slice, "b", "c", "d")
		if len(result) != 4 {
			t.Errorf("taskAppendUnique() length = %v, want 4", len(result))
		}
		// Verify no duplicate "b".
		count := 0
		for _, s := range result {
			if s == "b" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("taskAppendUnique() b count = %v, want 1", count)
		}
	})

	t.Run("taskContainsString", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		if !taskContainsString(slice, "b") {
			t.Error("taskContainsString() should find 'b'")
		}
		if taskContainsString(slice, "d") {
			t.Error("taskContainsString() should not find 'd'")
		}
		if taskContainsString(nil, "a") {
			t.Error("taskContainsString() should return false for nil")
		}
	})

	t.Run("taskRemoveString", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		result := taskRemoveString(slice, "b")
		if len(result) != 2 {
			t.Errorf("taskRemoveString() length = %v, want 2", len(result))
		}
		if taskContainsString(result, "b") {
			t.Error("taskRemoveString() should remove 'b'")
		}
	})

	t.Run("compareTaskIDs", func(t *testing.T) {
		if compareTaskIDs("1", "2") >= 0 {
			t.Error("compareTaskIDs(1, 2) should be < 0")
		}
		if compareTaskIDs("2", "1") <= 0 {
			t.Error("compareTaskIDs(2, 1) should be > 0")
		}
		if compareTaskIDs("10", "2") <= 0 {
			t.Error("compareTaskIDs(10, 2) should be > 0 (numeric)")
		}
		if compareTaskIDs("a", "b") >= 0 {
			t.Error("compareTaskIDs(a, b) should be < 0 (string)")
		}
	})

	t.Run("parseTaskID", func(t *testing.T) {
		if parseTaskID("123") != 123 {
			t.Error("parseTaskID(123) should return 123")
		}
		if parseTaskID("abc") != 0 {
			t.Error("parseTaskID(abc) should return 0")
		}
		if parseTaskID("12a") != 0 {
			t.Error("parseTaskID(12a) should return 0")
		}
	})

	t.Run("sortTasksByID", func(t *testing.T) {
		tasks := []TaskListItem{
			{ID: "10"},
			{ID: "2"},
			{ID: "1"},
		}
		sortTasksByID(tasks)
		if tasks[0].ID != "1" || tasks[1].ID != "2" || tasks[2].ID != "10" {
			t.Errorf("sortTasksByID() = [%s, %s, %s], want [1, 2, 10]",
				tasks[0].ID, tasks[1].ID, tasks[2].ID)
		}
	})
}
