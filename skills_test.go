package claudeagent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseSKILLMd tests YAML frontmatter parsing.
func TestParseSKILLMd(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantName    string
		wantDesc    string
		wantTools   []string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid minimal frontmatter",
			content: `---
name: Test Skill
description: A test Skill for validation
---

# Test Skill
This is a test Skill.`,
			wantName:  "Test Skill",
			wantDesc:  "A test Skill for validation",
			wantTools: nil,
			wantErr:   false,
		},
		{
			name: "valid frontmatter with allowed-tools",
			content: `---
name: Research Analyst
description: Perform deep fundamental analysis on equities
allowed-tools:
  - fetch_research
  - fetch_quote
---

# Research Analyst
Detailed instructions here.`,
			wantName:  "Research Analyst",
			wantDesc:  "Perform deep fundamental analysis on equities",
			wantTools: []string{"fetch_research", "fetch_quote"},
			wantErr:   false,
		},
		{
			name: "missing frontmatter delimiters",
			content: `name: Test
description: Test`,
			wantErr:     true,
			errContains: "missing frontmatter delimiters",
		},
		{
			name: "invalid YAML",
			content: `---
name: Test
description: [invalid yaml structure
---

Content`,
			wantErr:     true,
			errContains: "failed to parse YAML",
		},
		{
			name: "empty frontmatter",
			content: `---
---

Content`,
			wantErr: false, // Parsing succeeds, validation fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, markdown, err := parseSKILLMd([]byte(tt.content))

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseSKILLMd() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("parseSKILLMd() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("parseSKILLMd() unexpected error = %v", err)
				return
			}

			if metadata.Name != tt.wantName {
				t.Errorf("parseSKILLMd() name = %q, want %q", metadata.Name, tt.wantName)
			}

			if metadata.Description != tt.wantDesc {
				t.Errorf("parseSKILLMd() description = %q, want %q", metadata.Description, tt.wantDesc)
			}

			if len(metadata.AllowedTools) != len(tt.wantTools) {
				t.Errorf("parseSKILLMd() allowed-tools count = %d, want %d", len(metadata.AllowedTools), len(tt.wantTools))
			} else {
				for i, tool := range tt.wantTools {
					if metadata.AllowedTools[i] != tool {
						t.Errorf("parseSKILLMd() allowed-tools[%d] = %q, want %q", i, metadata.AllowedTools[i], tool)
					}
				}
			}

			if markdown == "" {
				t.Errorf("parseSKILLMd() markdown content is empty")
			}
		})
	}
}

// TestValidateSkillMetadata tests metadata validation.
func TestValidateSkillMetadata(t *testing.T) {
	tests := []struct {
		name        string
		metadata    SkillMetadata
		wantErr     bool
		errContains string
	}{
		{
			name: "valid metadata",
			metadata: SkillMetadata{
				Name:        "Test Skill",
				Description: "A test Skill",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			metadata: SkillMetadata{
				Name:        "",
				Description: "A test Skill",
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "missing description",
			metadata: SkillMetadata{
				Name:        "Test Skill",
				Description: "",
			},
			wantErr:     true,
			errContains: "description is required",
		},
		{
			name: "whitespace-only name",
			metadata: SkillMetadata{
				Name:        "   ",
				Description: "A test Skill",
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "whitespace-only description",
			metadata: SkillMetadata{
				Name:        "Test Skill",
				Description: "   ",
			},
			wantErr:     true,
			errContains: "description is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSkillMetadata(tt.metadata)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateSkillMetadata() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("validateSkillMetadata() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("validateSkillMetadata() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestSkillLoader tests Skill loading from filesystem.
func TestSkillLoader(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create user Skills directory
	userSkillsDir := filepath.Join(tmpDir, "user-skills")
	if err := os.MkdirAll(userSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create user Skills directory: %v", err)
	}

	// Create project Skills directory
	projectSkillsDir := filepath.Join(tmpDir, "project-skills")
	if err := os.MkdirAll(projectSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create project Skills directory: %v", err)
	}

	// Create test Skill in user directory
	userSkillDir := filepath.Join(userSkillsDir, "test-skill")
	if err := os.MkdirAll(userSkillDir, 0755); err != nil {
		t.Fatalf("failed to create user Skill directory: %v", err)
	}

	userSkillContent := `---
name: User Test Skill
description: A test Skill from user directory
allowed-tools:
  - Read
  - Write
---

# User Test Skill
This is a user Skill.`

	if err := os.WriteFile(filepath.Join(userSkillDir, "SKILL.md"), []byte(userSkillContent), 0644); err != nil {
		t.Fatalf("failed to write user SKILL.md: %v", err)
	}

	// Create support file in user Skill
	if err := os.WriteFile(filepath.Join(userSkillDir, "reference.md"), []byte("# Reference"), 0644); err != nil {
		t.Fatalf("failed to write reference.md: %v", err)
	}

	// Create test Skill in project directory
	projectSkillDir := filepath.Join(projectSkillsDir, "project-skill")
	if err := os.MkdirAll(projectSkillDir, 0755); err != nil {
		t.Fatalf("failed to create project Skill directory: %v", err)
	}

	projectSkillContent := `---
name: Project Test Skill
description: A test Skill from project directory
---

# Project Test Skill
This is a project Skill.`

	if err := os.WriteFile(filepath.Join(projectSkillDir, "SKILL.md"), []byte(projectSkillContent), 0644); err != nil {
		t.Fatalf("failed to write project SKILL.md: %v", err)
	}

	// Create invalid Skill (missing description)
	invalidSkillDir := filepath.Join(projectSkillsDir, "invalid-skill")
	if err := os.MkdirAll(invalidSkillDir, 0755); err != nil {
		t.Fatalf("failed to create invalid Skill directory: %v", err)
	}

	invalidSkillContent := `---
name: Invalid Skill
---

# Invalid Skill`

	if err := os.WriteFile(filepath.Join(invalidSkillDir, "SKILL.md"), []byte(invalidSkillContent), 0644); err != nil {
		t.Fatalf("failed to write invalid SKILL.md: %v", err)
	}

	t.Run("load from both directories", func(t *testing.T) {
		loader := NewSkillLoader(userSkillsDir, projectSkillsDir)
		skills, err := loader.Load()

		if err != nil {
			t.Errorf("Load() unexpected error = %v", err)
		}

		// Should load 2 valid Skills (user and project), skip 1 invalid
		if len(skills) != 2 {
			t.Errorf("Load() loaded %d Skills, want 2", len(skills))
		}

		// Check user Skill
		var userSkill *Skill
		for i := range skills {
			if skills[i].Scope == "user" {
				userSkill = &skills[i]
				break
			}
		}

		if userSkill == nil {
			t.Errorf("Load() did not find user Skill")
		} else {
			if userSkill.Name != "User Test Skill" {
				t.Errorf("user Skill name = %q, want %q", userSkill.Name, "User Test Skill")
			}
			if userSkill.Description != "A test Skill from user directory" {
				t.Errorf("user Skill description = %q, want %q", userSkill.Description, "A test Skill from user directory")
			}
			if len(userSkill.AllowedTools) != 2 {
				t.Errorf("user Skill allowed-tools count = %d, want 2", len(userSkill.AllowedTools))
			}
			if len(userSkill.SupportFiles) < 1 {
				t.Errorf("user Skill support files count = %d, want at least 1", len(userSkill.SupportFiles))
			}
		}

		// Check project Skill
		var projectSkill *Skill
		for i := range skills {
			if skills[i].Scope == "project" {
				projectSkill = &skills[i]
				break
			}
		}

		if projectSkill == nil {
			t.Errorf("Load() did not find project Skill")
		} else {
			if projectSkill.Name != "Project Test Skill" {
				t.Errorf("project Skill name = %q, want %q", projectSkill.Name, "Project Test Skill")
			}
		}
	})

	t.Run("load from user directory only", func(t *testing.T) {
		loader := NewSkillLoader(userSkillsDir, "")
		skills, err := loader.Load()

		if err != nil {
			t.Errorf("Load() unexpected error = %v", err)
		}

		if len(skills) != 1 {
			t.Errorf("Load() loaded %d Skills, want 1", len(skills))
		}

		if skills[0].Scope != "user" {
			t.Errorf("Skill scope = %q, want %q", skills[0].Scope, "user")
		}
	})

	t.Run("load from non-existent directory", func(t *testing.T) {
		loader := NewSkillLoader("/nonexistent", "")
		skills, err := loader.Load()

		// Should not error, just return empty list
		if err != nil {
			t.Errorf("Load() unexpected error = %v", err)
		}

		if len(skills) != 0 {
			t.Errorf("Load() loaded %d Skills from non-existent directory, want 0", len(skills))
		}
	})

	t.Run("validate SKILL.md", func(t *testing.T) {
		loader := NewSkillLoader("", "")

		// Valid Skill
		err := loader.ValidateSKILLMd(filepath.Join(userSkillDir, "SKILL.md"))
		if err != nil {
			t.Errorf("ValidateSKILLMd() unexpected error for valid Skill = %v", err)
		}

		// Invalid Skill
		err = loader.ValidateSKILLMd(filepath.Join(invalidSkillDir, "SKILL.md"))
		if err == nil {
			t.Errorf("ValidateSKILLMd() expected error for invalid Skill, got nil")
		}
	})
}

// TestLoadFromPath tests loading a single Skill from a path.
func TestLoadFromPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test Skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create Skill directory: %v", err)
	}

	skillContent := `---
name: Path Test Skill
description: Testing LoadFromPath
---

# Path Test Skill
Content here.`

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	loader := NewSkillLoader("", "")
	skill, err := loader.LoadFromPath(skillDir, "user")

	if err != nil {
		t.Errorf("LoadFromPath() unexpected error = %v", err)
	}

	if skill.Name != "Path Test Skill" {
		t.Errorf("LoadFromPath() name = %q, want %q", skill.Name, "Path Test Skill")
	}

	if skill.Scope != "user" {
		t.Errorf("LoadFromPath() scope = %q, want %q", skill.Scope, "user")
	}

	if skill.Content == "" {
		t.Errorf("LoadFromPath() content is empty")
	}
}

// TestDiscoverSupportFiles tests support file discovery.
func TestDiscoverSupportFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	files := []string{
		"SKILL.md",     // Should be skipped
		"reference.md", // Should be included
		"examples.md",  // Should be included
		".hidden",      // Should be skipped
		"README.md",    // Should be included
	}

	for _, file := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create test file %s: %v", file, err)
		}
	}

	// Create subdirectory
	scriptsDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("failed to create scripts directory: %v", err)
	}

	supportFiles, err := discoverSupportFiles(tmpDir)
	if err != nil {
		t.Errorf("discoverSupportFiles() unexpected error = %v", err)
	}

	// Should find: reference.md, examples.md, README.md, scripts/
	// Should skip: SKILL.md, .hidden
	expectedCount := 4
	if len(supportFiles) != expectedCount {
		t.Errorf("discoverSupportFiles() found %d files, want %d", len(supportFiles), expectedCount)
	}

	// Verify SKILL.md is not included
	for _, file := range supportFiles {
		if file == "SKILL.md" {
			t.Errorf("discoverSupportFiles() included SKILL.md, should be skipped")
		}
		if file == ".hidden" {
			t.Errorf("discoverSupportFiles() included .hidden, should be skipped")
		}
	}
}

// Helper function to check if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && contains(s, substr)))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
