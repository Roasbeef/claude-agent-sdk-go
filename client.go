package claudeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sync"
	"time"
)

// Client is the high-level API for interacting with Claude Code CLI.
//
// Client manages the subprocess transport, control protocol, and provides
// ergonomic methods for querying and streaming interactions. It uses Go 1.23+
// iter.Seq for streaming message iteration.
type Client struct {
	options   Options
	transport *SubprocessTransport
	protocol  *Protocol
	skills    []Skill
	mu        sync.Mutex
	connected bool

	// Message routing.
	msgCh     chan Message
	msgCtx    context.Context
	msgCancel context.CancelFunc
}

// NewClient creates a new Claude agent client with the given options.
//
// The client is not connected until Connect() is called. Options are validated
// and merged with defaults.
//
// Example:
//
//	client, err := claudeagent.NewClient(
//	    claudeagent.WithSystemPrompt("You are a helpful assistant"),
//	    claudeagent.WithModel("claude-sonnet-4-5-20250929"),
//	)
func NewClient(opts ...Option) (*Client, error) {
	// Start with defaults
	options := DefaultOptions()

	// Apply options
	for _, opt := range opts {
		opt(&options)
	}

	// Validate configuration
	if err := validateOptions(&options); err != nil {
		return nil, err
	}

	client := &Client{
		options: options,
	}

	// Load Skills if enabled
	if options.SkillsConfig.EnableSkills {
		loader := NewSkillLoader(
			options.SkillsConfig.UserSkillsDir,
			options.SkillsConfig.ProjectSkillsDir,
		)
		skills, err := loader.Load()
		if err != nil {
			// Log warning but continue (Skills loading is not critical)
			// In production, use structured logging here
			_ = err
		}
		client.skills = skills
	}

	return client, nil
}

// Connect establishes connection to the Claude CLI subprocess.
//
// This spawns the CLI process and sets up communication pipes.
// Connect must be called before Query or Stream.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil // Already connected
	}

	// Create transport.
	transport, err := NewSubprocessTransport(&c.options)
	if err != nil {
		return err
	}
	c.transport = transport

	// Connect transport.
	if err := transport.Connect(ctx); err != nil {
		return err
	}

	// Create protocol handler.
	c.protocol = NewProtocol(transport, &c.options)

	// Create message channel for routing.
	c.msgCh = make(chan Message, 64)
	c.msgCtx, c.msgCancel = context.WithCancel(context.Background())

	// Start message pump that routes all messages.
	go c.messagePump()

	// Small delay to ensure message pump is ready to receive.
	time.Sleep(50 * time.Millisecond)

	// Initialize the SDK control protocol.
	if err := c.protocol.Initialize(ctx); err != nil {
		c.msgCancel()
		transport.Close()
		return fmt.Errorf("failed to initialize: %w", err)
	}

	c.connected = true
	return nil
}

// messagePump reads from transport and routes messages.
func (c *Client) messagePump() {
	defer close(c.msgCh)
	for msg, err := range c.transport.ReadMessages(c.msgCtx) {
		if err != nil {
			continue
		}
		// Route control messages to protocol handler.
		if isControlMessage(msg) {
			_ = c.protocol.HandleControlMessage(c.msgCtx, msg)
			continue
		}
		// Send non-control messages to consumer channel.
		select {
		case c.msgCh <- msg:
		case <-c.msgCtx.Done():
			return
		}
	}
}

// Query performs a one-shot query and returns an iterator over response messages.
//
// The iterator yields messages as they arrive from Claude, including:
// - AssistantMessage: Text responses and tool requests
// - QuestionMessage: Questions from Claude (call Respond() to answer)
// - TodoUpdateMessage: Task tracking updates
// - SubagentResultMessage: Subagent outcomes
// - ResultMessage: Final completion status
//
// When Claude invokes the AskUserQuestion tool:
// - If WithAskUserQuestionHandler is configured, questions are handled automatically
// - Otherwise, a QuestionMessage is yielded. Call its Respond() method to answer.
//
// The iterator stops when the result message is received or the context is canceled.
//
// Example:
//
//	for msg := range client.Query(ctx, "Help me configure the project") {
//	    switch m := msg.(type) {
//	    case QuestionMessage:
//	        fmt.Println("Claude asks:", m.Questions[0].Question)
//	        m.Respond(m.Answer(0, "Yes"))
//	    case AssistantMessage:
//	        fmt.Println(m.ContentText())
//	    case ResultMessage:
//	        fmt.Printf("Done: %s\n", m.Status)
//	    }
//	}
func (c *Client) Query(ctx context.Context, prompt string) iter.Seq[Message] {
	return func(yield func(Message) bool) {
		// Ensure connected.
		if !c.connected {
			if err := c.Connect(ctx); err != nil {
				return
			}
		}

		// Send user message in TypeScript SDK format.
		userMsg := UserMessage{
			Type:      "user",
			SessionID: c.options.SessionOptions.SessionID,
			Message: APIUserMessage{
				Role: "user",
				Content: []UserContentBlock{
					{Type: "text", Text: prompt},
				},
			},
			ParentToolUseID: nil,
		}

		if err := c.protocol.SendMessage(ctx, userMsg); err != nil {
			return
		}

		// Read messages from channel until result.
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-c.msgCh:
				if !ok {
					return // Channel closed.
				}

				// Check for AskUserQuestion tool calls.
				if c.options.AskUserQuestionHandler != nil {
					// Handler configured - use callback API.
					if handled := c.handleAskUserQuestion(ctx, msg); handled {
						continue
					}
				} else {
					// No handler - yield QuestionMessage if present.
					if questionMsg := c.extractQuestionMessage(ctx, msg); questionMsg != nil {
						if !yield(*questionMsg) {
							return
						}
						continue
					}
				}

				// Yield message to consumer.
				if !yield(msg) {
					return
				}

				// Stop on result message.
				if _, ok := msg.(ResultMessage); ok {
					return
				}
			}
		}
	}
}

// extractQuestionMessage checks if the message contains an AskUserQuestion tool call
// and returns a QuestionMessage if found. Returns nil if no question is present.
//
// The responder uses context.Background() to ensure the answer can be sent even if
// the original query context has been cancelled. This allows users to respond to
// questions asynchronously without worrying about context lifecycle.
func (c *Client) extractQuestionMessage(_ context.Context, msg Message) *QuestionMessage {
	assistant, ok := msg.(AssistantMessage)
	if !ok {
		return nil
	}

	for _, block := range assistant.Message.Content {
		if block.Type == "tool_use" && block.Name == "AskUserQuestion" {
			// Parse the question input.
			var input AskUserQuestionInput
			if err := json.Unmarshal(block.Input, &input); err != nil {
				continue
			}

			// Capture for closure.
			toolUseID := block.ID

			// Create QuestionMessage with embedded QuestionSet.
			// Use context.Background() for responder to allow async responses
			// even after the original query context is cancelled.
			return &QuestionMessage{
				QuestionSet: QuestionSet{
					ToolUseID: toolUseID,
					Questions: input.Questions,
					SessionID: c.options.SessionOptions.SessionID,
				},
				responder: func(answers Answers) error {
					return c.sendToolResult(context.Background(), toolUseID, answers)
				},
			}
		}
	}

	return nil
}

// handleAskUserQuestion checks if the message contains an AskUserQuestion tool call
// and handles it using the configured handler. Returns true if the message was handled.
func (c *Client) handleAskUserQuestion(ctx context.Context, msg Message) bool {
	assistant, ok := msg.(AssistantMessage)
	if !ok {
		return false
	}

	for _, block := range assistant.Message.Content {
		if block.Type == "tool_use" && block.Name == "AskUserQuestion" {
			// Parse the question input.
			var input AskUserQuestionInput
			if err := json.Unmarshal(block.Input, &input); err != nil {
				continue
			}

			// Create QuestionSet.
			qs := QuestionSet{
				ToolUseID: block.ID,
				Questions: input.Questions,
				SessionID: c.options.SessionOptions.SessionID,
			}

			// Call the handler.
			answers, err := c.options.AskUserQuestionHandler(ctx, qs)
			if err != nil {
				// Send error back to Claude so conversation doesn't hang.
				c.sendToolError(ctx, block.ID, fmt.Sprintf("question handler error: %v", err))
				return true
			}

			// Send the tool result.
			if err := c.sendToolResult(ctx, block.ID, answers); err != nil {
				// Send error back to Claude so conversation doesn't hang.
				c.sendToolError(ctx, block.ID, fmt.Sprintf("failed to send answer: %v", err))
				return true
			}

			return true
		}
	}

	return false
}

// Questions returns an iterator for interactive Q&A sessions.
//
// This method combines message streaming with question handling. When Claude
// invokes the AskUserQuestion tool, the iterator yields a QuestionSet and an
// AnswerFunc. Call the AnswerFunc with your answers to continue the conversation.
//
// The iterator yields (QuestionSet, AnswerFunc) pairs. Regular messages are
// processed internally but not yielded. Use Query() or Stream() if you need
// access to all messages.
//
// Example:
//
//	for qs, answer := range client.Questions(ctx, "Help me configure the project") {
//	    fmt.Printf("Claude asks: %s\n", qs.Questions[0].Question)
//
//	    // Use helper methods to construct answers
//	    if err := answer(qs.Answer(0, "Yes")); err != nil {
//	        log.Fatal(err)
//	    }
//	}
func (c *Client) Questions(ctx context.Context, prompt string) iter.Seq2[QuestionSet, AnswerFunc] {
	return func(yield func(QuestionSet, AnswerFunc) bool) {
		// Ensure connected.
		if !c.connected {
			if err := c.Connect(ctx); err != nil {
				return
			}
		}

		// Send user message.
		userMsg := UserMessage{
			Type:      "user",
			SessionID: c.options.SessionOptions.SessionID,
			Message: APIUserMessage{
				Role: "user",
				Content: []UserContentBlock{
					{Type: "text", Text: prompt},
				},
			},
			ParentToolUseID: nil,
		}

		if err := c.protocol.SendMessage(ctx, userMsg); err != nil {
			return
		}

		// Read messages from channel until result.
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-c.msgCh:
				if !ok {
					return // Channel closed.
				}

				// Check for AskUserQuestion tool calls in assistant messages.
				if assistant, ok := msg.(AssistantMessage); ok {
					for _, block := range assistant.Message.Content {
						if block.Type == "tool_use" && block.Name == "AskUserQuestion" {
							// Parse the question input.
							var input AskUserQuestionInput
							if err := json.Unmarshal(block.Input, &input); err != nil {
								continue
							}

							// Create QuestionSet.
							qs := QuestionSet{
								ToolUseID: block.ID,
								Questions: input.Questions,
								SessionID: c.options.SessionOptions.SessionID,
							}

							// Create answer function that sends tool result.
							toolUseID := block.ID
							answerFunc := func(answers Answers) error {
								return c.sendToolResult(ctx, toolUseID, answers)
							}

							// Yield to consumer.
							if !yield(qs, answerFunc) {
								return
							}
						}
					}
				}

				// Stop on result message.
				if _, ok := msg.(ResultMessage); ok {
					return
				}
			}
		}
	}
}

// sendToolResult sends a tool result back to Claude.
func (c *Client) sendToolResult(ctx context.Context, toolUseID string, answers Answers) error {
	// Format answers in the expected structure.
	result := map[string]interface{}{
		"answers": answers,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal answers: %w", err)
	}

	// Send as a user message with tool result.
	msg := UserMessage{
		Type:            "user",
		SessionID:       c.options.SessionOptions.SessionID,
		ParentToolUseID: &toolUseID,
		ToolUseResult:   json.RawMessage(resultJSON),
		Message: APIUserMessage{
			Role: "user",
			Content: []UserContentBlock{
				{Type: "tool_result", Text: string(resultJSON)},
			},
		},
	}

	return c.protocol.SendMessage(ctx, msg)
}

// sendToolError sends an error result back to Claude for a tool use.
// This prevents the conversation from hanging when question handling fails.
func (c *Client) sendToolError(ctx context.Context, toolUseID string, errMsg string) error {
	// Format error in the expected structure.
	result := map[string]interface{}{
		"error": errMsg,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal error: %w", err)
	}

	// Send as a user message with tool result indicating error.
	msg := UserMessage{
		Type:            "user",
		SessionID:       c.options.SessionOptions.SessionID,
		ParentToolUseID: &toolUseID,
		ToolUseResult:   json.RawMessage(resultJSON),
		Message: APIUserMessage{
			Role: "user",
			Content: []UserContentBlock{
				{Type: "tool_result", Text: string(resultJSON)},
			},
		},
	}

	return c.protocol.SendMessage(ctx, msg)
}

// Stream returns a bidirectional stream for interactive conversations.
//
// Streams allow multiple rounds of user prompts and assistant responses
// within a single session. Use Send() to submit prompts and range over
// Messages() to receive responses.
//
// Example:
//
//	stream, err := client.Stream(ctx)
//	defer stream.Close()
//
//	stream.Send(ctx, "What's the weather?")
//	for msg := range stream.Messages() {
//	    // Process messages
//	}
func (c *Client) Stream(ctx context.Context) (*Stream, error) {
	// Ensure connected
	if !c.connected {
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
	}

	return &Stream{
		client:    c,
		ctx:       ctx,
		sessionID: c.options.SessionOptions.SessionID,
		sendCh:    make(chan string, 4),
		closeCh:   make(chan struct{}),
	}, nil
}

// Close terminates the Claude CLI subprocess and cleans up resources.
//
// Close should be called when the client is no longer needed. After Close,
// the client cannot be used again.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false

	// Cancel message pump.
	if c.msgCancel != nil {
		c.msgCancel()
	}

	if c.transport != nil {
		return c.transport.Close()
	}

	return nil
}

// ListSkills returns all loaded Skills (user + project).
//
// Skills are loaded during client creation based on SkillsConfig.
// The returned slice is a copy, safe for concurrent access.
func (c *Client) ListSkills() []Skill {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return a copy to avoid external mutation
	result := make([]Skill, len(c.skills))
	copy(result, c.skills)
	return result
}

// GetSkill retrieves a Skill by name.
//
// Returns ErrSkillNotFound if the Skill does not exist.
func (c *Client) GetSkill(name string) (*Skill, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.skills {
		if c.skills[i].Name == name {
			// Return a copy to avoid external mutation
			skill := c.skills[i]
			return &skill, nil
		}
	}

	return nil, &ErrSkillNotFound{Name: name}
}

// ReloadSkills rescans filesystem and reloads all Skills.
//
// This is useful for picking up new Skills or changes to existing Skills
// without restarting the client. Returns ErrSkillsDisabled if Skills are
// disabled in configuration.
func (c *Client) ReloadSkills() error {
	if !c.options.SkillsConfig.EnableSkills {
		return &ErrSkillsDisabled{}
	}

	loader := NewSkillLoader(
		c.options.SkillsConfig.UserSkillsDir,
		c.options.SkillsConfig.ProjectSkillsDir,
	)

	skills, err := loader.Load()
	if err != nil {
		return fmt.Errorf("failed to reload Skills: %w", err)
	}

	c.mu.Lock()
	c.skills = skills
	c.mu.Unlock()

	return nil
}

// ValidateSkill validates a Skill at the given path without loading it.
//
// This is useful for checking Skill validity before adding it to a Skills
// directory. The path should point to a SKILL.md file.
func (c *Client) ValidateSkill(path string) error {
	loader := NewSkillLoader("", "")
	return loader.ValidateSKILLMd(path)
}

// Stream represents a bidirectional conversation stream.
//
// Streams maintain session state and allow multiple rounds of interaction.
// They must be closed when done to free resources.
type Stream struct {
	client    *Client
	ctx       context.Context
	sessionID string
	sendCh    chan string
	closeCh   chan struct{}
	closeOnce sync.Once
}

// Send submits a user message to the stream.
//
// Messages are queued and sent asynchronously. The response will appear
// in the Messages() iterator.
func (s *Stream) Send(ctx context.Context, prompt string) error {
	select {
	case <-s.closeCh:
		return &ErrTransportClosed{}
	case <-ctx.Done():
		return ctx.Err()
	case s.sendCh <- prompt:
		return nil
	}
}

// Messages returns an iterator over response messages.
//
// The iterator yields all messages from the stream until Close() is called
// or the context is canceled.
//
// Example:
//
//	for msg := range stream.Messages() {
//	    switch m := msg.(type) {
//	    case *AssistantMessage:
//	        fmt.Println(m.ContentText())
//	    case *StreamEvent:
//	        if m.Event == "delta" {
//	            fmt.Print(m.Delta)
//	        }
//	    }
//	}
func (s *Stream) Messages() iter.Seq[Message] {
	return func(yield func(Message) bool) {
		// Start send handler.
		go s.handleSends()

		// Read from the shared message channel.
		for {
			select {
			case <-s.closeCh:
				return
			case <-s.ctx.Done():
				return
			case msg, ok := <-s.client.msgCh:
				if !ok {
					return // Channel closed.
				}

				// Yield message to consumer.
				if !yield(msg) {
					return
				}
			}
		}
	}
}

// handleSends processes queued user messages.
func (s *Stream) handleSends() {
	for {
		select {
		case <-s.closeCh:
			return
		case <-s.ctx.Done():
			return
		case prompt := <-s.sendCh:
			userMsg := UserMessage{
				Type:      "user",
				SessionID: s.sessionID,
				Message: APIUserMessage{
					Role: "user",
					Content: []UserContentBlock{
						{Type: "text", Text: prompt},
					},
				},
				ParentToolUseID: nil,
			}

			if err := s.client.protocol.SendMessage(s.ctx, userMsg); err != nil {
				// Log error but continue.
				continue
			}
		}
	}
}

// Interrupt sends an interrupt signal to stop the current generation.
func (s *Stream) Interrupt(ctx context.Context) error {
	// Send interrupt control message in SDK format.
	req := SDKControlRequest{
		Type:      "control_request",
		RequestID: s.client.protocol.nextRequestID(),
		Request: SDKControlRequestBody{
			Subtype: "interrupt",
		},
	}
	return s.client.transport.Write(ctx, req)
}

// RewindFiles restores files to a checkpoint at the specified user message.
//
// This requires EnableFileCheckpointing to be true in Options.
// The userMessageUUID should be the UUID of a previous user message.
func (s *Stream) RewindFiles(ctx context.Context, userMessageUUID string) error {
	req := SDKControlRequest{
		Type:      "control_request",
		RequestID: s.client.protocol.nextRequestID(),
		Request: SDKControlRequestBody{
			Subtype:       "rewind_files",
			UserMessageID: userMessageUUID,
		},
	}
	return s.client.transport.Write(ctx, req)
}

// SetPermissionMode dynamically changes the permission mode for this session.
func (s *Stream) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	req := SDKControlRequest{
		Type:      "control_request",
		RequestID: s.client.protocol.nextRequestID(),
		Request: SDKControlRequestBody{
			Subtype: "set_permission_mode",
			Mode:    string(mode),
		},
	}
	return s.client.transport.Write(ctx, req)
}

// SetModel dynamically changes the model for this session.
// Pass empty string to reset to default.
func (s *Stream) SetModel(ctx context.Context, model string) error {
	req := SDKControlRequest{
		Type:      "control_request",
		RequestID: s.client.protocol.nextRequestID(),
		Request: SDKControlRequestBody{
			Subtype: "set_model",
			Model:   model,
		},
	}
	return s.client.transport.Write(ctx, req)
}

// SetMaxThinkingTokens dynamically changes the max thinking tokens limit.
// Pass nil to remove the limit.
func (s *Stream) SetMaxThinkingTokens(ctx context.Context, tokens *int) error {
	req := SDKControlRequest{
		Type:      "control_request",
		RequestID: s.client.protocol.nextRequestID(),
		Request: SDKControlRequestBody{
			Subtype:           "set_max_thinking_tokens",
			MaxThinkingTokens: tokens,
		},
	}
	return s.client.transport.Write(ctx, req)
}

// SupportedCommands returns the list of available slash commands.
func (s *Stream) SupportedCommands(ctx context.Context) ([]SlashCommand, error) {
	respCh := s.client.protocol.sendRequest(ctx, "supported_commands", nil)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, &ErrProtocol{Message: resp.Error.Message}
		}
		// Parse response
		commands, ok := resp.Result["commands"].([]interface{})
		if !ok {
			return nil, &ErrProtocol{Message: "invalid commands response"}
		}
		result := make([]SlashCommand, 0, len(commands))
		for _, cmd := range commands {
			cmdMap, ok := cmd.(map[string]interface{})
			if !ok {
				continue
			}
			result = append(result, SlashCommand{
				Name:         getString(cmdMap, "name"),
				Description:  getString(cmdMap, "description"),
				ArgumentHint: getString(cmdMap, "argumentHint"),
			})
		}
		return result, nil
	}
}

// SupportedModels returns the list of available models.
func (s *Stream) SupportedModels(ctx context.Context) ([]ModelInfo, error) {
	respCh := s.client.protocol.sendRequest(ctx, "supported_models", nil)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, &ErrProtocol{Message: resp.Error.Message}
		}
		// Parse response
		models, ok := resp.Result["models"].([]interface{})
		if !ok {
			return nil, &ErrProtocol{Message: "invalid models response"}
		}
		result := make([]ModelInfo, 0, len(models))
		for _, model := range models {
			modelMap, ok := model.(map[string]interface{})
			if !ok {
				continue
			}
			result = append(result, ModelInfo{
				Value:       getString(modelMap, "value"),
				DisplayName: getString(modelMap, "displayName"),
				Description: getString(modelMap, "description"),
			})
		}
		return result, nil
	}
}

// McpServerStatus returns the connection status of all MCP servers.
func (s *Stream) McpServerStatus(ctx context.Context) ([]McpServerStatus, error) {
	respCh := s.client.protocol.sendRequest(ctx, "mcp_server_status", nil)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, &ErrProtocol{Message: resp.Error.Message}
		}
		// Parse response
		servers, ok := resp.Result["servers"].([]interface{})
		if !ok {
			return nil, &ErrProtocol{Message: "invalid mcp_server_status response"}
		}
		result := make([]McpServerStatus, 0, len(servers))
		for _, srv := range servers {
			srvMap, ok := srv.(map[string]interface{})
			if !ok {
				continue
			}
			status := McpServerStatus{
				Name:   getString(srvMap, "name"),
				Status: McpServerState(getString(srvMap, "status")),
			}
			if info, ok := srvMap["serverInfo"].(map[string]interface{}); ok {
				status.ServerInfo = &McpServerInfo{
					Name:    getString(info, "name"),
					Version: getString(info, "version"),
				}
			}
			result = append(result, status)
		}
		return result, nil
	}
}

// AccountInfo returns account information for the current session.
func (s *Stream) AccountInfo(ctx context.Context) (*AccountInfo, error) {
	respCh := s.client.protocol.sendRequest(ctx, "account_info", nil)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, &ErrProtocol{Message: resp.Error.Message}
		}
		// Parse response
		return &AccountInfo{
			Email:            getString(resp.Result, "email"),
			Organization:     getString(resp.Result, "organization"),
			SubscriptionType: getString(resp.Result, "subscriptionType"),
			TokenSource:      getString(resp.Result, "tokenSource"),
			APIKeySource:     getString(resp.Result, "apiKeySource"),
		}, nil
	}
}

// SessionID returns the current session ID.
func (s *Stream) SessionID() string {
	return s.sessionID
}

// Close terminates the stream.
//
// After Close, no more messages can be sent or received on this stream.
// The underlying client connection remains active for other streams.
func (s *Stream) Close() error {
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})
	return nil
}

// validateOptions validates client configuration.
func validateOptions(opts *Options) error {
	// Validate model
	if opts.Model == "" {
		return &ErrInvalidConfiguration{
			Field:  "Model",
			Reason: "model must be specified",
		}
	}

	// Validate permission mode
	validModes := map[PermissionMode]bool{
		PermissionModeDefault:     true,
		PermissionModePlan:        true,
		PermissionModeAcceptEdits: true,
		PermissionModeBypassAll:   true,
	}
	if opts.PermissionMode != "" && !validModes[opts.PermissionMode] {
		return &ErrInvalidConfiguration{
			Field:  "PermissionMode",
			Reason: fmt.Sprintf("invalid permission mode: %s", opts.PermissionMode),
		}
	}

	// Validate session options
	if opts.SessionOptions.Resume != "" && opts.SessionOptions.ForkFrom != "" {
		return &ErrInvalidConfiguration{
			Field:  "SessionOptions",
			Reason: "cannot specify both Resume and ForkFrom",
		}
	}

	return nil
}

// isControlMessage checks if a message is a control protocol message.
func isControlMessage(msg Message) bool {
	switch msg.(type) {
	case ControlRequest, ControlResponse,
		SDKControlRequest, SDKControlResponse, SDKControlCancelRequest,
		KeepAliveMessage:
		return true
	default:
		return false
	}
}
