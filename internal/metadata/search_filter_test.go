package metadata

import (
	"reflect"
	"testing"
)

func TestSplitFilterTermsRoutesGenresAndTags(t *testing.T) {
	genres, tags := splitFilterTerms([]string{"adventure", "Isekai", "ROMANCE", "mahou shoujo", "sci-fi", "nonsense", "adventure", " "})
	if !reflect.DeepEqual(genres, []string{"Adventure", "Mahou Shoujo", "Romance", "Sci-Fi"}) {
		t.Fatalf("genres: %v", genres)
	}
	if !reflect.DeepEqual(tags, []string{"Isekai"}) {
		t.Fatalf("tags: %v", tags)
	}
}

func TestSplitFilterTermsIsBounded(t *testing.T) {
	names := []string{"action", "adventure", "comedy", "drama", "ecchi", "fantasy", "horror", "mecha", "music", "mystery"}
	genres, tags := splitFilterTerms(names)
	if len(genres)+len(tags) != maxFilterTerms {
		t.Fatalf("expected at most %d terms, got %d", maxFilterTerms, len(genres)+len(tags))
	}
}
