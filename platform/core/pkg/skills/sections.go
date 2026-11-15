package skills

import (
	"sort"
	"strings"
)

// FaultSectionUnknown is closed like the other faults: the detail names the
// available headings so the copilot can retry without a second full body.
const FaultSectionUnknown FaultCode = "skill_section_unknown"

// Section names a part of a skill body. An empty name means the whole body.
func sectionKey(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimLeft(name, "#")
	return strings.ToLower(strings.TrimSpace(name))
}

// splitSections indexes every markdown heading (any level) by normalized
// title. The preamble before the first heading is keyed "".
func splitSections(body string) map[string]string {
	out := map[string]string{"": body}
	var currentKey string
	var current strings.Builder
	flush := func() {
		if currentKey != "" || current.Len() > 0 {
			out[currentKey] = strings.TrimSpace(current.String())
		}
	}
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			if key := sectionKey(trim); key != "" {
				flush()
				currentKey = key
				current.Reset()
				current.WriteString(line)
				current.WriteString("\n")
				continue
			}
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	flush()
	return out
}

// SectionNames lists headings in source order, without the leading #s.
func SectionNames(body string) []string {
	var names []string
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "#") {
			continue
		}
		name := strings.TrimSpace(strings.TrimLeft(trim, "#"))
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// LoadSection resolves one named section for a role. Unknown names return a
// closed fault listing the available sections in source order.
func (s *Store) LoadSection(name, role, section string) (string, *Fault) {
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
	key := sectionKey(section)
	if key == "" {
		return skill.Body, nil
	}
	sections := splitSections(skill.Body)
	if body, ok := sections[key]; ok {
		return body, nil
	}
	names := SectionNames(skill.Body)
	if len(names) == 0 {
		return "", &Fault{Code: FaultSectionUnknown, Skill: name, Detail: "this skill has no sections; omit section to read the whole body"}
	}
	return "", &Fault{
		Code:   FaultSectionUnknown,
		Skill:  name,
		Detail: "unknown section; available: " + strings.Join(names, ", "),
	}
}

// SortedSectionNames is a stable diagnostic helper.
func SortedSectionNames(body string) []string {
	out := SectionNames(body)
	sort.Strings(out)
	return out
}
