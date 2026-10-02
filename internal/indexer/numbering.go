package indexer

// NumberingOverride describes confirmed release numbering for a catalog media ID.
// Keep this registry versioned; add entries only with verified release evidence.
type NumberingOverride struct {
	Titles         []string
	TaggedOffset   int
	Season         int
	Part           int
	AbsoluteOffset int
}

var NumberingOverrides = map[int]NumberingOverride{
	// Nyaa releases number the one-episode 1st STAGE as S06E01;
	// the 2nd–3rd STAGE continuation starts at S06E02 (2026-09-25).
	210482: {Season: 6, AbsoluteOffset: 1, TaggedOffset: 1, Titles: []string{"Steel Ball Run", "JoJo no Kimyou na Bouken Steel Ball Run"}},
}
