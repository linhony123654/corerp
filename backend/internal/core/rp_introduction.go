package core

import "strings"

// ExplicitSelfIntroduction is a conservative authority check for the
// introduce_self hint. A claimed introduction only teaches a canonical name
// when the accepted speech actually contains that name as self-identification.
// Other forms of identity evidence require their own sourced event.
func ExplicitSelfIntroduction(text, canonicalName string) bool {
	name := strings.TrimSpace(canonicalName)
	if name == "" || len([]rune(name)) > 64 {
		return false
	}
	clause := strings.TrimSpace(text)
	if cut := strings.IndexAny(clause, "，,。！!？?；;\n"); cut >= 0 {
		clause = clause[:cut]
	}
	clause = strings.TrimSpace(clause)
	for _, marker := range []string{"我叫", "我是", "我的名字是", "在下是", "鄙人是", "I'm ", "I am ", "My name is "} {
		at := strings.Index(strings.ToLower(clause), strings.ToLower(marker))
		if at != 0 {
			continue
		}
		after := []rune(strings.TrimSpace(clause[at+len(marker):]))
		if len(after) > 24 {
			after = after[:24]
		}
		part := string(after)
		idx := strings.Index(strings.ToLower(part), strings.ToLower(name))
		if idx < 0 {
			continue
		}
		following := strings.TrimSpace(part[idx+len(name):])
		if !strings.HasPrefix(following, "的") {
			return true
		}
	}
	return false
}
