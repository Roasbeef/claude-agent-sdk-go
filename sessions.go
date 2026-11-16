package claudeagent

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SDKSessionInfo is metadata returned by ListSessions and GetSessionInfo.
type SDKSessionInfo struct {
	SessionID    string `json:"sessionId"`
	Summary      string `json:"summary"`
	LastModified int64  `json:"lastModified"`
	FileSize     int64  `json:"fileSize,omitempty"`
	CustomTitle  string `json:"customTitle,omitempty"`
	FirstPrompt  string `json:"firstPrompt,omitempty"`
	GitBranch    string `json:"gitBranch,omitempty"`
	Cwd          string `json:"cwd,omitempty"`
	Tag          string `json:"tag,omitempty"`
	CreatedAt    int64  `json:"createdAt,omitempty"`
	// Archived reports whether ArchiveSession marked the session. It is
	// false once UnarchiveSession ran or a prompt was sent in the session
	// after the mark, and nil when the session was never archived
	// (sdk.d.ts v0.3.325 L6428).
	Archived *bool `json:"archived,omitempty"`
}

// SessionMessage is a user, assistant, or optionally system transcript message.
type SessionMessage struct {
	Type            string          `json:"type"`
	UUID            string          `json:"uuid"`
	SessionID       string          `json:"session_id"`
	Message         json.RawMessage `json:"message"`
	ParentToolUseID *string         `json:"parent_tool_use_id"`
	// ParentAgentID is the agentId of the subagent that spawned this
	// subagent, or nil for a depth-1 subagent (spawned by the main loop), the
	// main session itself, or sessions whose metadata lacks the field
	// (sdk.d.ts v0.3.207).
	ParentAgentID *string `json:"parent_agent_id"`
}

// ListSessionsOptions controls ListSessions.
type ListSessionsOptions struct {
	Dir     string
	Limit   int
	Offset  int
	BaseDir string
}

// GetSessionInfoOptions controls GetSessionInfo.
type GetSessionInfoOptions struct {
	Dir     string
	BaseDir string
}

// GetSessionMessagesOptions controls GetSessionMessages.
type GetSessionMessagesOptions struct {
	Dir                   string
	Limit                 int
	Offset                int
	IncludeSystemMessages bool
	BaseDir               string
}

// GetSubagentMessagesOptions controls GetSubagentMessages.
type GetSubagentMessagesOptions struct {
	Dir     string
	Limit   int
	Offset  int
	BaseDir string
}

// ListSubagentsOptions controls ListSubagents.
type ListSubagentsOptions struct {
	Dir     string
	BaseDir string
}

// SessionMutationOptions are shared by the session mutation helpers:
// RenameSession, TagSession, ArchiveSession, UnarchiveSession,
// DeleteSession and ForkSession.
type SessionMutationOptions struct {
	Dir     string
	BaseDir string
}

// ForkSessionOptions controls ForkSession.
type ForkSessionOptions struct {
	SessionMutationOptions
	// UpToMessageID cuts the copy at the message with this uuid, inclusive.
	// The TS SDK also treats compaction ids specially as of v0.3.325 (a
	// summary's id leaves out the messages kept after it, a compact_boundary
	// id ends before the boundary); this port cuts at the uuid as given
	// (sdk.d.ts v0.3.325 L909).
	UpToMessageID string
	Title         string
}

// ForkSessionResult is returned by ForkSession.
type ForkSessionResult struct {
	SessionID string `json:"sessionId"`
}

type sessionFile struct {
	sessionID  string
	projectKey string
	path       string
	cwd        string
}

var sessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F-]{8,}$`)

// ListSessions returns session metadata from the local Claude projects store.
func ListSessions(opts *ListSessionsOptions) ([]SDKSessionInfo, error) {
	files, err := findSessionFiles(sessionOptionsDir(opts), sessionOptionsBaseDir(opts))
	if err != nil {
		return nil, err
	}
	out := make([]SDKSessionInfo, 0, len(files))
	for _, file := range files {
		info, err := readSessionInfo(file)
		if err != nil || info == nil {
			continue
		}
		out = append(out, *info)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastModified > out[j].LastModified
	})
	offset, limit := 0, 0
	if opts != nil {
		offset = opts.Offset
		limit = opts.Limit
	}
	return paginateSessions(out, offset, limit), nil
}

// GetSessionInfo returns metadata for one session, or nil if it is not found.
func GetSessionInfo(sessionID string, opts *GetSessionInfoOptions) (*SDKSessionInfo, error) {
	if !validSessionID(sessionID) {
		return nil, fmt.Errorf("invalid sessionId: %s", sessionID)
	}
	file, err := findSessionFile(sessionID, sessionInfoOptionsDir(opts), sessionInfoOptionsBaseDir(opts))
	if err != nil || file == nil {
		return nil, err
	}
	return readSessionInfo(*file)
}

// GetSessionMessages reads conversation messages from a session transcript.
func GetSessionMessages(sessionID string, opts *GetSessionMessagesOptions) ([]SessionMessage, error) {
	if !validSessionID(sessionID) {
		return nil, fmt.Errorf("invalid sessionId: %s", sessionID)
	}
	file, err := findSessionFile(sessionID, sessionMessagesOptionsDir(opts), sessionMessagesOptionsBaseDir(opts))
	if err != nil || file == nil {
		return []SessionMessage{}, err
	}
	includeSystem := opts != nil && opts.IncludeSystemMessages
	msgs, err := readSessionMessages(file.path, includeSystem)
	if err != nil {
		return nil, err
	}
	offset, limit := 0, 0
	if opts != nil {
		offset = opts.Offset
		limit = opts.Limit
	}
	return paginateMessages(msgs, offset, limit), nil
}

// ListSubagents returns subagent IDs recorded under a session.
func ListSubagents(sessionID string, opts *ListSubagentsOptions) ([]string, error) {
	if !validSessionID(sessionID) {
		return nil, fmt.Errorf("invalid sessionId: %s", sessionID)
	}
	file, err := findSessionFile(sessionID, subagentsOptionsDir(opts), subagentsOptionsBaseDir(opts))
	if err != nil || file == nil {
		return []string{}, err
	}
	dir := strings.TrimSuffix(file.path, ".jsonl")
	entries, err := os.ReadDir(filepath.Join(dir, "subagents"))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "agent-") && strings.HasSuffix(name, ".jsonl") {
			out = append(out, strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".jsonl"))
		}
	}
	sort.Strings(out)
	return out, nil
}

// GetSubagentMessages reads a subagent transcript.
func GetSubagentMessages(sessionID, agentID string, opts *GetSubagentMessagesOptions) ([]SessionMessage, error) {
	if !validSessionID(sessionID) {
		return nil, fmt.Errorf("invalid sessionId: %s", sessionID)
	}
	file, err := findSessionFile(sessionID, subagentMessagesOptionsDir(opts), subagentMessagesOptionsBaseDir(opts))
	if err != nil || file == nil || agentID == "" {
		return []SessionMessage{}, err
	}
	path := filepath.Join(strings.TrimSuffix(file.path, ".jsonl"), "subagents", "agent-"+agentID+".jsonl")
	msgs, err := readSessionMessages(path, false)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionMessage{}, nil
		}
		return nil, err
	}
	offset, limit := 0, 0
	if opts != nil {
		offset = opts.Offset
		limit = opts.Limit
	}
	return paginateMessages(msgs, offset, limit), nil
}

// RenameSession appends a custom-title entry to a session transcript.
func RenameSession(sessionID, title string, opts *SessionMutationOptions) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title must be non-empty")
	}
	return appendSessionMutation(sessionID, opts, map[string]interface{}{
		"type":        "custom-title",
		"customTitle": title,
	})
}

// TagSession appends a tag entry to a session transcript. Pass an empty tag to clear it.
func TagSession(sessionID, tag string, opts *SessionMutationOptions) error {
	if strings.TrimSpace(tag) == "" && tag != "" {
		return errors.New("tag must be non-empty")
	}
	return appendSessionMutation(sessionID, opts, map[string]interface{}{
		"type": "tag",
		"tag":  strings.TrimSpace(tag),
	})
}

// DeleteSession removes a session transcript and any subagent transcripts.
// It deletes data only on this computer, not anything stored by Anthropic.
func DeleteSession(sessionID string, opts *SessionMutationOptions) error {
	if !validSessionID(sessionID) {
		return fmt.Errorf("invalid sessionId: %s", sessionID)
	}
	file, err := findSessionFile(sessionID, mutationOptionsDir(opts), mutationOptionsBaseDir(opts))
	if err != nil || file == nil {
		return err
	}
	if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(strings.TrimSuffix(file.path, ".jsonl"))
}

// ArchiveSession marks a session archived. It stays on disk and ListSessions
// still returns it, with Archived set to true. A prompt sent in the session
// later unarchives it. The transcript is not changed and a process running
// the session is not stopped. It returns an error when the session is not
// found (sdk.d.ts v0.3.325 L147).
func ArchiveSession(sessionID string, opts *SessionMutationOptions) error {
	return setSessionArchived(sessionID, true, opts)
}

// UnarchiveSession clears the mark ArchiveSession set, so ListSessions
// returns the session with Archived set to false. It returns an error when
// the session is not found (sdk.d.ts v0.3.325 L10636).
func UnarchiveSession(sessionID string, opts *SessionMutationOptions) error {
	return setSessionArchived(sessionID, false, opts)
}

// archiveMarkMaxBytes bounds the read of an archived.json sidecar, as the TS
// SDK does.
const archiveMarkMaxBytes = 4096

// archiveMark is the archived.json sidecar the TS SDK and the CLI share. It
// lives in the session's own directory beside the transcript, not in it, so
// the mark never alters the conversation.
type archiveMark struct {
	// Archived is a pointer so that a mark without the key, or with null,
	// reads as no mark, as the TS SDK's schema has it.
	Archived *bool  `json:"archived"`
	At       string `json:"at"`
	By       string `json:"by,omitempty"`
}

func archiveMarkPath(file sessionFile) string {
	return filepath.Join(strings.TrimSuffix(file.path, ".jsonl"), "archived.json")
}

func setSessionArchived(sessionID string, archived bool, opts *SessionMutationOptions) error {
	if !validSessionID(sessionID) {
		return fmt.Errorf("invalid sessionId: %s", sessionID)
	}
	files, err := findSessionFiles(mutationOptionsDir(opts), mutationOptionsBaseDir(opts))
	if err != nil {
		return err
	}

	// A session resumed from another folder can have a transcript in more
	// than one project directory; mark each so no listing disagrees.
	var matched []sessionFile
	for _, file := range files {
		if file.sessionID == sessionID {
			matched = append(matched, file)
		}
	}
	if len(matched) == 0 {
		if dir := mutationOptionsDir(opts); dir != "" {
			return fmt.Errorf("session %s not found in project directory for %s", sessionID, dir)
		}
		return fmt.Errorf("session %s not found in any project directory", sessionID)
	}

	data, err := json.Marshal(archiveMark{
		Archived: &archived,
		At:       time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		By:       "sdk",
	})
	if err != nil {
		return err
	}
	var errs []error
	for _, file := range matched {
		errs = append(errs, writeArchiveMark(archiveMarkPath(file), data))
	}
	return errors.Join(errs...)
}

func writeArchiveMark(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// The name is the TS SDK's, <name>.tmp.<8 hex>, which the CLI's
	// retention sweep matches exactly and removes if a crash strands one
	// before the rename.
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return err
	}
	tmp, err := os.OpenFile(
		path+".tmp."+hex.EncodeToString(suffix[:]),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600,
	)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// readArchiveMark returns the session's archive mark, or nil when there is
// none or it cannot be read as one.
func readArchiveMark(file sessionFile) *archiveMark {
	path := archiveMarkPath(file)
	// Only a regular file is a mark: a symlink or FIFO in its place would
	// otherwise redirect or block the listing.
	if st, err := os.Lstat(path); err != nil || !st.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, archiveMarkMaxBytes+1))
	if err != nil || len(data) > archiveMarkMaxBytes {
		return nil
	}
	var mark archiveMark
	if err := json.Unmarshal(data, &mark); err != nil || mark.Archived == nil {
		return nil
	}
	if _, err := time.Parse(time.RFC3339Nano, mark.At); err != nil {
		return nil
	}
	return &mark
}

// effectiveArchived folds the mark with the transcript. The CLI rewrites
// the mark to false when a person prompts in an archived session, but only
// on the prompt paths it handles itself, and an older CLI never does; so a
// person's prompt written after the mark unarchives the session here too,
// as in the TS SDK.
func effectiveArchived(mark *archiveMark, entries []map[string]interface{}) *bool {
	if mark == nil {
		return nil
	}
	archived := *mark.Archived
	if archived {
		at, _ := time.Parse(time.RFC3339Nano, mark.At)
		for _, entry := range entries {
			if ts, ok := humanPromptTime(entry); ok && ts.After(at) {
				archived = false
				break
			}
		}
	}
	return &archived
}

// absentOrFalse reports whether key is missing from m or holds false, the
// only two values the TS SDK's schema admits for its prompt flags.
func absentOrFalse(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	return !ok || v == false
}

// humanPromptTime returns the timestamp of a transcript entry that records a
// prompt a person sent, either as a user message or as a queued command. It
// ports the TS SDK's test for unarchiving (JLe): an entry with any flag that
// marks it synthetic, scheduled, a wakeup, a tool result or a non-human
// origin does not count, nor does text the CLI injected.
func humanPromptTime(entry map[string]interface{}) (time.Time, bool) {
	if !absentOrFalse(entry, "isSidechain") {
		return time.Time{}, false
	}
	var content, origin interface{}
	switch entry["type"] {
	case "user":
		for _, flag := range []string{
			"isMeta", "isCompactSummary", "isVirtual",
			"isVisibleInTranscriptOnly", "textNotTyped", "handedOff",
			"queueSkipAttachments",
		} {
			if !absentOrFalse(entry, flag) {
				return time.Time{}, false
			}
		}
		// TS types these as undefined: present with any value, the entry
		// is a wakeup, a scheduled task or a persisted injection.
		for _, key := range []string{"persist", "wakeupSource", "scheduledTaskId"} {
			if _, ok := entry[key]; ok {
				return time.Time{}, false
			}
		}
		if v, ok := entry["toolUseResult"]; ok && v != nil {
			return time.Time{}, false
		}
		if v, ok := entry["promptSource"]; ok {
			if _, isString := v.(string); !isString {
				return time.Time{}, false
			}
		}
		if v, ok := entry["turnOrigin"]; ok {
			switch v {
			case "human", "sdk", "unknown":
			default:
				return time.Time{}, false
			}
		}
		msg, ok := entry["message"].(map[string]interface{})
		if !ok {
			return time.Time{}, false
		}
		if _, ok := msg["content"]; !ok {
			return time.Time{}, false
		}
		content, origin = msg["content"], entry["origin"]
	case "attachment":
		att, ok := entry["attachment"].(map[string]interface{})
		if !ok || att["type"] != "queued_command" || !absentOrFalse(att, "isMeta") {
			return time.Time{}, false
		}
		if v, ok := att["commandMode"]; ok && v != "prompt" && v != "bash" {
			return time.Time{}, false
		}
		if _, ok := att["prompt"]; !ok {
			return time.Time{}, false
		}
		content, origin = att["prompt"], att["origin"]
	default:
		return time.Time{}, false
	}
	hasOrigin := false
	if origin != nil {
		o, ok := origin.(map[string]interface{})
		if !ok {
			return time.Time{}, false
		}
		if _, ok := o["kind"].(string); !ok {
			return time.Time{}, false
		}
		if o["kind"] != "human" {
			return time.Time{}, false
		}
		hasOrigin = true
	}

	var blocks []interface{}
	switch c := content.(type) {
	case string:
		blocks = []interface{}{map[string]interface{}{"type": "text", "text": c}}
	case []interface{}:
		blocks = c
	}
	var texts []string
	media := false
	for _, item := range blocks {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		switch block["type"] {
		case "tool_result":
			return time.Time{}, false
		case "image", "document":
			media = true
		}
		text, _ := block["text"].(string)
		texts = append(texts, strings.TrimLeft(text, " \t\r\n"))
	}
	if !media && !archivePromptText(texts, hasOrigin) {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, sessionGetString(entry, "timestamp"))
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

var (
	commandNamePattern = regexp.MustCompile(`<command-name>(.*?)</command-name>`)
	bashInputPattern   = regexp.MustCompile(`<bash-input>[\s\S]*?</bash-input>`)
)

// archivePromptText reports whether texts, each already trimmed at the
// start, hold something a person typed, by the TS SDK's rules for
// unarchiving. A turn with a human origin counts unless it is a slash
// command. Without one, a prompt-type slash command (<command-message>) and
// a shell command (<bash-input>) count, and so does any text that is not a
// slash-command name or other injected tag. That is wider than promptText,
// which picks what a session listing shows, so the two stay separate.
func archivePromptText(texts []string, hasOrigin bool) bool {
	if hasOrigin {
		nonEmpty := false
		for _, t := range texts {
			if strings.HasPrefix(t, "<command-name>") {
				return false
			}
			nonEmpty = nonEmpty || t != ""
		}
		return nonEmpty
	}
	for _, t := range texts {
		if strings.HasPrefix(t, "<command-message>") {
			return true
		}
		c := strings.TrimSpace(strings.ReplaceAll(t, "\n", " "))
		if c == "" || commandNamePattern.MatchString(c) {
			continue
		}
		if bashInputPattern.MatchString(c) || !notPromptPattern.MatchString(c) {
			return true
		}
	}
	return false
}

// ForkSession copies a session transcript to a new session ID, remapping UUIDs.
func ForkSession(sessionID string, opts *ForkSessionOptions) (*ForkSessionResult, error) {
	if !validSessionID(sessionID) {
		return nil, fmt.Errorf("invalid sessionId: %s", sessionID)
	}
	dir, baseDir := "", ""
	if opts != nil {
		dir = opts.Dir
		baseDir = opts.BaseDir
	}
	file, err := findSessionFile(sessionID, dir, baseDir)
	if err != nil || file == nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	entries, err := readTranscriptEntries(file.path)
	if err != nil {
		return nil, err
	}
	if opts != nil && opts.UpToMessageID != "" {
		cut := -1
		for i, entry := range entries {
			if sessionGetString(entry, "uuid") == opts.UpToMessageID {
				cut = i
				break
			}
		}
		if cut < 0 {
			return nil, fmt.Errorf("message %s not found", opts.UpToMessageID)
		}
		entries = entries[:cut+1]
	}
	newID := newUUID()
	uuidMap := map[string]string{sessionID: newID}
	for _, entry := range entries {
		if uuid := sessionGetString(entry, "uuid"); uuid != "" {
			uuidMap[uuid] = newUUID()
		}
	}
	for _, entry := range entries {
		entry["sessionId"] = newID
		entry["session_id"] = newID
		if uuid := sessionGetString(entry, "uuid"); uuid != "" {
			entry["uuid"] = uuidMap[uuid]
		}
		if parent := sessionGetString(entry, "parentUuid"); parent != "" {
			if mapped := uuidMap[parent]; mapped != "" {
				entry["parentUuid"] = mapped
			}
		}
	}
	title := ""
	if opts != nil {
		title = strings.TrimSpace(opts.Title)
	}
	if title != "" {
		entries = append(entries, mutationEntry(newID, map[string]interface{}{
			"type":        "custom-title",
			"customTitle": title,
		}))
	}
	target := filepath.Join(filepath.Dir(file.path), newID+".jsonl")
	if err := writeTranscriptEntries(target, entries); err != nil {
		return nil, err
	}
	return &ForkSessionResult{SessionID: newID}, nil
}

func appendSessionMutation(sessionID string, opts *SessionMutationOptions, entry map[string]interface{}) error {
	if !validSessionID(sessionID) {
		return fmt.Errorf("invalid sessionId: %s", sessionID)
	}
	file, err := findSessionFile(sessionID, mutationOptionsDir(opts), mutationOptionsBaseDir(opts))
	if err != nil || file == nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("session %s not found", sessionID)
	}
	return appendTranscriptEntry(file.path, mutationEntry(sessionID, entry))
}

func mutationEntry(sessionID string, entry map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range entry {
		out[k] = v
	}
	out["sessionId"] = sessionID
	out["session_id"] = sessionID
	out["uuid"] = newUUID()
	out["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	return out
}

func findSessionFile(sessionID, dir, baseDir string) (*sessionFile, error) {
	files, err := findSessionFiles(dir, baseDir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.sessionID == sessionID {
			return &file, nil
		}
	}
	return nil, nil
}

func findSessionFiles(dir, baseDir string) ([]sessionFile, error) {
	projectsDir, err := sessionsProjectsDir(baseDir)
	if err != nil {
		return nil, err
	}
	projectDirs := []string{}
	if dir != "" {
		projectDirs = append(projectDirs, filepath.Join(projectsDir, projectKey(dir)))
	} else {
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			if os.IsNotExist(err) {
				return []sessionFile{}, nil
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				projectDirs = append(projectDirs, filepath.Join(projectsDir, entry.Name()))
			}
		}
	}
	out := []sessionFile{}
	for _, projectDir := range projectDirs {
		entries, err := os.ReadDir(projectDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
			out = append(out, sessionFile{
				sessionID:  sessionID,
				projectKey: filepath.Base(projectDir),
				path:       filepath.Join(projectDir, entry.Name()),
				cwd:        dir,
			})
		}
	}
	return out, nil
}

func readSessionInfo(file sessionFile) (*SDKSessionInfo, error) {
	stat, err := os.Stat(file.path)
	if err != nil {
		return nil, err
	}
	entries, err := readTranscriptEntries(file.path)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	data := sessionSummaryData{}
	for _, entry := range entries {
		if sidechain, _ := entry["isSidechain"].(bool); sidechain {
			return nil, nil
		}
		data.fold(entry)
	}
	summary := firstNonEmpty(data.customTitle, data.aiTitle, data.lastPromptEntry, data.lastPrompt, data.summaryHint, data.firstPrompt)
	if summary == "" {
		return nil, nil
	}
	return &SDKSessionInfo{
		SessionID:    file.sessionID,
		Summary:      summary,
		LastModified: stat.ModTime().UnixMilli(),
		FileSize:     stat.Size(),
		CustomTitle:  firstNonEmpty(data.customTitle, data.aiTitle),
		FirstPrompt:  data.firstPrompt,
		GitBranch:    data.gitBranch,
		Cwd:          firstNonEmpty(data.cwd, file.cwd),
		Tag:          data.tag,
		CreatedAt:    data.createdAt,
		Archived:     effectiveArchived(readArchiveMark(file), entries),
	}, nil
}

type sessionSummaryData struct {
	customTitle string
	aiTitle     string
	lastPrompt  string
	// lastPromptEntry is the CLI's own last-prompt record, which wins over
	// a prompt derived from user entries, as in the TS SDK.
	lastPromptEntry string
	summaryHint     string
	firstPrompt     string
	gitBranch       string
	cwd             string
	tag             string
	createdAt       int64
}

func (d *sessionSummaryData) fold(entry map[string]interface{}) {
	if d.createdAt == 0 {
		if parsed := parseTimestampMillis(sessionGetString(entry, "timestamp")); parsed != 0 {
			d.createdAt = parsed
		}
	}
	if d.cwd == "" {
		d.cwd = sessionGetString(entry, "cwd")
	}
	if v := sessionGetString(entry, "customTitle"); v != "" {
		d.customTitle = v
	}
	if v := sessionGetString(entry, "aiTitle"); v != "" {
		d.aiTitle = v
	}
	if v := sessionGetString(entry, "summary"); v != "" {
		d.summaryHint = v
	}
	if v := sessionGetString(entry, "gitBranch"); v != "" {
		d.gitBranch = v
	}
	if entry["type"] == "tag" {
		d.tag = sessionGetString(entry, "tag")
	}
	if entry["type"] == "last-prompt" {
		if v := strings.TrimSpace(sessionGetString(entry, "lastPrompt")); v != "" {
			d.lastPromptEntry = v
		}
	}
	isMeta, _ := entry["isMeta"].(bool)
	isCompactSummary, _ := entry["isCompactSummary"].(bool)
	if entry["type"] == "user" && !isMeta && !isCompactSummary {
		if prompt := extractTextFromMessage(entry["message"]); prompt != "" {
			if d.firstPrompt == "" {
				d.firstPrompt = prompt
			}
			d.lastPrompt = prompt
		}
	}
}

func readSessionMessages(path string, includeSystem bool) ([]SessionMessage, error) {
	entries, err := readTranscriptEntries(path)
	if err != nil {
		return nil, err
	}
	out := []SessionMessage{}
	for _, entry := range entries {
		typ := sessionGetString(entry, "type")
		if typ != "user" && typ != "assistant" && (!includeSystem || typ != "system") {
			continue
		}
		msgBytes, _ := json.Marshal(entry["message"])
		var parent *string
		if v := sessionGetString(entry, "parent_tool_use_id"); v != "" {
			parent = &v
		}
		var parentAgent *string
		if v := sessionGetString(entry, "parent_agent_id"); v != "" {
			parentAgent = &v
		}
		out = append(out, SessionMessage{
			Type:            typ,
			UUID:            sessionGetString(entry, "uuid"),
			SessionID:       firstNonEmpty(sessionGetString(entry, "session_id"), sessionGetString(entry, "sessionId")),
			Message:         json.RawMessage(msgBytes),
			ParentToolUseID: parent,
			ParentAgentID:   parentAgent,
		})
	}
	return out, nil
}

func readTranscriptEntries(path string) ([]map[string]interface{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var out []map[string]interface{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, entry)
	}
	return out, scanner.Err()
}

func writeTranscriptEntries(path string, entries []map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			_ = file.Close()
			_ = os.Remove(tmp)
			return err
		}
		if _, err := writer.Write(append(line, '\n')); err != nil {
			_ = file.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func appendTranscriptEntry(path string, entry map[string]interface{}) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(line, '\n'))
	return err
}

func sessionsProjectsDir(baseDir string) (string, error) {
	if baseDir == "" {
		if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
			baseDir = env
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			baseDir = filepath.Join(home, ".claude")
		}
	}
	return filepath.Join(baseDir, "projects"), nil
}

func projectKey(dir string) string {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err == nil {
		dir = abs
	}
	dir = filepath.Clean(dir)
	var b strings.Builder
	for _, r := range dir {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func validSessionID(sessionID string) bool {
	return sessionIDPattern.MatchString(sessionID) && !strings.ContainsAny(sessionID, `/\`)
}

func sessionGetString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func extractTextFromMessage(message interface{}) string {
	m, ok := message.(map[string]interface{})
	if !ok {
		return ""
	}
	// The CLI writes a typed prompt as a bare string; block arrays come
	// from SDK sends and from content with attachments.
	if text, ok := m["content"].(string); ok {
		return promptText(text)
	}
	content, isList := m["content"].([]interface{})
	if !isList {
		return ""
	}
	for _, item := range content {
		block, ok := item.(map[string]interface{})
		if !ok || block["type"] != "text" {
			continue
		}
		if text := promptText(sessionGetString(block, "text")); text != "" {
			return text
		}
	}
	return ""
}

// notPromptPattern matches text the CLI writes into user entries that a
// person did not type: slash-command and shell wrappers, caveats, and the
// interrupt marker. It mirrors the TS SDK's filter for session prompts.
var notPromptPattern = regexp.MustCompile(`^(?:\s*<[a-z][\w-]*[\s>]|\[Request interrupted by user[^\]]*\])`)

func promptText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || notPromptPattern.MatchString(text) {
		return ""
	}
	return truncatePrompt(text)
}

func truncatePrompt(text string) string {
	if len(text) > 200 {
		return strings.TrimSpace(text[:200])
	}
	return text
}

func parseTimestampMillis(value string) int64 {
	if value == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

func paginateSessions(in []SDKSessionInfo, offset, limit int) []SDKSessionInfo {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(in) {
		return []SDKSessionInfo{}
	}
	if limit <= 0 || offset+limit > len(in) {
		return in[offset:]
	}
	return in[offset : offset+limit]
}

func paginateMessages(in []SessionMessage, offset, limit int) []SessionMessage {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(in) {
		return []SessionMessage{}
	}
	if limit <= 0 || offset+limit > len(in) {
		return in[offset:]
	}
	return in[offset : offset+limit]
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:]
}

func sessionOptionsDir(opts *ListSessionsOptions) string {
	if opts == nil {
		return ""
	}
	return opts.Dir
}

func sessionOptionsBaseDir(opts *ListSessionsOptions) string {
	if opts == nil {
		return ""
	}
	return opts.BaseDir
}

func sessionInfoOptionsDir(opts *GetSessionInfoOptions) string {
	if opts == nil {
		return ""
	}
	return opts.Dir
}

func sessionInfoOptionsBaseDir(opts *GetSessionInfoOptions) string {
	if opts == nil {
		return ""
	}
	return opts.BaseDir
}

func sessionMessagesOptionsDir(opts *GetSessionMessagesOptions) string {
	if opts == nil {
		return ""
	}
	return opts.Dir
}

func sessionMessagesOptionsBaseDir(opts *GetSessionMessagesOptions) string {
	if opts == nil {
		return ""
	}
	return opts.BaseDir
}

func subagentsOptionsDir(opts *ListSubagentsOptions) string {
	if opts == nil {
		return ""
	}
	return opts.Dir
}

func subagentsOptionsBaseDir(opts *ListSubagentsOptions) string {
	if opts == nil {
		return ""
	}
	return opts.BaseDir
}

func subagentMessagesOptionsDir(opts *GetSubagentMessagesOptions) string {
	if opts == nil {
		return ""
	}
	return opts.Dir
}

func subagentMessagesOptionsBaseDir(opts *GetSubagentMessagesOptions) string {
	if opts == nil {
		return ""
	}
	return opts.BaseDir
}

func mutationOptionsDir(opts *SessionMutationOptions) string {
	if opts == nil {
		return ""
	}
	return opts.Dir
}

func mutationOptionsBaseDir(opts *SessionMutationOptions) string {
	if opts == nil {
		return ""
	}
	return opts.BaseDir
}
