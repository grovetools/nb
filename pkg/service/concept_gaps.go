package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// conceptGapsFileName is the append-only JSONL store inside a concepts dir.
const conceptGapsFileName = ".gaps.jsonl"

// Gap kinds.
const (
	ConceptGapKindPreset  = "preset"  // file belongs to a concept whose preset is missing it
	ConceptGapKindConcept = "concept" // no concept covers the file at all
)

// ConceptGap records a file flagged as essential-but-missing from a concept's
// derived context.
type ConceptGap struct {
	ID         string    `json:"id"`         // short hash of file+concept+ts
	File       string    `json:"file"`       // the essential-but-missing file (abs or workspace-rel)
	ConceptID  string    `json:"concept_id"` // owning concept if known, "" = unknown → concept gap
	Kind       string    `json:"kind"`       // "preset" | "concept"
	Reason     string    `json:"reason"`     // rater's one-liner: why essential
	Source     string    `json:"source"`     // who flagged: e.g. "flow-context-tuner", job id, manual
	CreatedAt  time.Time `json:"created_at"`
	Resolved   bool      `json:"resolved"`
	ResolvedBy string    `json:"resolved_by,omitempty"` // e.g. "added-to-preset", "new-concept:<id>", "rejected"
}

// ConceptGapListFilter selects which gaps ListConceptGaps returns.
type ConceptGapListFilter struct {
	IncludeResolved bool   // include resolved gaps alongside unresolved ones
	OnlyResolved    bool   // return resolved gaps only
	ConceptID       string // restrict to one concept ("" = all)
}

// RecordConceptGap appends a gap to the current workspace's gap store and
// returns it with ID, kind, and timestamp filled in.
func (s *Service) RecordConceptGap(ctx *WorkspaceContext, gap ConceptGap) (ConceptGap, error) {
	path, err := s.conceptGapsPath(ctx)
	if err != nil {
		return ConceptGap{}, err
	}
	return recordConceptGap(path, gap, time.Now())
}

// ListConceptGaps returns gaps from the current workspace's gap store.
// A missing store file yields an empty list.
func (s *Service) ListConceptGaps(ctx *WorkspaceContext, filter ConceptGapListFilter) ([]ConceptGap, error) {
	path, err := s.conceptGapsPath(ctx)
	if err != nil {
		return nil, err
	}
	gaps, err := readConceptGaps(path)
	if err != nil {
		return nil, err
	}
	return filterConceptGaps(gaps, filter), nil
}

// ResolveConceptGap marks the gap with the given ID (or unique ID prefix) as
// resolved and rewrites the store.
func (s *Service) ResolveConceptGap(ctx *WorkspaceContext, id, resolvedBy string) (ConceptGap, error) {
	path, err := s.conceptGapsPath(ctx)
	if err != nil {
		return ConceptGap{}, err
	}
	return resolveConceptGap(path, id, resolvedBy)
}

func (s *Service) conceptGapsPath(ctx *WorkspaceContext) (string, error) {
	conceptsDir, err := s.GetConceptsDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(conceptsDir, conceptGapsFileName), nil
}

func recordConceptGap(path string, gap ConceptGap, now time.Time) (ConceptGap, error) {
	if strings.TrimSpace(gap.File) == "" {
		return ConceptGap{}, fmt.Errorf("gap file is required")
	}
	switch gap.Kind {
	case "":
		if gap.ConceptID != "" {
			gap.Kind = ConceptGapKindPreset
		} else {
			gap.Kind = ConceptGapKindConcept
		}
	case ConceptGapKindPreset:
		if gap.ConceptID == "" {
			return ConceptGap{}, fmt.Errorf("gap kind %q requires a concept id", ConceptGapKindPreset)
		}
	case ConceptGapKindConcept:
	default:
		return ConceptGap{}, fmt.Errorf("invalid gap kind %q (want %q or %q)", gap.Kind, ConceptGapKindPreset, ConceptGapKindConcept)
	}

	gap.CreatedAt = now
	gap.Resolved = false
	gap.ResolvedBy = ""
	sum := sha256.Sum256([]byte(gap.File + "|" + gap.ConceptID + "|" + gap.Reason + "|" + now.Format(time.RFC3339Nano)))
	gap.ID = hex.EncodeToString(sum[:])[:12]

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ConceptGap{}, fmt.Errorf("ensure concepts directory: %w", err)
	}
	data, err := json.Marshal(gap)
	if err != nil {
		return ConceptGap{}, fmt.Errorf("marshal gap: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return ConceptGap{}, fmt.Errorf("open gap store: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return ConceptGap{}, fmt.Errorf("append gap: %w", err)
	}
	return gap, nil
}

func readConceptGaps(path string) ([]ConceptGap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read gap store: %w", err)
	}

	var gaps []ConceptGap
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var gap ConceptGap
		if err := json.Unmarshal([]byte(line), &gap); err != nil {
			return nil, fmt.Errorf("parse gap store %s line %d: %w", path, i+1, err)
		}
		gaps = append(gaps, gap)
	}
	return gaps, nil
}

func filterConceptGaps(gaps []ConceptGap, filter ConceptGapListFilter) []ConceptGap {
	filtered := make([]ConceptGap, 0, len(gaps))
	for _, g := range gaps {
		if filter.OnlyResolved && !g.Resolved {
			continue
		}
		if !filter.OnlyResolved && !filter.IncludeResolved && g.Resolved {
			continue
		}
		if filter.ConceptID != "" && g.ConceptID != filter.ConceptID {
			continue
		}
		filtered = append(filtered, g)
	}
	return filtered
}

func resolveConceptGap(path, id, resolvedBy string) (ConceptGap, error) {
	if strings.TrimSpace(id) == "" {
		return ConceptGap{}, fmt.Errorf("gap id is required")
	}
	gaps, err := readConceptGaps(path)
	if err != nil {
		return ConceptGap{}, err
	}

	// Exact match first, then a unique ID prefix.
	target := -1
	for i, g := range gaps {
		if g.ID == id {
			target = i
			break
		}
	}
	if target == -1 {
		for i, g := range gaps {
			if strings.HasPrefix(g.ID, id) {
				if target != -1 {
					return ConceptGap{}, fmt.Errorf("gap id prefix %q is ambiguous", id)
				}
				target = i
			}
		}
	}
	if target == -1 {
		return ConceptGap{}, fmt.Errorf("gap %q not found", id)
	}

	gaps[target].Resolved = true
	gaps[target].ResolvedBy = resolvedBy

	var sb strings.Builder
	for _, g := range gaps {
		data, err := json.Marshal(g)
		if err != nil {
			return ConceptGap{}, fmt.Errorf("marshal gap: %w", err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o644); err != nil {
		return ConceptGap{}, fmt.Errorf("write gap store: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return ConceptGap{}, fmt.Errorf("replace gap store: %w", err)
	}
	return gaps[target], nil
}
