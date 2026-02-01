//go:build integration && unix

package claudeagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegrationTaskListSDKCreate tests that tasks created via SDK are stored
// correctly and can be read back via a separate TaskManager instance.
//
// Note: Full SDK-to-CLI integration (where Claude's TaskList tool sees SDK-created
// tasks) requires the CLI to be configured with the same CLAUDE_CODE_TASK_LIST_ID.
// This test verifies the file-based storage format is correct.
func TestIntegrationTaskListSDKCreate(t *testing.T) {
	// Use a unique task list ID for this test.
	listID := "integration-test-" + time.Now().Format("20060102-150405")

	// Create a temp directory for task storage to isolate from real tasks.
	tmpDir, err := os.MkdirTemp("", "task-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create task via SDK.
	store, err := NewFileTaskStore(tmpDir)
	require.NoError(t, err)

	tm := NewTaskManagerWithStore(listID, store)
	ctx := context.Background()

	task, err := tm.Create(ctx, "SDK created task", "This task was created by the SDK")
	require.NoError(t, err)
	t.Logf("Created task via SDK: ID=%s, Subject=%s", task.ID, task.Subject)

	// Verify the task file was created in the expected location.
	taskPath := filepath.Join(tmpDir, listID, task.ID+".json")
	_, err = os.Stat(taskPath)
	require.NoError(t, err, "task file should exist at %s", taskPath)
	t.Logf("Task file created at: %s", taskPath)

	// Verify another TaskManager instance can read the task.
	store2, err := NewFileTaskStore(tmpDir)
	require.NoError(t, err)
	tm2 := NewTaskManagerWithStore(listID, store2)

	readTask, err := tm2.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, task.Subject, readTask.Subject)
	assert.Equal(t, task.Description, readTask.Description)
	assert.Equal(t, TaskListStatusPending, readTask.Status)
	t.Logf("Second TaskManager read task: ID=%s, Subject=%s", readTask.ID, readTask.Subject)
}

// TestIntegrationTaskListExportImport tests that tasks can be exported from one
// store and imported into another, enabling backup/restore and migration.
func TestIntegrationTaskListExportImport(t *testing.T) {
	// Use unique task list IDs for this test.
	listID := "integration-export-" + time.Now().Format("20060102-150405")

	// Create two separate temp directories for task storage.
	srcDir, err := os.MkdirTemp("", "task-src-*")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	dstDir, err := os.MkdirTemp("", "task-dst-*")
	require.NoError(t, err)
	defer os.RemoveAll(dstDir)

	ctx := context.Background()

	// Create source store with tasks.
	srcStore, err := NewFileTaskStore(srcDir)
	require.NoError(t, err)
	srcTM := NewTaskManagerWithStore(listID, srcStore)

	// Create several tasks with different states.
	task1, _ := srcTM.Create(ctx, "Export test task 1", "First task")
	task2, _ := srcTM.Create(ctx, "Export test task 2", "Second task")
	task3, _ := srcTM.Create(ctx, "Export test task 3", "Third task")

	srcTM.Claim(ctx, task2.ID, "test-agent")
	srcTM.Complete(ctx, task3.ID)

	t.Logf("Created tasks in source: task1=%s, task2=%s, task3=%s", task1.ID, task2.ID, task3.ID)

	// Export tasks from source (FileTaskStore implements TaskStoreWithExport).
	exported, err := srcStore.Export(ctx, listID)
	require.NoError(t, err)
	require.Len(t, exported, 3)
	t.Logf("Exported %d tasks", len(exported))

	// Import into destination store.
	dstStore, err := NewFileTaskStore(dstDir)
	require.NoError(t, err)

	err = dstStore.Import(ctx, listID, exported, true)
	require.NoError(t, err)
	t.Log("Imported tasks to destination store")

	// Verify destination has the same tasks with same states.
	dstTM := NewTaskManagerWithStore(listID, dstStore)
	tasks, err := dstTM.List(ctx)
	require.NoError(t, err)
	require.Len(t, tasks, 3)

	// Check each task preserved its state.
	for _, task := range tasks {
		t.Logf("Imported task: ID=%s, Subject=%s, Status=%s, Owner=%s",
			task.ID, task.Subject, task.Status, task.Owner)
	}

	// Verify specific states.
	importedTask2, err := dstTM.Get(ctx, task2.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskListStatusInProgress, importedTask2.Status)
	assert.Equal(t, "test-agent", importedTask2.Owner)

	importedTask3, err := dstTM.Get(ctx, task3.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskListStatusCompleted, importedTask3.Status)
}

// TestIntegrationTaskListSharedAccess tests that multiple SDK instances can
// share the same task list.
func TestIntegrationTaskListSharedAccess(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Use a unique task list ID.
	listID := "integration-shared-" + time.Now().Format("20060102-150405")

	// Create a temp directory for task storage.
	tmpDir, err := os.MkdirTemp("", "task-shared-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create shared store.
	store, err := NewFileTaskStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()

	// Create first task manager (simulates first agent).
	tm1 := NewTaskManagerWithStore(listID, store)

	// Create second task manager (simulates second agent).
	tm2 := NewTaskManagerWithStore(listID, store)

	// Agent 1 creates a task.
	task1, err := tm1.Create(ctx, "Task from Agent 1", "Created by first agent")
	require.NoError(t, err)
	t.Logf("Agent 1 created task: ID=%s", task1.ID)

	// Agent 2 should see the task.
	tasks, err := tm2.List(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 1, "Agent 2 should see task from Agent 1")
	assert.Equal(t, "Task from Agent 1", tasks[0].Subject)

	// Agent 2 claims the task.
	err = tm2.Claim(ctx, task1.ID, "agent-2")
	require.NoError(t, err)
	t.Logf("Agent 2 claimed task: ID=%s", task1.ID)

	// Agent 1 should see the claimed status.
	task, err := tm1.Get(ctx, task1.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskListStatusInProgress, task.Status)
	assert.Equal(t, "agent-2", task.Owner)
	t.Logf("Agent 1 sees task claimed by: %s", task.Owner)

	// Agent 2 completes the task.
	err = tm2.Complete(ctx, task1.ID)
	require.NoError(t, err)
	t.Logf("Agent 2 completed task: ID=%s", task1.ID)

	// Agent 1 should see completed status.
	task, err = tm1.Get(ctx, task1.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskListStatusCompleted, task.Status)
	t.Logf("Agent 1 sees task status: %s", task.Status)
}

// TestIntegrationTaskListBlocking tests task blocking and unblocking behavior
// across agents sharing a task list.
func TestIntegrationTaskListBlocking(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Use a unique task list ID.
	listID := "integration-blocking-" + time.Now().Format("20060102-150405")

	// Create a temp directory for task storage.
	tmpDir, err := os.MkdirTemp("", "task-blocking-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create shared store.
	store, err := NewFileTaskStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()

	// Create two task managers.
	tm1 := NewTaskManagerWithStore(listID, store)
	tm2 := NewTaskManagerWithStore(listID, store)

	// Agent 1 creates a prerequisite task.
	prereq, err := tm1.Create(ctx, "Prerequisite task", "Must complete first")
	require.NoError(t, err)
	t.Logf("Created prerequisite: ID=%s", prereq.ID)

	// Agent 1 creates a dependent task that's blocked by prerequisite.
	dependent, err := tm1.Create(ctx, "Dependent task", "Blocked by prerequisite")
	require.NoError(t, err)
	t.Logf("Created dependent: ID=%s", dependent.ID)

	// Set up blocking relationship.
	err = tm1.Update(ctx, dependent.ID, AddBlockedBy(prereq.ID))
	require.NoError(t, err)
	t.Logf("Set dependent blocked by prerequisite")

	// Agent 2 should see the dependent as blocked.
	depTask, err := tm2.Get(ctx, dependent.ID)
	require.NoError(t, err)
	assert.True(t, depTask.IsBlocked(), "dependent task should be blocked")
	assert.Contains(t, depTask.BlockedBy, prereq.ID)
	t.Logf("Agent 2 sees dependent blocked by: %v", depTask.BlockedBy)

	// Agent 2 lists unblocked tasks - should only see prerequisite.
	unblocked, err := tm2.ListUnblocked(ctx)
	require.NoError(t, err)
	assert.Len(t, unblocked, 1, "should have 1 unblocked task")
	assert.Equal(t, prereq.ID, unblocked[0].ID, "only prerequisite should be available")
	t.Logf("Unblocked tasks: %d", len(unblocked))

	// Agent 2 completes the prerequisite.
	err = tm2.Complete(ctx, prereq.ID)
	require.NoError(t, err)
	t.Logf("Agent 2 completed prerequisite")

	// After completion, dependent should be unblocked.
	depTask, err = tm1.Get(ctx, dependent.ID)
	require.NoError(t, err)
	assert.False(t, depTask.IsBlocked(), "dependent should be unblocked after prerequisite completed")
	t.Logf("Dependent task blocked: %v, blockedBy: %v", depTask.IsBlocked(), depTask.BlockedBy)

	// Now both agents should see dependent as available.
	unblocked, err = tm1.ListUnblocked(ctx)
	require.NoError(t, err)
	assert.Len(t, unblocked, 1, "should have 1 unblocked task (dependent)")
	assert.Equal(t, dependent.ID, unblocked[0].ID)
	t.Logf("After completion, unblocked tasks: %d", len(unblocked))
}

// TestIntegrationTaskListSubscribe tests event subscription across agents.
func TestIntegrationTaskListSubscribe(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Use a unique task list ID.
	listID := "integration-subscribe-" + time.Now().Format("20060102-150405")

	// Create a temp directory for task storage.
	tmpDir, err := os.MkdirTemp("", "task-subscribe-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create shared store.
	store, err := NewFileTaskStore(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create two task managers.
	tm1 := NewTaskManagerWithStore(listID, store)
	tm2 := NewTaskManagerWithStore(listID, store)

	// Agent 2 subscribes to events.
	eventCh, err := tm2.Watch(ctx)
	require.NoError(t, err)
	require.NotNil(t, eventCh, "FileTaskStore should support subscriptions")
	t.Log("Agent 2 subscribed to task events")

	// Give subscription time to set up.
	time.Sleep(50 * time.Millisecond)

	// Track received events.
	var receivedEvents []TaskEvent
	done := make(chan struct{})

	go func() {
		defer close(done)
		timeout := time.After(5 * time.Second)
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					return
				}
				receivedEvents = append(receivedEvents, event)
				t.Logf("Received event: type=%s, taskID=%s", event.Type, event.TaskID)
				if len(receivedEvents) >= 3 {
					return
				}
			case <-timeout:
				return
			}
		}
	}()

	// Agent 1 creates a task - should generate "created" event.
	task, err := tm1.Create(ctx, "Event test task", "Testing events")
	require.NoError(t, err)
	t.Logf("Agent 1 created task: ID=%s", task.ID)

	// Agent 1 claims the task - should generate "claimed" event.
	err = tm1.Claim(ctx, task.ID, "agent-1")
	require.NoError(t, err)
	t.Logf("Agent 1 claimed task")

	// Agent 1 completes the task - should generate "completed" event.
	err = tm1.Complete(ctx, task.ID)
	require.NoError(t, err)
	t.Logf("Agent 1 completed task")

	// Wait for events to be received.
	<-done

	// Verify events were received.
	t.Logf("Total events received: %d", len(receivedEvents))
	assert.GreaterOrEqual(t, len(receivedEvents), 1, "should receive at least one event")

	// Check event types.
	eventTypes := make([]TaskEventType, len(receivedEvents))
	for i, e := range receivedEvents {
		eventTypes[i] = e.Type
	}
	t.Logf("Event types: %v", eventTypes)
}

// TestIntegrationTaskListStats tests statistics across shared task lists.
func TestIntegrationTaskListStats(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Use a unique task list ID.
	listID := "integration-stats-" + time.Now().Format("20060102-150405")

	// Create a temp directory for task storage.
	tmpDir, err := os.MkdirTemp("", "task-stats-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create shared store.
	store, err := NewFileTaskStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	tm := NewTaskManagerWithStore(listID, store)

	// Create tasks with various states.
	task1, _ := tm.Create(ctx, "Task 1", "Pending task")
	task2, _ := tm.Create(ctx, "Task 2", "Will be in progress")
	task3, _ := tm.Create(ctx, "Task 3", "Will be completed")
	task4, _ := tm.Create(ctx, "Task 4", "Blocked task")

	// Set up states.
	tm.Claim(ctx, task2.ID, "agent")
	tm.Complete(ctx, task3.ID)
	tm.Update(ctx, task4.ID, AddBlockedBy(task1.ID))

	// Get stats from another manager (simulating another agent).
	tm2 := NewTaskManagerWithStore(listID, store)
	stats, err := tm2.Stats(ctx)
	require.NoError(t, err)

	t.Logf("Stats: total=%d, pending=%d, in_progress=%d, completed=%d, blocked=%d, unblocked=%d",
		stats.Total, stats.Pending, stats.InProgress, stats.Completed, stats.Blocked, stats.Unblocked)

	assert.Equal(t, 4, stats.Total)
	assert.Equal(t, 2, stats.Pending)    // task1 and task4 are pending.
	assert.Equal(t, 1, stats.InProgress) // task2.
	assert.Equal(t, 1, stats.Completed)  // task3.
	assert.Equal(t, 1, stats.Blocked)    // task4.
	assert.Equal(t, 1, stats.Unblocked)  // task1 (pending and not blocked).
}
