package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnknownSkill: no skill of that name is there to launch.
var ErrUnknownSkill = errors.New("unknown skill")

// Skill is a skill found by name: the folder its SKILL.md is in and the instructions that follow
// the file's front matter.
type Skill struct {
	Name, Dir, Body string
}

// FindSkill is the skill named name among the project's own (.claude/skills/<name>/SKILL.md under
// root), with its instructions: the file without its front matter and the blank lines after it.
// It returns ErrUnknownSkill when there is none by that name.
//
// sr:capability skill-tool
func FindSkill(root, name string) (Skill, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return Skill{}, ErrUnknownSkill
	}
	dir := filepath.Join(root, ".claude", "skills", name)
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return Skill{}, ErrUnknownSkill
	}
	return Skill{Name: name, Dir: dir, Body: withoutFrontMatter(string(data))}, nil
}

// Instructions is what the skill puts before the agent: where the skill lives and its body, and the
// arguments the call gave, when it gave any.
func (s Skill) Instructions(args string) string {
	text := "Base directory for this skill: " + s.Dir + "\n\n" + s.Body
	if args != "" {
		text += "\n\nARGUMENTS: " + args
	}
	return text
}

// withoutFrontMatter is a file's text after its "---" fenced front matter and the blank lines that follow.
func withoutFrontMatter(text string) string {
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return text
	}
	return strings.TrimLeft(rest[end+len("\n---\n"):], "\n")
}
