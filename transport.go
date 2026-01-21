package claudeagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SubprocessTransport manages the Claude Code CLI subprocess lifecycle
// and handles stdin/stdout communication.
//
// The transport spawns the CLI with appropriate arguments, pipes stdin/stdout,
// and provides methods for sending/receiving JSON messages. Graceful shutdown
// is handled via context cancellation.
// writerRef wraps an io.Writer for atomic storage.
type writerRef struct {
	w io.Writer
}

type SubprocessTransport struct {
	runner    SubprocessRunner
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	scanner   *bufio.Scanner
	closed    atomic.Bool
	options   *Options
	mu        sync.Mutex
	errLogger atomic.Pointer[writerRef]
}

// NewSubprocessTransport creates a new transport for the Claude CLI.
//
// The CLI path is discovered from options or PATH. The transport is not
// connected until Connect() is called.
func NewSubprocessTransport(options *Options) (*SubprocessTransport, error) {
	cliPath, err := DiscoverCLIPath(options)
	if err != nil {
		return nil, err
	}

	runner := NewLocalSubprocessRunner(cliPath)

	t := &SubprocessTransport{
		runner:  runner,
		options: options,
	}
	t.errLogger.Store(&writerRef{w: io.Discard})
	return t, nil
}

// NewSubprocessTransportWithRunner creates a transport with a custom subprocess runner.
//
// This is primarily useful for testing with mock runners.
func NewSubprocessTransportWithRunner(
	runner SubprocessRunner,
	options *Options,
) *SubprocessTransport {
	t := &SubprocessTransport{
		runner:  runner,
		options: options,
	}
	t.errLogger.Store(&writerRef{w: io.Discard})
	return t
}

// SetStderrLogger sets where to send Claude CLI stderr output.
//
// By default, stderr is discarded. Set to os.Stderr or a logger for debugging.
// The provided io.Writer should be thread-safe as it will be written to from
// a background goroutine.
func (t *SubprocessTransport) SetStderrLogger(w io.Writer) {
	t.errLogger.Store(&writerRef{w: w})
}

// Connect spawns the Claude CLI subprocess and establishes communication.
//
// The CLI is started with the following arguments:
// - --output-format stream-json: Line-delimited JSON output
// - --model: The Claude model to use
// - --system-prompt: System instructions
// - --permission-mode: Permission mode
// - --verbose: Debug logging (if enabled)
// - --resume: Resume an existing session by ID (if SessionOptions.Resume is set)
// - --fork-session: Fork to a new session ID when resuming (if SessionOptions.ForkSession is set)
//
// Environment variables are set for:
// - ANTHROPIC_API_KEY: API authentication
// - CLAUDE_CODE_ENTRYPOINT: "sdk-go"
// - CLAUDE_AGENT_SDK_VERSION: SDK version
func (t *SubprocessTransport) Connect(ctx context.Context) error {
	if t.closed.Load() {
		return &ErrTransportClosed{}
	}

	// Build CLI arguments matching TypeScript SDK.
	// --output-format stream-json returns line-delimited JSON responses on stdout.
	// --verbose is required when using stream-json output format.
	// --input-format stream-json allows sending JSON messages on stdin.
	// Note: -p (print mode) is NOT used - TypeScript SDK doesn't use it.
	args := []string{
		"--output-format", "stream-json",
		"--verbose",
		"--input-format", "stream-json",
	}

	if t.options.Model != "" {
		args = append(args, "--model", t.options.Model)
	}

	if t.options.SystemPrompt != "" {
		args = append(args, "--system-prompt", t.options.SystemPrompt)
	}

	if t.options.PermissionMode != "" {
		args = append(args, "--permission-mode", string(t.options.PermissionMode))
	}

	// Add permission bypass flags if configured.
	if t.options.AllowDangerouslySkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	// Route permission prompts through SDK control channel if callback is set.
	if t.options.CanUseTool != nil {
		args = append(args, "--permission-prompt-tool", "stdio")
	}

	// Note: --verbose is already added above (required for stream-json).

	// Add settings sources for Skills
	if t.options.SkillsConfig.EnableSkills && len(t.options.SkillsConfig.SettingSources) > 0 {
		// --setting-sources takes a comma-separated list
		args = append(args, "--setting-sources", strings.Join(t.options.SkillsConfig.SettingSources, ","))
	}

	// Add MCP server configurations.
	// The CLI uses --mcp-config which takes JSON configuration.
	for name, config := range t.options.MCPServers {
		mcpConfig := map[string]interface{}{
			"command": config.Command,
		}
		if len(config.Args) > 0 {
			mcpConfig["args"] = config.Args
		}
		if len(config.Env) > 0 {
			mcpConfig["env"] = config.Env
		}

		// Wrap in outer object with server name as key.
		wrapper := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				name: mcpConfig,
			},
		}

		jsonBytes, err := json.Marshal(wrapper)
		if err == nil {
			args = append(args, "--mcp-config", string(jsonBytes))
		}
	}

	// Add strict MCP config flag if set.
	if t.options.StrictMCPConfig {
		args = append(args, "--strict-mcp-config")
	}

	// Add no-session-persistence flag if set.
	if t.options.NoSessionPersistence {
		args = append(args, "--no-session-persistence")
	}

	// Add session resume flag if set.
	if t.options.SessionOptions.Resume != "" {
		args = append(args, "--resume", t.options.SessionOptions.Resume)
	}

	// Add fork-session flag if set (used with --resume or --continue).
	if t.options.SessionOptions.ForkSession {
		args = append(args, "--fork-session")
	}

	// Build environment - start with current process env, then overlay options.
	env := os.Environ()
	for k, v := range t.options.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	// Add SDK markers.
	env = append(env,
		"CLAUDE_CODE_ENTRYPOINT=sdk-go",
		"CLAUDE_AGENT_SDK_VERSION=0.1.0",
	)

	// Set custom config directory for isolation if specified.
	if t.options.ConfigDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+t.options.ConfigDir)
	}

	// Start subprocess via runner with working directory.
	stdin, stdout, stderr, err := t.runner.Start(ctx, args, env, t.options.Cwd)
	if err != nil {
		return &ErrSubprocessFailed{Cause: err}
	}

	t.stdin = stdin
	t.stdout = stdout
	t.stderr = stderr
	t.scanner = bufio.NewScanner(stdout)

	// Forward stderr to logger.
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if ref := t.errLogger.Load(); ref != nil && ref.w != nil {
				fmt.Fprintln(ref.w, scanner.Text())
			}
		}
	}()

	return nil
}

// Write sends a JSON message to the CLI stdin.
//
// Messages are serialized to JSON and written as a single line followed by
// a newline. Write operations are serialized via a mutex to prevent
// interleaving.
func (t *SubprocessTransport) Write(ctx context.Context, msg Message) error {
	if t.closed.Load() {
		return &ErrTransportClosed{}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Serialize to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write as a single line
	data = append(data, '\n')

	// Write with context awareness
	done := make(chan error, 1)
	go func() {
		_, err := t.stdin.Write(data)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// ReadMessages returns an iterator over messages read from CLI stdout.
//
// Messages are read line-by-line, parsed as JSON, and yielded to the iterator.
// The iterator stops when the CLI terminates or the context is canceled.
// Parse errors are yielded as errors in the Seq2 iterator.
func (t *SubprocessTransport) ReadMessages(ctx context.Context) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		// Use the scanner created in Connect(). The scanner buffers data from
		// stdout, so we MUST use the same scanner instance - creating a new one
		// would miss any data already buffered by the original scanner.
		for {
			// Check context cancellation.
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Read next line using the pre-created scanner.
			if !t.scanner.Scan() {
				// EOF or error - subprocess likely exited.
				if err := t.scanner.Err(); err != nil {
					yield(nil, fmt.Errorf("scanner error: %w", err))
				}
				return
			}

			line := t.scanner.Bytes()
			if len(line) == 0 {
				continue // Skip empty lines.
			}

			// Parse message.
			msg, err := ParseMessage(line)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// Yield message.
			if !yield(msg, nil) {
				return
			}
		}
	}
}

// Close terminates the CLI subprocess and cleans up resources.
//
// Close attempts a graceful shutdown by closing stdin, which signals the
// CLI to exit. If the process doesn't exit within a timeout, it is killed.
func (t *SubprocessTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Close stdin to signal termination
	if t.stdin != nil {
		t.stdin.Close()
	}

	// Wait for process to exit with timeout
	if t.runner != nil {
		done := make(chan error, 1)
		go func() {
			done <- t.runner.Wait()
		}()

		// Wait with timeout
		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(5 * time.Second):
			// Timeout - force kill
			_ = t.runner.Kill()
		}
	}

	// Close remaining pipes
	if t.stdout != nil {
		t.stdout.Close()
	}
	if t.stderr != nil {
		t.stderr.Close()
	}

	return nil
}

// IsAlive returns true if the subprocess is still running.
func (t *SubprocessTransport) IsAlive() bool {
	if t.closed.Load() {
		return false
	}
	if t.runner == nil {
		return false
	}
	return t.runner.IsAlive()
}
