package gocomments

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

type Kind string

const (
	KindLine      Kind = "line"
	KindBlock     Kind = "block"
	KindDoc       Kind = "doc"
	KindDirective Kind = "directive"
)

type Comment struct {
	ID          string
	File        string
	Line        int
	EndLine     int
	Kind        Kind
	Protected   bool
	Reason      string
	Text        string
	AbsPath     string
	StartOffset int
	EndOffset   int
	DeclName    string
	Trailing    bool
}

type rawEntry struct {
	file        string
	absPath     string
	startOffset int
	endOffset   int
	line        int
	endLine     int
	rawText     string
	kind        Kind
	protected   bool
	reason      string
	declName    string
	trailing    bool
}

func computeIDs(entries []rawEntry) []Comment {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].file != entries[j].file {
			return entries[i].file < entries[j].file
		}
		return entries[i].startOffset < entries[j].startOffset
	})

	occurrence := make(map[string]int, len(entries))
	out := make([]Comment, 0, len(entries))
	for _, e := range entries {
		key := e.file + "\x00" + e.rawText
		idx := occurrence[key]
		occurrence[key] = idx + 1
		out = append(out, Comment{
			ID:          hashID(e.file, e.rawText, idx),
			File:        e.file,
			Line:        e.line,
			EndLine:     e.endLine,
			Kind:        e.kind,
			Protected:   e.protected,
			Reason:      e.reason,
			Text:        escapeText(e.rawText),
			AbsPath:     e.absPath,
			StartOffset: e.startOffset,
			EndOffset:   e.endOffset,
			DeclName:    e.declName,
			Trailing:    e.trailing,
		})
	}
	return out
}

func hashID(relPath, normalizedText string, occurrenceIndex int) string {
	sum := sha256.Sum256([]byte(relPath + "\x00" + normalizedText + "\x00" + strconv.Itoa(occurrenceIndex)))
	return hex.EncodeToString(sum[:])[:12]
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
