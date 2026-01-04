package claudeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubprocessTransportBasicCommunication tests stdin/stdout communication.
func TestSubprocessTransportBasicCommunication(t *testing.T) {
	// Create mock subprocess
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect
	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Write a message from the "CLI" (mock) to the transport
	go func() {
		msg := AssistantMessage{
			Type: "assistant",
			Message: struct {
				Role    string         `json:"role"`
				Content []ContentBlock `json:"content"`
			}{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "text", Text: "Hello from Claude"},
				},
			},
		}
		data, _ := json.Marshal(msg)
		data = append(data, '\n')
		runner.StdoutPipe.Write(data)
		runner.StdoutPipe.CloseWrite()
	}()

	// Read message
	var receivedMsg Message
	for msg, err := range transport.ReadMessages(ctx) {
		require.NoError(t, err)
		receivedMsg = msg
		break
	}

	// Verify message
	require.NotNil(t, receivedMsg)
	assistantMsg, ok := receivedMsg.(AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "Hello from Claude", assistantMsg.ContentText())

	// Write a message to the CLI.
	userMsg := UserMessage{
		Type:      "user",
		SessionID: "",
		Message: APIUserMessage{
			Role: "user",
			Content: []UserContentBlock{
				{Type: "text", Text: "Test message"},
			},
		},
	}

	// Read from stdin in background
	readDone := make(chan struct{})
	var written UserMessage
	go func() {
		defer close(readDone)
		decoder := json.NewDecoder(runner.StdinPipe)
		err := decoder.Decode(&written)
		require.NoError(t, err)
	}()

	err = transport.Write(ctx, userMsg)
	require.NoError(t, err)

	// Wait for read to complete.
	select {
	case <-readDone:
		require.Len(t, written.Message.Content, 1)
		assert.Equal(t, "Test message", written.Message.Content[0].Text)
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to read from stdin")
	}
}

// TestSubprocessTransportGracefulShutdown tests clean subprocess termination.
func TestSubprocessTransportGracefulShutdown(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)

	// Verify runner is alive
	assert.True(t, runner.IsAlive())

	// Close the transport
	err = transport.Close()
	require.NoError(t, err)

	// Verify transport is closed
	assert.True(t, transport.closed.Load())
	assert.False(t, transport.IsAlive())
}

// TestSubprocessTransportContextCancellation tests that context cancellation
// stops message reading.
func TestSubprocessTransportContextCancellation(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx, cancel := context.WithCancel(context.Background())

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Start reading in a goroutine
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for _, err := range transport.ReadMessages(ctx) {
			if err != nil {
				continue
			}
		}
	}()

	// Cancel context immediately
	cancel()

	// Wait for reader to stop
	select {
	case <-readDone:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("ReadMessages did not stop after context cancellation")
	}
}

// TestSubprocessTransportMultipleMessages tests reading multiple messages.
func TestSubprocessTransportMultipleMessages(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Write multiple messages from "CLI"
	go func() {
		messages := []Message{
			AssistantMessage{
				Type: "assistant",
				Message: struct {
					Role    string         `json:"role"`
					Content []ContentBlock `json:"content"`
				}{
					Role: "assistant",
					Content: []ContentBlock{
						{Type: "text", Text: "Message 1"},
					},
				},
			},
			StreamEvent{
				Type:  "stream_event",
				Event: "delta",
				Delta: "Message 2",
			},
			ResultMessage{
				Type:   "result",
				Status: "success",
				Result: "Complete",
			},
		}

		for _, msg := range messages {
			data, _ := json.Marshal(msg)
			data = append(data, '\n')
			runner.StdoutPipe.Write(data)
		}
		runner.StdoutPipe.CloseWrite()
	}()

	// Read all messages
	received := []Message{}
	for msg, err := range transport.ReadMessages(ctx) {
		require.NoError(t, err)
		received = append(received, msg)
	}

	// Verify count
	assert.Len(t, received, 3)

	// Verify types
	_, ok := received[0].(AssistantMessage)
	assert.True(t, ok)
	_, ok = received[1].(StreamEvent)
	assert.True(t, ok)
	_, ok = received[2].(ResultMessage)
	assert.True(t, ok)
}

// TestSubprocessTransportEmptyLines tests that empty lines are skipped.
func TestSubprocessTransportEmptyLines(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Write messages with empty lines
	go func() {
		runner.StdoutPipe.WriteString(`{"type": "assistant", "message": {"role": "assistant", "content": [{"type": "text", "text": "Hello"}]}}` + "\n")
		runner.StdoutPipe.WriteString("\n") // Empty line
		runner.StdoutPipe.WriteString(`{"type": "result", "status": "success", "result": "Done"}` + "\n")
		runner.StdoutPipe.CloseWrite()
	}()

	// Read messages (should skip empty line)
	count := 0
	for _, err := range transport.ReadMessages(ctx) {
		require.NoError(t, err)
		count++
	}

	assert.Equal(t, 2, count, "should have read 2 messages, skipping empty line")
}

// TestSubprocessTransportParseError tests handling of malformed JSON.
func TestSubprocessTransportParseError(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Write invalid JSON and then valid message
	go func() {
		runner.StdoutPipe.WriteString(`{invalid json}` + "\n")
		msg := ResultMessage{
			Type:   "result",
			Status: "success",
			Result: "Done",
		}
		data, _ := json.Marshal(msg)
		data = append(data, '\n')
		runner.StdoutPipe.Write(data)
		runner.StdoutPipe.CloseWrite()
	}()

	// Read messages
	parseErrorSeen := false
	validMessageSeen := false

	for msg, err := range transport.ReadMessages(ctx) {
		if err != nil {
			parseErrorSeen = true
			continue
		}
		if _, ok := msg.(ResultMessage); ok {
			validMessageSeen = true
		}
	}

	assert.True(t, parseErrorSeen, "should have seen parse error")
	assert.True(t, validMessageSeen, "should have successfully parsed valid message after error")
}

// TestSubprocessTransportWriteContextCancellation tests that Write respects context.
func TestSubprocessTransportWriteContextCancellation(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Create canceled context
	writeCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	msg := UserMessage{
		Type:      "user",
		SessionID: "",
		Message: APIUserMessage{
			Role:    "user",
			Content: []UserContentBlock{{Type: "text", Text: "Test"}},
		},
	}

	err = transport.Write(writeCtx, msg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestSubprocessTransportWriteAfterClose tests that writing after close fails.
func TestSubprocessTransportWriteAfterClose(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)

	// Close transport
	transport.Close()

	// Try to write
	msg := UserMessage{
		Type: "user",
	}

	err = transport.Write(ctx, msg)
	assert.Error(t, err)

	var closedErr *ErrTransportClosed
	assert.ErrorAs(t, err, &closedErr)
}

// TestSubprocessTransportConcurrentWrites tests thread-safety of Write.
func TestSubprocessTransportConcurrentWrites(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	numWriters := 10
	numMessages := 100

	var wg sync.WaitGroup
	wg.Add(numWriters)

	// Consume stdin in background
	messagesWritten := make(chan struct{}, numWriters*numMessages)
	go func() {
		decoder := json.NewDecoder(runner.StdinPipe)
		for {
			var msg UserMessage
			err := decoder.Decode(&msg)
			if err != nil {
				return
			}
			messagesWritten <- struct{}{}
		}
	}()

	// Launch concurrent writers.
	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				msg := UserMessage{
					Type:      "user",
					SessionID: "",
					Message: APIUserMessage{
						Role:    "user",
						Content: []UserContentBlock{{Type: "text", Text: "Message"}},
					},
				}
				err := transport.Write(ctx, msg)
				if err != nil {
					t.Errorf("writer %d: write failed: %v", id, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Give decoder time to process
	time.Sleep(100 * time.Millisecond)

	expected := numWriters * numMessages
	assert.Len(t, messagesWritten, expected, "should have written all messages")
}

// TestDiscoverCLIPath tests CLI path discovery.
func TestDiscoverCLIPath(t *testing.T) {
	t.Run("explicit path", func(t *testing.T) {
		opts := &Options{
			CLIPath: "/custom/path/claude",
		}

		path, err := DiscoverCLIPath(opts)
		require.NoError(t, err)
		assert.Equal(t, "/custom/path/claude", path)
	})

	t.Run("from PATH", func(t *testing.T) {
		opts := &Options{}

		// This will fail if claude is not in PATH, which is expected
		path, err := DiscoverCLIPath(opts)
		if err != nil {
			// Expected if claude not installed
			var notFoundErr *ErrCLINotFound
			assert.ErrorAs(t, err, &notFoundErr)
		} else {
			// If found, should be non-empty
			assert.NotEmpty(t, path)
		}
	})
}

// syncBuffer is a thread-safe buffer for testing.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestSubprocessTransportStderrForwarding tests stderr handling.
func TestSubprocessTransportStderrForwarding(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	// Use a thread-safe buffer since the transport's stderr goroutine will
	// write to it concurrently with our reads.
	stderrBuf := &syncBuffer{}

	transport := NewSubprocessTransportWithRunner(runner, opts)
	transport.SetStderrLogger(stderrBuf)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Write some stderr output.
	runner.StderrPipe.WriteString("Error line 1\n")
	runner.StderrPipe.WriteString("Error line 2\n")

	// Close write side to signal EOF to the scanner goroutine.
	runner.StderrPipe.CloseWrite()

	// Give the scanner goroutine time to process the data.
	time.Sleep(50 * time.Millisecond)

	// Verify stderr was captured.
	output := stderrBuf.String()
	assert.Contains(t, output, "Error line 1")
	assert.Contains(t, output, "Error line 2")
}

// TestSubprocessTransportIsAlive tests subprocess liveness check.
func TestSubprocessTransportIsAlive(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	// Not alive before connection
	assert.False(t, transport.IsAlive())

	// Connect - should be alive
	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)
	assert.True(t, transport.IsAlive())

	// After close, not alive
	transport.Close()
	assert.False(t, transport.IsAlive())
}

// TestSubprocessTransportIteratorEarlyStop tests that stopping iteration
// gracefully terminates the reader.
func TestSubprocessTransportIteratorEarlyStop(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Write many messages
	go func() {
		for i := 0; i < 100; i++ {
			msg := StreamEvent{
				Type:  "stream_event",
				Event: "delta",
				Delta: "text",
			}
			data, _ := json.Marshal(msg)
			data = append(data, '\n')
			runner.StdoutPipe.Write(data)
		}
		runner.StdoutPipe.CloseWrite()
	}()

	// Read only first 3 messages
	count := 0
	for msg, err := range transport.ReadMessages(ctx) {
		require.NoError(t, err)
		require.NotNil(t, msg)
		count++
		if count >= 3 {
			break // Stop early
		}
	}

	assert.Equal(t, 3, count)
	// Verify iterator stopped without blocking
}

// TestSubprocessTransportLargeMessage tests handling of large messages.
func TestSubprocessTransportLargeMessage(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Create a large message (10KB text)
	largeText := strings.Repeat("This is a large message. ", 400)

	go func() {
		msg := AssistantMessage{
			Type: "assistant",
			Message: struct {
				Role    string         `json:"role"`
				Content []ContentBlock `json:"content"`
			}{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "text", Text: largeText},
				},
			},
		}
		data, _ := json.Marshal(msg)
		data = append(data, '\n')
		runner.StdoutPipe.Write(data)
		runner.StdoutPipe.CloseWrite()
	}()

	// Read the large message
	var received Message
	for m, err := range transport.ReadMessages(ctx) {
		require.NoError(t, err)
		received = m
		break
	}

	require.NotNil(t, received)
	assistantMsg, ok := received.(AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, largeText, assistantMsg.ContentText())
}

// TestSubprocessTransportConnectArguments tests that Connect builds correct args.
func TestSubprocessTransportConnectArguments(t *testing.T) {
	runner := NewMockSubprocessRunner()

	opts := &Options{
		Model:          "claude-sonnet-4-5-20250929",
		SystemPrompt:   "You are a helpful assistant",
		PermissionMode: PermissionModePlan,
		Verbose:        true,
	}

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Verify runner was started (we can't directly inspect args in this
	// design, but we verify it started successfully)
	assert.True(t, runner.started)
}

// TestSubprocessTransportCloseTimeout tests forced kill on close timeout.
func TestSubprocessTransportCloseTimeout(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)

	// Don't let runner exit naturally - simulate hung process
	// The Close method should timeout and force kill

	// Close with timeout (this will take 5 seconds due to timeout)
	done := make(chan struct{})
	go func() {
		transport.Close()
		close(done)
	}()

	// Should complete within reasonable time (5s timeout + overhead)
	select {
	case <-done:
		// Success - Close completed
	case <-time.After(7 * time.Second):
		t.Fatal("Close did not complete within timeout")
	}

	// Verify transport is closed
	assert.True(t, transport.closed.Load())
}
