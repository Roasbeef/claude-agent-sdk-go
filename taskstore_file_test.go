//go:build unix

package claudeagent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileTaskStore(t *testing.T) {
	// Create temp directory for tests.
	tmpDir, err := os.MkdirTemp("", "taskstore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewFileTaskStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileTaskStore() error = %v", err)
	}

	testTaskStore(t, store, "file-test")
}

func TestFileTaskStoreDefaultPath(t *testing.T) {
	// Test that NewFileTaskStore with empty path uses default.
	store, err := NewFileTaskStore("")
	if err != nil {
		t.Fatalf("NewFileTaskStore() error = %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".claude", "tasks")
	if store.baseDir != expected {
		t.Errorf("baseDir = %v, want %v", store.baseDir, expected)
	}
}

func TestFileTaskStoreAtomicWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-atomic-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx := context.Background()

	// Create a task.
	id, err := store.Create(ctx, "atomic-test", TaskListItem{
		Subject:     "Test",
		Description: "D",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify file exists.
	taskPath := filepath.Join(tmpDir, "atomic-test", id+".json")
	if _, err := os.Stat(taskPath); os.IsNotExist(err) {
		t.Error("task file should exist after create")
	}

	// Verify no temp files remain.
	entries, _ := os.ReadDir(filepath.Join(tmpDir, "atomic-test"))
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("temp file should not remain: %s", entry.Name())
		}
	}
}

func TestFileTaskStoreSubscribe(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-sub-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := store.Subscribe(ctx, "sub-test")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Create and check event.
	store.Create(ctx, "sub-test", TaskListItem{Subject: "Test", Description: "D"})

	select {
	case event := <-ch:
		if event.Type != TaskEventCreated {
			t.Errorf("event.Type = %v, want created", event.Type)
		}
	default:
		t.Error("expected event from subscribe")
	}

	// Cancel and verify cleanup.
	cancel()
}

func TestFileTaskStoreExport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-export-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx := context.Background()

	// Create tasks.
	store.Create(ctx, "export-test", TaskListItem{Subject: "Task 1", Description: "D1"})
	store.Create(ctx, "export-test", TaskListItem{Subject: "Task 2", Description: "D2"})

	// Export.
	tasks, err := store.Export(ctx, "export-test")
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("Export() length = %v, want 2", len(tasks))
	}
}

func TestFileTaskStoreImport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-import-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx := context.Background()

	// Import with clear.
	tasks := []TaskListItem{
		{ID: "1", Subject: "Task 1", Description: "D1", Status: TaskListStatusPending},
		{ID: "2", Subject: "Task 2", Description: "D2", Status: TaskListStatusCompleted},
	}
	err = store.Import(ctx, "import-test", tasks, true)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	// Verify imported.
	imported, _ := store.List(ctx, "import-test")
	if len(imported) != 2 {
		t.Errorf("Import() list length = %v, want 2", len(imported))
	}
}

func TestFileTaskStoreListIDs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-listids-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx := context.Background()

	// Create tasks in multiple lists.
	store.Create(ctx, "list-a", TaskListItem{Subject: "A", Description: "D"})
	store.Create(ctx, "list-b", TaskListItem{Subject: "B", Description: "D"})

	// List IDs.
	ids, err := store.ListIDs(ctx)
	if err != nil {
		t.Fatalf("ListIDs() error = %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("ListIDs() length = %v, want 2", len(ids))
	}
}

func TestFileTaskStoreLocking(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-lock-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx := context.Background()

	// Create a task to lock.
	id, _ := store.Create(ctx, "lock-test", TaskListItem{Subject: "Test", Description: "D"})

	// Acquire lock.
	release, err := store.Lock(ctx, "lock-test", id)
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}

	// TryLock should fail.
	_, acquired, err := store.TryLock(ctx, "lock-test", id)
	if err != nil {
		t.Fatalf("TryLock() error = %v", err)
	}
	if acquired {
		t.Error("TryLock() should return false when locked")
	}

	// Release and try again.
	release()

	release2, acquired, err := store.TryLock(ctx, "lock-test", id)
	if err != nil {
		t.Fatalf("TryLock() error = %v", err)
	}
	if !acquired {
		t.Error("TryLock() should return true after release")
	}
	release2()
}

func TestFileTaskStoreConcurrent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-concurrent-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx := context.Background()
	listID := "concurrent-test"

	var wg sync.WaitGroup
	numGoroutines := 5
	tasksPerGoroutine := 5

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

func TestFileTaskStoreUnblock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-unblock-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx := context.Background()

	// Create blocker.
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

func TestFileTaskStoreEventTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskstore-events-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewFileTaskStore(tmpDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, _ := store.Subscribe(ctx, "event-test")

	// Test create event.
	id, _ := store.Create(ctx, "event-test", TaskListItem{Subject: "Test", Description: "D"})
	event := <-ch
	if event.Type != TaskEventCreated {
		t.Errorf("event.Type = %v, want created", event.Type)
	}

	// Test claim event.
	store.Update(ctx, "event-test", id, TaskUpdateInput{
		TaskID: id,
		Owner:  "agent-1",
	})
	event = <-ch
	if event.Type != TaskEventClaimed {
		t.Errorf("event.Type = %v, want claimed", event.Type)
	}

	// Test complete event.
	store.Update(ctx, "event-test", id, TaskUpdateInput{
		TaskID: id,
		Status: TaskListStatusCompleted,
	})
	event = <-ch
	if event.Type != TaskEventCompleted {
		t.Errorf("event.Type = %v, want completed", event.Type)
	}

	// Test delete event.
	store.Delete(ctx, "event-test", id)
	event = <-ch
	if event.Type != TaskEventDeleted {
		t.Errorf("event.Type = %v, want deleted", event.Type)
	}
}
