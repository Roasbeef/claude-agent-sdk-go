package claudeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
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
