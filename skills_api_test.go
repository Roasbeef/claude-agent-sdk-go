package claudeagent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestClientListSkills tests the ListSkills API.
func TestClientListSkills(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create project Skills directory
	projectSkillsDir := filepath.Join(tmpDir, "project-skills")
	if err := os.MkdirAll(projectSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create project Skills directory: %v", err)
	}

	// Create test Skill
	skillDir := filepath.Join(projectSkillsDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create Skill directory: %v", err)
	}

	skillContent := `---
name: Test Skill
description: A test Skill for API testing
---

# Test Skill
Content here.`

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Create empty user skills dir to isolate from real user skills
	userSkillsDir := filepath.Join(tmpDir, "user-skills")
	if err := os.MkdirAll(userSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create user Skills directory: %v", err)
	}

	// Create client with Skills enabled
	client, err := NewClient(
		WithSkills(SkillsConfig{
			EnableSkills:     true,
			UserSkillsDir:    userSkillsDir,
			ProjectSkillsDir: projectSkillsDir,
			SettingSources:   []string{"project"},
		}),
	)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Test ListSkills
	skills := client.ListSkills()
	if len(skills) != 1 {
		t.Errorf("ListSkills() returned %d Skills, want 1", len(skills))
	}

	if len(skills) > 0 {
		if skills[0].Name != "Test Skill" {
			t.Errorf("ListSkills() Skill name = %q, want %q", skills[0].Name, "Test Skill")
		}
		if skills[0].Scope != "project" {
			t.Errorf("ListSkills() Skill scope = %q, want %q", skills[0].Scope, "project")
		}
	}
}

// TestClientGetSkill tests the GetSkill API.
func TestClientGetSkill(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create project Skills directory
	projectSkillsDir := filepath.Join(tmpDir, "project-skills")
	if err := os.MkdirAll(projectSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create project Skills directory: %v", err)
	}

	// Create test Skill
	skillDir := filepath.Join(projectSkillsDir, "research-analyst")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create Skill directory: %v", err)
	}

	skillContent := `---
name: Research Analyst
description: Perform fundamental analysis
---

# Research Analyst`

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Create empty user skills dir to isolate from real user skills
	userSkillsDir := filepath.Join(tmpDir, "user-skills")
	if err := os.MkdirAll(userSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create user Skills directory: %v", err)
	}

	// Create client with Skills enabled
	client, err := NewClient(
		WithSkills(SkillsConfig{
			EnableSkills:     true,
			UserSkillsDir:    userSkillsDir,
			ProjectSkillsDir: projectSkillsDir,
			SettingSources:   []string{"project"},
		}),
	)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Test GetSkill - existing Skill
	skill, err := client.GetSkill("Research Analyst")
	if err != nil {
		t.Errorf("GetSkill() unexpected error: %v", err)
	}
	if skill == nil {
		t.Fatalf("GetSkill() returned nil Skill")
	}
	if skill.Name != "Research Analyst" {
		t.Errorf("GetSkill() name = %q, want %q", skill.Name, "Research Analyst")
	}

	// Test GetSkill - non-existent Skill
	_, err = client.GetSkill("Nonexistent Skill")
	if err == nil {
		t.Errorf("GetSkill() expected error for non-existent Skill, got nil")
	}
	var skillNotFound *ErrSkillNotFound
	if !errors.As(err, &skillNotFound) {
		t.Errorf("GetSkill() error type = %T, want *ErrSkillNotFound", err)
	}
}

// TestClientReloadSkills tests the ReloadSkills API.
func TestClientReloadSkills(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create project Skills directory
	projectSkillsDir := filepath.Join(tmpDir, "project-skills")
	if err := os.MkdirAll(projectSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create project Skills directory: %v", err)
	}

	// Create initial Skill
	skillDir1 := filepath.Join(projectSkillsDir, "skill1")
	if err := os.MkdirAll(skillDir1, 0755); err != nil {
		t.Fatalf("failed to create Skill directory: %v", err)
	}

	skillContent1 := `---
name: Skill 1
description: First Skill
---

# Skill 1`

	if err := os.WriteFile(filepath.Join(skillDir1, "SKILL.md"), []byte(skillContent1), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Create empty user skills dir to isolate from real user skills
	userSkillsDir := filepath.Join(tmpDir, "user-skills")
	if err := os.MkdirAll(userSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create user Skills directory: %v", err)
	}

	// Create client with Skills enabled
	client, err := NewClient(
		WithSkills(SkillsConfig{
			EnableSkills:     true,
			UserSkillsDir:    userSkillsDir,
			ProjectSkillsDir: projectSkillsDir,
			SettingSources:   []string{"project"},
		}),
	)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Verify initial Skills
	skills := client.ListSkills()
	if len(skills) != 1 {
		t.Fatalf("ListSkills() returned %d Skills, want 1", len(skills))
	}

	// Add a new Skill
	skillDir2 := filepath.Join(projectSkillsDir, "skill2")
	if err := os.MkdirAll(skillDir2, 0755); err != nil {
		t.Fatalf("failed to create Skill directory: %v", err)
	}

	skillContent2 := `---
name: Skill 2
description: Second Skill
---

# Skill 2`

	if err := os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte(skillContent2), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Reload Skills
	if err := client.ReloadSkills(); err != nil {
		t.Errorf("ReloadSkills() unexpected error: %v", err)
	}

	// Verify Skills reloaded
	skills = client.ListSkills()
	if len(skills) != 2 {
		t.Errorf("ListSkills() after reload returned %d Skills, want 2", len(skills))
	}
}

// TestClientReloadSkillsDisabled tests ReloadSkills with Skills disabled.
func TestClientReloadSkillsDisabled(t *testing.T) {
	// Create client with Skills disabled
	client, err := NewClient(
		WithSkillsDisabled(),
	)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Attempt to reload Skills
	err = client.ReloadSkills()
	if err == nil {
		t.Errorf("ReloadSkills() expected error when Skills disabled, got nil")
	}
	var skillsDisabled *ErrSkillsDisabled
	if !errors.As(err, &skillsDisabled) {
		t.Errorf("ReloadSkills() error type = %T, want *ErrSkillsDisabled", err)
	}
}

// TestClientValidateSkill tests the ValidateSkill API.
func TestClientValidateSkill(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create valid SKILL.md
	validPath := filepath.Join(tmpDir, "valid-SKILL.md")
	validContent := `---
name: Valid Skill
description: A valid Skill
---

# Valid Skill`

	if err := os.WriteFile(validPath, []byte(validContent), 0644); err != nil {
		t.Fatalf("failed to write valid SKILL.md: %v", err)
	}

	// Create invalid SKILL.md (missing description)
	invalidPath := filepath.Join(tmpDir, "invalid-SKILL.md")
	invalidContent := `---
name: Invalid Skill
---

# Invalid Skill`

	if err := os.WriteFile(invalidPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to write invalid SKILL.md: %v", err)
	}

	// Create client
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Validate valid Skill
	if err := client.ValidateSkill(validPath); err != nil {
		t.Errorf("ValidateSkill() unexpected error for valid Skill: %v", err)
	}

	// Validate invalid Skill
	if err := client.ValidateSkill(invalidPath); err == nil {
		t.Errorf("ValidateSkill() expected error for invalid Skill, got nil")
	}
}

// TestClientSkillsConcurrency tests concurrent access to Skills.
func TestClientSkillsConcurrency(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create project Skills directory
	projectSkillsDir := filepath.Join(tmpDir, "project-skills")
	if err := os.MkdirAll(projectSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create project Skills directory: %v", err)
	}

	// Create test Skill
	skillDir := filepath.Join(projectSkillsDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create Skill directory: %v", err)
	}

	skillContent := `---
name: Test Skill
description: A test Skill
---

# Test Skill`

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Create empty user skills dir to isolate from real user skills
	userSkillsDir := filepath.Join(tmpDir, "user-skills")
	if err := os.MkdirAll(userSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create user Skills directory: %v", err)
	}

	// Create client with Skills enabled
	client, err := NewClient(
		WithSkills(SkillsConfig{
			EnableSkills:     true,
			UserSkillsDir:    userSkillsDir,
			ProjectSkillsDir: projectSkillsDir,
			SettingSources:   []string{"project"},
		}),
	)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Run concurrent operations
	done := make(chan bool)

	// Concurrent ListSkills
	for i := 0; i < 10; i++ {
		go func() {
			client.ListSkills()
			done <- true
		}()
	}

	// Concurrent GetSkill
	for i := 0; i < 10; i++ {
		go func() {
			client.GetSkill("Test Skill")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// No panic = success
}
