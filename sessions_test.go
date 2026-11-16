package claudeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSessionID = "11111111-1111-4111-8111-111111111111"

func makeSessionFixture(t *testing.T) (baseDir string, projectDir string) {
	t.Helper()

	baseDir = t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(cwd, 0700))
	projectDir = filepath.Join(baseDir, "projects", projectKey(cwd))
	require.NoError(t, os.MkdirAll(projectDir, 0700))

	entries := []map[string]interface{}{
		{
			"type":      "user",
			"uuid":      "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			"sessionId": testSessionID,
			"timestamp": "2026-04-26T01:02:03Z",
			"cwd":       cwd,
			"gitBranch": "main",
			"message": map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "first prompt"},
				},
			},
		},
		{
			"type":       "assistant",
			"uuid":       "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			"parentUuid": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			"sessionId":  testSessionID,
			"timestamp":  "2026-04-26T01:03:03Z",
			"message": map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "answer"},
				},
			},
		},
		{
			"type":      "summary",
			"uuid":      "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
			"sessionId": testSessionID,
			"timestamp": "2026-04-26T01:04:03Z",
			"summary":   "summary hint",
		},
	}
	require.NoError(t, writeTranscriptEntries(filepath.Join(projectDir, testSessionID+".jsonl"), entries))

	subagentsDir := filepath.Join(projectDir, testSessionID, "subagents")
	require.NoError(t, os.MkdirAll(subagentsDir, 0700))
	require.NoError(t, writeTranscriptEntries(filepath.Join(subagentsDir, "agent-worker.jsonl"), entries[:2]))
	return baseDir, cwd
}

func TestListAndGetSessions(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)

	sessions, err := ListSessions(&ListSessionsOptions{BaseDir: baseDir, Dir: cwd})
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, testSessionID, sessions[0].SessionID)
	assert.Equal(t, "first prompt", sessions[0].Summary)
	assert.Equal(t, "first prompt", sessions[0].FirstPrompt)
	assert.Equal(t, "main", sessions[0].GitBranch)
	assert.Equal(t, cwd, sessions[0].Cwd)
	assert.NotZero(t, sessions[0].CreatedAt)
	assert.NotZero(t, sessions[0].LastModified)
	assert.NotZero(t, sessions[0].FileSize)

	info, err := GetSessionInfo(testSessionID, &GetSessionInfoOptions{BaseDir: baseDir})
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, sessions[0].SessionID, info.SessionID)
}

func TestGetSessionMessages(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)

	messages, err := GetSessionMessages(testSessionID, &GetSessionMessagesOptions{
		BaseDir: baseDir,
		Dir:     cwd,
		Limit:   1,
		Offset:  1,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "assistant", messages[0].Type)
	assert.Equal(t, testSessionID, messages[0].SessionID)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(messages[0].Message, &decoded))
	assert.Equal(t, "assistant", decoded["role"])
}

func TestReadSessionMessagesParentAgentID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	transcript := `{"type":"user","uuid":"u1","session_id":"s1","message":{"role":"user"},"parent_tool_use_id":null,"parent_agent_id":"agent-parent"}
{"type":"assistant","uuid":"a1","session_id":"s1","message":{"role":"assistant"},"parent_tool_use_id":null,"parent_agent_id":null}
`
	require.NoError(t, os.WriteFile(path, []byte(transcript), 0600))

	msgs, err := readSessionMessages(path, false)
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	require.NotNil(t, msgs[0].ParentAgentID)
	assert.Equal(t, "agent-parent", *msgs[0].ParentAgentID)
	assert.Nil(t, msgs[1].ParentAgentID, "null parent_agent_id decodes to nil")
}

func TestSubagentHelpers(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)

	agents, err := ListSubagents(testSessionID, &ListSubagentsOptions{BaseDir: baseDir, Dir: cwd})
	require.NoError(t, err)
	assert.Equal(t, []string{"worker"}, agents)

	messages, err := GetSubagentMessages(testSessionID, "worker", &GetSubagentMessagesOptions{
		BaseDir: baseDir,
		Dir:     cwd,
	})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, "user", messages[0].Type)
}

func TestRenameTagAndDeleteSession(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)
	opts := &SessionMutationOptions{BaseDir: baseDir, Dir: cwd}

	require.NoError(t, RenameSession(testSessionID, "new title", opts))
	require.NoError(t, TagSession(testSessionID, "important", opts))

	info, err := GetSessionInfo(testSessionID, &GetSessionInfoOptions{BaseDir: baseDir, Dir: cwd})
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "new title", info.Summary)
	assert.Equal(t, "new title", info.CustomTitle)
	assert.Equal(t, "important", info.Tag)

	require.NoError(t, DeleteSession(testSessionID, opts))
	info, err = GetSessionInfo(testSessionID, &GetSessionInfoOptions{BaseDir: baseDir, Dir: cwd})
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestForkSession(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)

	result, err := ForkSession(testSessionID, &ForkSessionOptions{
		SessionMutationOptions: SessionMutationOptions{BaseDir: baseDir, Dir: cwd},
		Title:                  "forked title",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEqual(t, testSessionID, result.SessionID)

	info, err := GetSessionInfo(result.SessionID, &GetSessionInfoOptions{BaseDir: baseDir, Dir: cwd})
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "forked title", info.Summary)

	messages, err := GetSessionMessages(result.SessionID, &GetSessionMessagesOptions{BaseDir: baseDir, Dir: cwd})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, result.SessionID, messages[0].SessionID)
	assert.NotEqual(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", messages[0].UUID)
}

// TestListSessionsCLIStringPrompt covers the shape `claude -p` writes: the
// prompt as a bare string, preceded by queue-operation entries and followed
// by a last-prompt entry. Such a session used to be dropped from listings
// because no block-array text was found.
func TestListSessionsCLIStringPrompt(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo")
	projectDir := filepath.Join(baseDir, "projects", projectKey(cwd))
	require.NoError(t, os.MkdirAll(projectDir, 0700))

	write := func(lines ...string) {
		t.Helper()
		require.NoError(t, os.WriteFile(
			filepath.Join(projectDir, testSessionID+".jsonl"),
			[]byte(strings.Join(lines, "\n")+"\n"), 0600,
		))
	}
	user := func(uuid, content string, extra string) string {
		return `{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user","content":` +
			content + `},"uuid":"` + uuid + `","timestamp":"2026-11-16T17:13:12.583Z","cwd":"` + cwd + `"` + extra + `}`
	}
	list := func() SDKSessionInfo {
		t.Helper()
		sessions, err := ListSessions(&ListSessionsOptions{BaseDir: baseDir, Dir: cwd})
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		return sessions[0]
	}

	write(
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2026-11-16T17:13:12.450Z","sessionId":"`+testSessionID+`","content":"first"}`,
		user("u0", `"<local-command-caveat>meta</local-command-caveat>"`, `,"isMeta":true`),
		user("u1", `"<command-name>/model</command-name>\n<command-message>model</command-message>"`, ""),
		user("u2", `"[Request interrupted by user]"`, ""),
		user("u3", `"first"`, ""),
		user("u4", `"second"`, ""),
		`{"type":"last-prompt","lastPrompt":"from the cli","leafUuid":"u4","sessionId":"`+testSessionID+`"}`,
		user("u5", `"This session is being continued from a previous conversation."`, `,"isCompactSummary":true`),
	)
	info := list()
	assert.Equal(t, "first", info.FirstPrompt,
		"meta, wrapper and interrupt entries are not prompts")
	assert.Equal(t, "from the cli", info.Summary,
		"the CLI's last-prompt entry wins, and a compact summary after it does not")
	assert.Equal(t, cwd, info.Cwd)

	// Without a last-prompt entry the derived last prompt is the summary.
	write(user("u3", `"first"`, ""), user("u4", `"second"`, ""),
		user("u5", `"This session is being continued from a previous conversation."`, `,"isCompactSummary":true`))
	assert.Equal(t, "second", list().Summary)
}

func TestArchiveAndUnarchiveSession(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)
	opts := &SessionMutationOptions{BaseDir: baseDir, Dir: cwd}
	infoOpts := &GetSessionInfoOptions{BaseDir: baseDir, Dir: cwd}

	info, err := GetSessionInfo(testSessionID, infoOpts)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Nil(t, info.Archived, "never archived reads as absent")

	require.NoError(t, ArchiveSession(testSessionID, opts))
	info, err = GetSessionInfo(testSessionID, infoOpts)
	require.NoError(t, err)
	require.NotNil(t, info.Archived)
	assert.True(t, *info.Archived)

	// The mark is the sidecar the TS SDK writes, not a transcript entry.
	markPath := filepath.Join(baseDir, "projects", projectKey(cwd), testSessionID, "archived.json")
	raw, err := os.ReadFile(markPath)
	require.NoError(t, err)
	var mark map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &mark))
	assert.Equal(t, true, mark["archived"])
	assert.Equal(t, "sdk", mark["by"])
	assert.Regexp(t, `^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`, mark["at"])
	st, err := os.Stat(markPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), st.Mode().Perm())

	sessions, err := ListSessions(&ListSessionsOptions{BaseDir: baseDir, Dir: cwd})
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NotNil(t, sessions[0].Archived)
	assert.True(t, *sessions[0].Archived)

	require.NoError(t, UnarchiveSession(testSessionID, opts))
	info, err = GetSessionInfo(testSessionID, infoOpts)
	require.NoError(t, err)
	require.NotNil(t, info.Archived)
	assert.False(t, *info.Archived)
}

func TestArchivedSessionUnarchivedByLaterPrompt(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)
	projectDir := filepath.Join(baseDir, "projects", projectKey(cwd))
	transcript := filepath.Join(projectDir, testSessionID+".jsonl")
	writeMark := func(at string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Join(projectDir, testSessionID), 0700))
		require.NoError(t, os.WriteFile(
			filepath.Join(projectDir, testSessionID, "archived.json"),
			[]byte(`{"archived":true,"at":"`+at+`","by":"sdk"}`), 0600,
		))
	}
	archived := func() *bool {
		t.Helper()
		info, err := GetSessionInfo(testSessionID, &GetSessionInfoOptions{BaseDir: baseDir, Dir: cwd})
		require.NoError(t, err)
		require.NotNil(t, info)
		return info.Archived
	}
	userEntry := func(ts string, content interface{}, extra map[string]interface{}) map[string]interface{} {
		e := map[string]interface{}{
			"type":      "user",
			"sessionId": testSessionID,
			"timestamp": ts,
			"message":   map[string]interface{}{"role": "user", "content": content},
		}
		for k, v := range extra {
			e[k] = v
		}
		return e
	}

	writeMark("2026-04-26T02:00:00.000Z")
	require.True(t, *archived(), "fixture prompt predates the mark")

	// Entries that are not a person's prompt keep the mark in force.
	for _, e := range []map[string]interface{}{
		userEntry("2026-04-26T03:00:00Z", []interface{}{
			map[string]interface{}{"type": "tool_result", "tool_use_id": "t1", "content": "ok"},
		}, nil),
		userEntry("2026-04-26T03:00:00Z", "meta", map[string]interface{}{"isMeta": true}),
		userEntry("2026-04-26T03:00:00Z", "cron", map[string]interface{}{
			"origin": map[string]interface{}{"kind": "scheduled-trigger"},
		}),
		userEntry("2026-04-26T03:00:00Z", "<command-name>/clear</command-name>", nil),
	} {
		require.NoError(t, appendTranscriptEntry(transcript, e))
	}
	require.True(t, *archived())

	require.NoError(t, appendTranscriptEntry(transcript,
		userEntry("2026-04-26T03:00:00Z", "back to work", nil)))
	assert.False(t, *archived(), "a later prompt unarchives")

	writeMark("2026-04-26T04:00:00.000Z")
	assert.True(t, *archived(), "re-archiving after the prompt holds")

	// A queued command a person typed counts like a prompt.
	require.NoError(t, appendTranscriptEntry(transcript, map[string]interface{}{
		"type":      "attachment",
		"timestamp": "2026-04-26T05:00:00Z",
		"attachment": map[string]interface{}{
			"type": "queued_command", "prompt": "queued",
			"origin": map[string]interface{}{"kind": "human"},
		},
	}))
	assert.False(t, *archived())
}

func TestArchiveSessionErrors(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)

	err := ArchiveSession("not a uuid", &SessionMutationOptions{BaseDir: baseDir})
	assert.ErrorContains(t, err, "invalid sessionId")

	missing := "22222222-2222-4222-8222-222222222222"
	err = ArchiveSession(missing, &SessionMutationOptions{BaseDir: baseDir})
	assert.ErrorContains(t, err, "not found in any project directory")
	err = UnarchiveSession(missing, &SessionMutationOptions{BaseDir: baseDir, Dir: cwd})
	assert.ErrorContains(t, err, "not found in project directory for "+cwd)

	// An oversized or malformed sidecar reads as no mark rather than failing
	// the listing.
	markDir := filepath.Join(baseDir, "projects", projectKey(cwd), testSessionID)
	require.NoError(t, os.WriteFile(filepath.Join(markDir, "archived.json"), []byte(`{"archived":true}`), 0600))
	info, err := GetSessionInfo(testSessionID, &GetSessionInfoOptions{BaseDir: baseDir, Dir: cwd})
	require.NoError(t, err)
	assert.Nil(t, info.Archived)
}

// TestHumanPromptTime pins which transcript entries unarchive a session,
// row for row against the TS SDK's schema (JLe in sdk.mjs v0.3.325).
func TestHumanPromptTime(t *testing.T) {
	const ts = "2026-04-26T03:00:00Z"
	user := func(content interface{}, extra map[string]interface{}) map[string]interface{} {
		e := map[string]interface{}{
			"type": "user", "timestamp": ts,
			"message": map[string]interface{}{"role": "user", "content": content},
		}
		for k, v := range extra {
			e[k] = v
		}
		return e
	}
	queued := func(att map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
		a := map[string]interface{}{"type": "queued_command", "prompt": "queued"}
		for k, v := range att {
			a[k] = v
		}
		e := map[string]interface{}{"type": "attachment", "timestamp": ts, "attachment": a}
		for k, v := range extra {
			e[k] = v
		}
		return e
	}
	human := map[string]interface{}{"kind": "human"}

	for _, tc := range []struct {
		name  string
		entry map[string]interface{}
		want  bool
	}{
		{"plain string", user("hi", nil), true},
		{"text block", user([]interface{}{map[string]interface{}{"type": "text", "text": "hi"}}, nil), true},
		{"image only", user([]interface{}{map[string]interface{}{"type": "image"}}, nil), true},
		{"document only", user([]interface{}{map[string]interface{}{"type": "document"}}, nil), true},
		{"tool result", user([]interface{}{map[string]interface{}{"type": "tool_result"}}, nil), false},
		{"empty", user("   ", nil), false},
		{"no timestamp", map[string]interface{}{"type": "user", "message": map[string]interface{}{"content": "hi"}}, false},

		{"isMeta", user("hi", map[string]interface{}{"isMeta": true}), false},
		{"isMeta false", user("hi", map[string]interface{}{"isMeta": false}), true},
		{"isMeta non-bool", user("hi", map[string]interface{}{"isMeta": "yes"}), false},
		{"isSidechain", user("hi", map[string]interface{}{"isSidechain": true}), false},
		{"isCompactSummary", user("hi", map[string]interface{}{"isCompactSummary": true}), false},
		{"isVirtual", user("hi", map[string]interface{}{"isVirtual": true}), false},
		{"isVisibleInTranscriptOnly", user("hi", map[string]interface{}{"isVisibleInTranscriptOnly": true}), false},
		{"textNotTyped", user("hi", map[string]interface{}{"textNotTyped": true}), false},
		{"handedOff", user("hi", map[string]interface{}{"handedOff": true}), false},
		{"queueSkipAttachments", user("hi", map[string]interface{}{"queueSkipAttachments": true}), false},
		{"wakeupSource", user("hi", map[string]interface{}{"wakeupSource": "loop"}), false},
		{"scheduledTaskId", user("hi", map[string]interface{}{"scheduledTaskId": "t1"}), false},
		{"persist", user("hi", map[string]interface{}{"persist": false}), false},
		{"toolUseResult", user("hi", map[string]interface{}{"toolUseResult": "x"}), false},
		{"toolUseResult null", user("hi", map[string]interface{}{"toolUseResult": nil}), true},
		{"turnOrigin sdk", user("hi", map[string]interface{}{"turnOrigin": "sdk"}), true},
		{"turnOrigin other", user("hi", map[string]interface{}{"turnOrigin": "cron"}), false},
		{"promptSource non-string", user("hi", map[string]interface{}{"promptSource": 1}), false},

		{"human origin", user("hi", map[string]interface{}{"origin": human}), true},
		{"other origin", user("hi", map[string]interface{}{"origin": map[string]interface{}{"kind": "peer"}}), false},
		{"human origin slash command", user("<command-name>/x</command-name>", map[string]interface{}{"origin": human}), false},
		{"human origin other tag", user("<task-notification>x</task-notification>", map[string]interface{}{"origin": human}), true},

		{"slash command name", user("<command-name>/clear</command-name>", nil), false},
		{"prompt slash command", user("<command-message>review</command-message>\n<command-name>/review</command-name>", nil), true},
		{"bash input", user("<bash-input>ls</bash-input>", nil), true},
		{"local command output", user("<local-command-stdout>x</local-command-stdout>", nil), false},
		{"interrupt marker", user("[Request interrupted by user]", nil), false},

		{"queued human", queued(map[string]interface{}{"origin": human}, nil), true},
		{"queued no origin", queued(nil, nil), true},
		{"queued meta", queued(map[string]interface{}{"isMeta": true}, nil), false},
		{"queued sidechain", queued(nil, map[string]interface{}{"isSidechain": true}), false},
		{"queued bash mode", queued(map[string]interface{}{"commandMode": "bash"}, nil), true},
		{"queued notification mode", queued(map[string]interface{}{"commandMode": "task-notification"}, nil), false},
		{"other attachment", map[string]interface{}{"type": "attachment", "timestamp": ts,
			"attachment": map[string]interface{}{"type": "date"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got := humanPromptTime(tc.entry)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestArchiveMarkEdgeCases(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)
	markDir := filepath.Join(baseDir, "projects", projectKey(cwd), testSessionID)
	markPath := filepath.Join(markDir, "archived.json")
	archived := func() *bool {
		t.Helper()
		info, err := GetSessionInfo(testSessionID, &GetSessionInfoOptions{BaseDir: baseDir, Dir: cwd})
		require.NoError(t, err)
		require.NotNil(t, info)
		return info.Archived
	}
	const at = `"at":"2026-04-26T02:00:00.000Z"`

	for name, body := range map[string]string{
		"no archived key": `{` + at + `}`,
		"archived null":   `{"archived":null,` + at + `}`,
		"oversized":       `{"archived":true,` + at + `,"pad":"` + strings.Repeat("x", archiveMarkMaxBytes) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(markPath, []byte(body), 0600))
			assert.Nil(t, archived())
		})
	}

	t.Run("symlink is not a mark", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "elsewhere.json")
		require.NoError(t, os.WriteFile(target, []byte(`{"archived":true,`+at+`}`), 0600))
		require.NoError(t, os.Remove(markPath))
		require.NoError(t, os.Symlink(target, markPath))
		assert.Nil(t, archived())
		require.NoError(t, os.Remove(markPath))
	})

	t.Run("no temp file left behind", func(t *testing.T) {
		require.NoError(t, ArchiveSession(testSessionID, &SessionMutationOptions{BaseDir: baseDir, Dir: cwd}))
		entries, err := os.ReadDir(markDir)
		require.NoError(t, err)
		for _, e := range entries {
			assert.NotContains(t, e.Name(), ".tmp.")
		}
	})
}

// TestArchiveSessionMarksEveryProjectDir covers a transcript present under
// two project directories: with no Dir, both get the mark.
func TestArchiveSessionMarksEveryProjectDir(t *testing.T) {
	baseDir, cwd := makeSessionFixture(t)
	src := filepath.Join(baseDir, "projects", projectKey(cwd), testSessionID+".jsonl")
	otherDir := filepath.Join(baseDir, "projects", "-other-project")
	require.NoError(t, os.MkdirAll(otherDir, 0700))
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(otherDir, testSessionID+".jsonl"), data, 0600))

	require.NoError(t, ArchiveSession(testSessionID, &SessionMutationOptions{BaseDir: baseDir}))
	for _, dir := range []string{filepath.Dir(src), otherDir} {
		_, err := os.Stat(filepath.Join(dir, testSessionID, "archived.json"))
		assert.NoError(t, err, dir)
	}
}
