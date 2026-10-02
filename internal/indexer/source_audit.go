package indexer

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
)

type SearchAudit struct {
	Options   SearchOptions     `json:"options"`
	ElapsedMS int64             `json:"elapsed_ms"`
	Hashes    []string          `json:"hashes"`
	Error     string            `json:"error,omitempty"`
	Causes    map[string]string `json:"causes,omitempty"`
}

type CandidateDecision struct {
	InfoHash string      `json:"info_hash"`
	Title    string      `json:"title"`
	Accepted bool        `json:"accepted"`
	Batch    bool        `json:"batch"`
	Reason   string      `json:"reason"`
	Language LanguageTag `json:"language"`
}

// SourceAudit is a portable corpus: raw candidates are retained, including every
// rejected release. Replay never calls an indexer or downloads torrent payloads.
type SourceAudit struct {
	Identity   EpisodeIdentity         `json:"identity"`
	CapturedAt time.Time               `json:"captured_at"`
	ElapsedMS  int64                   `json:"elapsed_ms"`
	Searches   []SearchAudit           `json:"searches"`
	Items      []TorrentItem           `json:"items"`
	Decisions  []CandidateDecision     `json:"decisions"`
	Result     *EpisodeSourcesResponse `json:"result,omitempty"`
	Error      string                  `json:"error,omitempty"`
}

type AuditProvider struct {
	Provider
	mu       sync.Mutex
	searches []SearchAudit
	items    map[string]TorrentItem
}

func NewAuditProvider(provider Provider) *AuditProvider {
	return &AuditProvider{Provider: provider, items: map[string]TorrentItem{}}
}

func (p *AuditProvider) Search(ctx context.Context, opts SearchOptions) ([]TorrentItem, error) {
	start := time.Now()
	items, err := p.Provider.Search(ctx, opts)
	audit := SearchAudit{Options: opts, ElapsedMS: time.Since(start).Milliseconds(), Hashes: []string{}}
	if err != nil {
		audit.Error = diagnostics.Redact(err.Error())
		if partial, ok := err.(*PartialError); ok {
			audit.Causes = partial.Causes
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		hash := strings.ToLower(strings.TrimSpace(item.InfoHash))
		if hash == "" {
			continue
		}
		item.InfoHash = hash
		audit.Hashes = append(audit.Hashes, hash)
		if previous, ok := p.items[hash]; !ok || item.Seeders > previous.Seeders {
			p.items[hash] = item
		}
	}
	p.searches = append(p.searches, audit)
	return items, err
}

func (p *AuditProvider) Snapshot(identity EpisodeIdentity) SourceAudit {
	p.mu.Lock()
	defer p.mu.Unlock()
	a := SourceAudit{Identity: identity, CapturedAt: time.Now().UTC(), Searches: append([]SearchAudit(nil), p.searches...), Items: []TorrentItem{}}
	for _, item := range p.items {
		a.Items = append(a.Items, item)
	}
	sort.Slice(a.Items, func(i, j int) bool { return a.Items[i].InfoHash < a.Items[j].InfoHash })
	sort.Slice(a.Searches, func(i, j int) bool {
		x, y := a.Searches[i].Options, a.Searches[j].Options
		if x.Query != y.Query {
			return x.Query < y.Query
		}
		if x.Category != y.Category {
			return x.Category < y.Category
		}
		return x.Page < y.Page
	})
	a.Decisions = AuditDecisions(identity, a.Items)
	return a
}

func AuditDecisions(identity EpisodeIdentity, items []TorrentItem) []CandidateDecision {
	matcher := NewEpisodeMatcher(identity)
	decisions := make([]CandidateDecision, 0, len(items))
	for _, item := range items {
		accepted, batch, reason := matcher.Match(item.Title)
		language, _, _ := ClassifyLanguage(item.Title)
		decisions = append(decisions, CandidateDecision{item.InfoHash, item.Title, accepted, batch, reason, language})
	}
	return decisions
}

// ReplaySourceAudit only repeats filtering/ranking. Preserve the original
// discovery completeness; a finite corpus cannot prove exhaustive discovery.
func ReplaySourceAudit(a SourceAudit) SourceAudit {
	a.Decisions = AuditDecisions(a.Identity, a.Items)
	res := EpisodeSourcesResponse{AnimeTitle: a.Identity.Titles[0], SeasonNumber: a.Identity.SeasonNumber, EpisodeNumber: a.Identity.EpisodeNumber, Sources: []EpisodeSource{}, Partial: true, Warning: "Rejeu d'un corpus : exhaustivité de la découverte non vérifiée."}
	if a.Result != nil {
		res.Partial = a.Result.Partial
		res.Warning = a.Result.Warning
	}
	for i, d := range a.Decisions {
		if !d.Accepted {
			continue
		}
		source := RankSource(a.Items[i], a.Identity, d.Batch)
		res.Sources = append(res.Sources, source)
		if source.IsFrench {
			res.FrenchSources++
		}
	}
	SortEpisodeSources(res.Sources)
	res.TotalSources = len(res.Sources)
	a.Result = &res
	return a
}
