# Skills

Skills are filesystem-based capability extensions. They package prompts,
instructions, and tool configurations that Claude can invoke by name.

## How Skills Work

Skills are defined in `SKILL.md` files and loaded from two locations:
- `~/.claude/skills/` - User skills (available in all projects)
- `./.claude/skills/` - Project skills (project-specific)

Each skill is a directory containing a `SKILL.md` file:

```
~/.claude/skills/
├── code-reviewer/
│   └── SKILL.md
├── git-helper/
│   └── SKILL.md
└── test-writer/
    └── SKILL.md
```

## SKILL.md Format

A skill file contains YAML front matter and markdown content:

```markdown
---
name: code-reviewer
description: Review code for bugs, style issues, and best practices
allowed_tools:
  - Read
  - Glob
  - Grep
---

# Code Reviewer

You are an expert code reviewer. When reviewing code:

1. Check for bugs and logic errors
2. Verify error handling is complete
3. Look for security vulnerabilities
4. Assess test coverage
5. Suggest performance improvements

Be specific about line numbers and provide code examples for fixes.
```

### Front Matter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique skill identifier |
| `description` | Yes | Short description (shown in listings) |
| `allowed_tools` | No | Tools the skill can use (whitelist) |
| `disallowed_tools` | No | Tools the skill cannot use (blacklist) |
| `model` | No | Model override for this skill |

The markdown body becomes the system prompt when the skill is invoked.

## Loading Skills

Skills load automatically when creating a client:

```go
client, _ := goclaude.NewClient()

// List all loaded skills
for _, skill := range client.ListSkills() {
    fmt.Printf("%s: %s\n", skill.Name, skill.Description)
}
```

Output:
```
code-reviewer: Review code for bugs, style issues, and best practices
git-helper: Help with git operations and commit messages
test-writer: Generate comprehensive test suites
```

## Getting a Skill

Retrieve a specific skill:

```go
skill, err := client.GetSkill("code-reviewer")
if err != nil {
    var notFound *goclaude.ErrSkillNotFound
    if errors.As(err, &notFound) {
        fmt.Printf("Skill %s not found\n", notFound.Name)
    }
}

fmt.Printf("Name: %s\n", skill.Name)
fmt.Printf("Description: %s\n", skill.Description)
fmt.Printf("Location: %s\n", skill.Location) // "user" or "project"
fmt.Printf("Tools: %v\n", skill.AllowedTools)
```

## Reloading Skills

Pick up new or changed skills without restarting:

```go
if err := client.ReloadSkills(); err != nil {
    log.Printf("Failed to reload: %v", err)
}
```

## Validating Skills

Check a skill file before adding it:

```go
err := client.ValidateSkill("/path/to/SKILL.md")
if err != nil {
    var invalid *goclaude.ErrSkillInvalid
    if errors.As(err, &invalid) {
        fmt.Printf("Validation failed: %s - %s\n", invalid.Field, invalid.Reason)
    }
}
```

## Configuring Skill Loading

Control which skills load:

```go
// Load only project skills
client, _ := goclaude.NewClient(
    goclaude.WithSkills(goclaude.SkillsConfig{
        EnableSkills:   true,
        SettingSources: []string{"project"},
    }),
)

// Custom skill directories
client, _ := goclaude.NewClient(
    goclaude.WithSkills(goclaude.SkillsConfig{
        EnableSkills:     true,
        UserSkillsDir:    "/custom/user/skills",
        ProjectSkillsDir: "./custom/project/skills",
    }),
)

// Disable skills entirely
client, _ := goclaude.NewClient(
    goclaude.WithSkillsDisabled(),
)
```

## Skill Examples

### Code Reviewer

```markdown
---
name: code-reviewer
description: Review code for bugs, style, and best practices
allowed_tools:
  - Read
  - Glob
  - Grep
---

# Code Reviewer

You are a senior software engineer performing code review. Your review should cover:

## What to Check

**Correctness**
- Logic errors and edge cases
- Off-by-one errors
- Null/nil handling
- Resource leaks

**Security**
- Input validation
- SQL injection
- XSS vulnerabilities
- Authentication/authorization

**Style**
- Naming conventions
- Code organization
- Comment quality
- Consistency

**Performance**
- Unnecessary allocations
- N+1 queries
- Missing indexes
- Caching opportunities

## Output Format

For each issue found:

1. Severity: Critical / Major / Minor / Suggestion
2. File and line number
3. Current code
4. Recommended fix with explanation
```

### Git Helper

```markdown
---
name: git-helper
description: Help with git operations and write commit messages
allowed_tools:
  - Bash
  - Read
disallowed_tools:
  - Write
  - Edit
---

# Git Helper

You help with git operations. You can run git commands but cannot modify files directly.

## Commit Messages

When asked to write a commit message:

1. Run `git diff --staged` to see changes
2. Analyze what changed and why
3. Write a message following conventional commits:
   - feat: new feature
   - fix: bug fix
   - refactor: code change that doesn't fix a bug or add a feature
   - docs: documentation only
   - test: adding or correcting tests
   - chore: maintenance tasks

Keep the first line under 72 characters.
Add a blank line before the body if needed.
```

### Test Writer

```markdown
---
name: test-writer
description: Generate comprehensive test suites
allowed_tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
---

# Test Writer

You generate comprehensive test suites. For each function or module:

## Test Categories

1. **Happy path** - Normal expected behavior
2. **Edge cases** - Boundary conditions, empty inputs
3. **Error cases** - Invalid inputs, failure modes
4. **Integration** - Interaction with dependencies

## Go Tests

Use table-driven tests:

```go
func TestFoo(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"empty input", "", "", true},
        {"valid input", "hello", "HELLO", false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Foo(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Foo() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Foo() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

Always run `go test` after generating tests to verify they pass.
```

### API Client

```markdown
---
name: api-client
description: Generate API client code from OpenAPI specs
allowed_tools:
  - Read
  - Write
  - WebFetch
model: claude-sonnet-4-5-20250929
---

# API Client Generator

You generate Go API client code from OpenAPI/Swagger specifications.

## Process

1. Fetch or read the OpenAPI spec
2. Parse endpoints, request/response types
3. Generate:
   - Type definitions for all schemas
   - Client struct with HTTP client
   - Method for each endpoint
   - Error handling
   - Optional: mock client for testing

## Style

- Use `net/http` for HTTP client
- Accept context.Context as first parameter
- Return typed responses, not raw bytes
- Include request/response logging option
- Add retries with exponential backoff for transient errors
```

## Organizing Skills

For teams, consider:

```
project/
├── .claude/
│   └── skills/
│       ├── domain-expert/     # Project-specific domain knowledge
│       ├── deploy-helper/     # Deployment procedures
│       └── debug-guide/       # Debugging workflows
```

User skills for personal workflows:

```
~/.claude/skills/
├── my-style/          # Personal code style preferences
├── daily-standup/     # Generate standup notes
└── timetrack/         # Time tracking integration
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/roasbeef/goclaude"
)

func main() {
    client, err := goclaude.NewClient(
        goclaude.WithSkills(goclaude.SkillsConfig{
            EnableSkills:   true,
            SettingSources: []string{"user", "project"},
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // List available skills
    fmt.Println("Available skills:")
    for _, skill := range client.ListSkills() {
        fmt.Printf("  - %s (%s): %s\n", skill.Name, skill.Location, skill.Description)
    }

    // Use a skill
    skill, err := client.GetSkill("code-reviewer")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("\nUsing skill: %s\n", skill.Name)
    fmt.Printf("Allowed tools: %v\n", skill.AllowedTools)

    // The skill's prompt is available in skill.Prompt
    // It's automatically used when the skill is invoked via Claude
}
```

## Best Practices

**Keep skills focused.** Each skill should do one thing well. Combine skills
for complex workflows.

**Use tool restrictions.** Limit tools to what the skill needs. A code reviewer
doesn't need `Write`.

**Version control project skills.** They're part of your project configuration.

**Document assumptions.** Skills should be self-explanatory in their markdown
content.

**Test skills manually.** Try invoking them with various prompts before relying
on them.

## See Also

- [Permissions](permissions.md) - How tool restrictions work
- [MCP Tools](mcp-tools.md) - Custom tools skills can use
