// Package skills loads agent-facing SKILL.md instructions from the embedded
// seeds and the operator's user directory.
//
// The cacheable system prompt carries only names and descriptions; the body is
// read on demand through the read-only load_skill tool. A malformed skill is
// never silently skipped: it becomes a closed SkillFault the Settings surface
// can show. Limits and symlink refusal are part of the contract, not a
// best-effort loader.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	// MaxSkills bounds the advertised surface, so a hostile user directory
	// cannot turn the prompt into a catalogue.
	MaxSkills = 64
	// MaxSkillBytes bounds one SKILL.md, body included.
	MaxSkillBytes = 16 * 1024
	// MaxDescription is the advisory line length in the system prompt.
	MaxDescription = 200
	// MaxName bounds the name charset/length.
	MaxName = 64
)

var (
	nameRe        = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)
	toolNameRe    = regexp.MustCompile(`^[a-z0-9_]+$`)
	allowedFields = map[string]bool{"name": true, "description": true, "role": true, "tools": true}
)

// Source says where a skill came from. Seeds are embedded in the binary; user
// skills are operator-owned files.
type Source string

const (
	SourceSeed Source = "seed"
	SourceUser Source = "user"
)

// Skill is one parsed SKILL.md. Body is deliberately excluded from JSON so a
// settings/catalog response can never leak the instructions wholesale.
type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Role        string   `json:"role"`
	Tools       []string `json:"tools"`
	Source      Source   `json:"source"`
	Enabled     bool     `json:"enabled"`
	Body        string   `json:"-"`
}

// FaultCode is the closed vocabulary shown in Settings.
type FaultCode string

const (
	FaultNameInvalid        FaultCode = "skill_name_invalid"
	FaultDescriptionInvalid FaultCode = "skill_description_invalid"
	FaultFrontmatterInvalid FaultCode = "skill_frontmatter_invalid"
	FaultToolInvalid        FaultCode = "skill_tool_invalid"
	FaultRoleInvalid        FaultCode = "skill_role_invalid"
	FaultTooLarge           FaultCode = "skill_too_large"
	FaultTooMany            FaultCode = "skill_too_many"
	FaultDuplicate          FaultCode = "skill_duplicate"
	FaultSymlink            FaultCode = "skill_symlink"
	FaultUnreadable         FaultCode = "skill_unreadable"
	FaultMissing            FaultCode = "skill_missing"
	FaultUnknown            FaultCode = "skill_unknown"
)

// Fault describes one rejected skill. Skill is the directory name (never an
// absolute path), so the Settings response cannot reveal the home layout.
type Fault struct {
	Code   FaultCode `json:"code"`
	Skill  string    `json:"skill"`
	Detail string    `json:"detail"`
}

// Limits is advertised alongside the catalogue so the Settings surface can
// explain a refusal without duplicating constants.
type Limits struct {
	MaxSkills      int `json:"max_skills"`
	MaxSkillBytes  int `json:"max_skill_bytes"`
	MaxDescription int `json:"max_description"`
}

var advertisedLimits = Limits{MaxSkills: MaxSkills, MaxSkillBytes: MaxSkillBytes, MaxDescription: MaxDescription}

// Catalog is the public, body-free view.
type Catalog struct {
	Skills []Skill `json:"skills"`
	Faults []Fault `json:"faults"`
	Limits Limits  `json:"limits"`
}

// Store is the loaded, immutable skill set. Load once at startup; a reload is
// a new Store, never a mutation under readers.
type Store struct {
	mu       sync.RWMutex
	skills   []Skill
	faults   []Fault
	byName   map[string]Skill
	disabled map[string]bool
}

// Options controls loading. UserDir may be empty to load seeds only.
type Options struct {
	UserDir string
}

// DefaultUserDir returns ~/.kubona/skills, overridable for tests and packaging
// via KUBONA_SKILLS_DIR.
func DefaultUserDir() string {
	if dir := strings.TrimSpace(os.Getenv("KUBONA_SKILLS_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kubona", "skills")
}

//go:embed seeds/*/SKILL.md
var seedFS embed.FS

// Load reads the embedded seeds and, when present, the user directory. An
// absent user directory is normal (no fault); every malformed entry becomes a
// fault and the rest still load.
func Load(opts Options) *Store {
	s := &Store{byName: map[string]Skill{}, disabled: map[string]bool{}}
	s.loadFS(seedFS, "seeds", SourceSeed)
	if opts.UserDir != "" {
		s.loadDir(opts.UserDir)
	}
	sort.Slice(s.skills, func(i, j int) bool { return s.skills[i].Name < s.skills[j].Name })
	sort.Slice(s.faults, func(i, j int) bool {
		if s.faults[i].Skill == s.faults[j].Skill {
			return s.faults[i].Code < s.faults[j].Code
		}
		return s.faults[i].Skill < s.faults[j].Skill
	})
	return s
}

func (s *Store) loadFS(fsys fs.FS, root string, source Source) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		s.addFault(FaultUnreadable, root, "seed directory unreadable")
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := root + "/" + name + "/SKILL.md"
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			s.addFault(FaultMissing, name, "SKILL.md missing or unreadable")
			continue
		}
		s.accept(raw, name, source)
	}
}

func (s *Store) loadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		s.addFault(FaultUnreadable, "user", "skills directory unreadable")
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		skillDir := filepath.Join(dir, name)
		info, lerr := os.Lstat(skillDir)
		if lerr != nil {
			s.addFault(FaultUnreadable, name, "skill directory unreadable")
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			s.addFault(FaultSymlink, name, "skill directory is a symlink; refused")
			continue
		}
		if !info.IsDir() {
			continue
		}
		path := filepath.Join(skillDir, "SKILL.md")
		info, lerr = os.Lstat(path)
		if lerr != nil {
			s.addFault(FaultMissing, name, "SKILL.md missing")
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			s.addFault(FaultSymlink, name, "SKILL.md is a symlink; refused")
			continue
		}
		if info.Size() > MaxSkillBytes {
			s.addFault(FaultTooLarge, name, fmt.Sprintf("SKILL.md is %d bytes; limit %d", info.Size(), MaxSkillBytes))
			continue
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			s.addFault(FaultUnreadable, name, "SKILL.md unreadable")
			continue
		}
		s.accept(raw, name, SourceUser)
	}
}

func (s *Store) accept(raw []byte, dirName string, source Source) {
	if len(raw) > MaxSkillBytes {
		s.addFault(FaultTooLarge, dirName, fmt.Sprintf("SKILL.md is %d bytes; limit %d", len(raw), MaxSkillBytes))
		return
	}
	skill, fault := parse(raw, dirName, source)
	if fault != nil {
		s.faults = append(s.faults, *fault)
		return
	}
	if _, exists := s.byName[skill.Name]; exists {
		s.addFault(FaultDuplicate, skill.Name, "name already loaded")
		return
	}
	if len(s.skills) >= MaxSkills {
		s.addFault(FaultTooMany, skill.Name, fmt.Sprintf("skill limit %d reached", MaxSkills))
		return
	}
	s.skills = append(s.skills, skill)
	s.byName[skill.Name] = skill
}

func (s *Store) addFault(code FaultCode, skill, detail string) {
	s.faults = append(s.faults, Fault{Code: code, Skill: skill, Detail: detail})
}

func parse(raw []byte, dirName string, source Source) (Skill, *Fault) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Skill{}, &Fault{Code: FaultFrontmatterInvalid, Skill: dirName, Detail: "SKILL.md must start with a --- frontmatter block"}
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return Skill{}, &Fault{Code: FaultFrontmatterInvalid, Skill: dirName, Detail: "frontmatter block has no closing ---"}
	}
	front := rest[:end]
	body := strings.TrimSpace(rest[end+len("\n---\n"):])
	if body == "" {
		return Skill{}, &Fault{Code: FaultFrontmatterInvalid, Skill: dirName, Detail: "skill body is empty"}
	}

	fields := map[string]string{}
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Skill{}, &Fault{Code: FaultFrontmatterInvalid, Skill: dirName, Detail: "frontmatter line is not key: value"}
		}
		key = strings.TrimSpace(key)
		if !allowedFields[key] {
			return Skill{}, &Fault{Code: FaultFrontmatterInvalid, Skill: dirName, Detail: "unknown frontmatter field " + key}
		}
		if _, dup := fields[key]; dup {
			return Skill{}, &Fault{Code: FaultFrontmatterInvalid, Skill: dirName, Detail: "duplicate frontmatter field " + key}
		}
		fields[key] = strings.TrimSpace(value)
	}

	name := fields["name"]
	if name == "" {
		name = dirName
	}
	if !nameRe.MatchString(name) || len(name) > MaxName {
		return Skill{}, &Fault{Code: FaultNameInvalid, Skill: dirName, Detail: "name must be lowercase [a-z0-9-], 1-64 chars, no leading/trailing hyphen"}
	}
	if name != dirName {
		return Skill{}, &Fault{Code: FaultNameInvalid, Skill: dirName, Detail: "frontmatter name must match the directory name"}
	}
	description := fields["description"]
	if description == "" || len([]rune(description)) > MaxDescription || strings.ContainsAny(description, "\r\n") {
		return Skill{}, &Fault{Code: FaultDescriptionInvalid, Skill: name, Detail: fmt.Sprintf("description must be one line, 1-%d chars", MaxDescription)}
	}
	role := fields["role"]
	if role == "" {
		role = "copilot"
	}
	if !nameRe.MatchString(role) {
		return Skill{}, &Fault{Code: FaultRoleInvalid, Skill: name, Detail: "role must be lowercase [a-z0-9-]"}
	}
	var tools []string
	seen := map[string]bool{}
	for _, part := range strings.Split(fields["tools"], ",") {
		tool := strings.TrimSpace(part)
		if tool == "" {
			continue
		}
		if !toolNameRe.MatchString(tool) {
			return Skill{}, &Fault{Code: FaultToolInvalid, Skill: name, Detail: "invalid tool name " + tool}
		}
		if seen[tool] {
			continue
		}
		seen[tool] = true
		tools = append(tools, tool)
	}
	return Skill{Name: name, Description: description, Role: role, Tools: tools, Source: source, Body: body}, nil
}

// Validate parses one candidate SKILL.md without writing anything. The
// directory name must match the frontmatter name (the same rule the loader
// applies), so an install cannot smuggle a second identity.
func Validate(raw []byte, dirName string) (Skill, *Fault) {
	if len(raw) > MaxSkillBytes {
		return Skill{}, &Fault{Code: FaultTooLarge, Skill: dirName, Detail: fmt.Sprintf("SKILL.md is %d bytes; limit %d", len(raw), MaxSkillBytes)}
	}
	return parse(raw, dirName, SourceUser)
}

// Install writes one validated user skill atomically. It refuses symlinks and
// path separators; the name is the directory name and must match frontmatter.
func Install(userDir, name string, raw []byte) error {
	if userDir == "" {
		return fmt.Errorf("user skill directory is not configured")
	}
	if !nameRe.MatchString(name) || len(name) > MaxName {
		return fmt.Errorf("invalid skill name %q", name)
	}
	if _, fault := Validate(raw, name); fault != nil {
		return fmt.Errorf("%s: %s", fault.Code, fault.Detail)
	}
	dir := filepath.Join(userDir, name)
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("skill directory %s is a symlink; refused", name)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "SKILL.md")
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("skill file %s is a symlink; refused", name)
	}
	tmp, err := os.CreateTemp(dir, ".SKILL.md-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Remove deletes one user skill directory. The API refuses seed skills before
// calling this; this function still refuses invalid names and symlinks.
func Remove(userDir, name string) error {
	if userDir == "" {
		return fmt.Errorf("user skill directory is not configured")
	}
	if !nameRe.MatchString(name) || len(name) > MaxName {
		return fmt.Errorf("invalid skill name %q", name)
	}
	dir := filepath.Join(userDir, name)
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("skill directory %s is a symlink; refused", name)
	}
	if !info.IsDir() {
		return fmt.Errorf("skill %s is not a directory", name)
	}
	return os.RemoveAll(dir)
}

// SourceOf reports the loaded source for one skill.
func (s *Store) SourceOf(name string) (Source, bool) {
	if s == nil {
		return "", false
	}
	skill, ok := s.byName[name]
	return skill.Source, ok
}

// Public returns the role-filtered catalogue without bodies or paths.
// SetDisabled replaces the operator-disabled set. Unknown names are ignored:
// a stale persisted row must not hide a newly installed skill.
func (s *Store) SetDisabled(names []string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disabled = make(map[string]bool, len(names))
	for _, name := range names {
		if _, ok := s.byName[name]; ok {
			s.disabled[name] = true
		}
	}
}

// SetEnabled flips one skill and reports whether it exists. The caller
// persists the decision; the store only owns the in-process view.
func (s *Store) SetEnabled(name string, enabled bool) error {
	if s == nil {
		return fmt.Errorf("skill store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byName[name]; !ok {
		return fmt.Errorf("unknown skill: %s", name)
	}
	if enabled {
		delete(s.disabled, name)
	} else {
		s.disabled[name] = true
	}
	return nil
}

func (s *Store) isDisabled(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disabled[name]
}

func (s *Store) Public(role string) Catalog {
	out := Catalog{Skills: []Skill{}, Faults: []Fault{}, Limits: advertisedLimits}
	if s == nil {
		return out
	}
	out.Faults = append(out.Faults, s.faults...)
	for _, skill := range s.skills {
		if role != "" && skill.Role != role {
			continue
		}
		skill.Body = ""
		skill.Enabled = !s.isDisabled(skill.Name)
		out.Skills = append(out.Skills, skill)
	}
	return out
}

// ForRole returns skills available to a caller role, sorted by name.
func (s *Store) ForRole(role string) []Skill {
	if s == nil {
		return nil
	}
	var out []Skill
	for _, skill := range s.skills {
		if s.isDisabled(skill.Name) {
			continue
		}
		if role == "" || skill.Role == role {
			out = append(out, skill)
		}
	}
	return out
}

// PromptSection advertises names and descriptions only. Bodies stay behind
// load_skill so the cacheable prefix stays small and stable.
func (s *Store) PromptSection(role string) string {
	list := s.ForRole(role)
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nSkills available on demand (descriptions only; call load_skill with the exact name to read the full instructions):\n")
	for _, skill := range list {
		fmt.Fprintf(&b, "- %s: %s\n", skill.Name, skill.Description)
	}
	return b.String()
}

// LoadBody resolves one skill body for a role. Unknown names and role
// mismatches both return FaultUnknown (never disclose another role's skills).
func (s *Store) LoadBody(name, role string) (string, *Fault) {
	if s == nil {
		return "", &Fault{Code: FaultUnknown, Skill: name, Detail: "skill store unavailable"}
	}
	if !nameRe.MatchString(name) {
		return "", &Fault{Code: FaultNameInvalid, Skill: name, Detail: "invalid skill name"}
	}
	skill, ok := s.byName[name]
	if !ok || (role != "" && skill.Role != role) {
		return "", &Fault{Code: FaultUnknown, Skill: name, Detail: "no such skill for this role"}
	}
	if s.isDisabled(name) {
		return "", &Fault{Code: FaultUnknown, Skill: name, Detail: "skill is disabled by the operator"}
	}
	return skill.Body, nil
}

// Len reports the loaded skill count (tests and diagnostics).
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	return len(s.skills)
}

// Faults reports rejected entries (tests and diagnostics).
func (s *Store) Faults() []Fault {
	if s == nil {
		return nil
	}
	return append([]Fault(nil), s.faults...)
}
