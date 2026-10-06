package metadata

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"
)

var (
	ErrAniListUserNotFound = errors.New("anilist user not found")
	ErrAniListListPrivate  = errors.New("anilist list is private")
)

// anilistUserName is AniList's own rule for user names; anything else cannot exist there.
var anilistUserName = regexp.MustCompile(`^[A-Za-z0-9_]{2,20}$`)

// AniListListEntry is one title of a user's AniList list.
type AniListListEntry struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// AniListList is the answer of AniListUserList. Truncated is set when the list was longer than the cap.
type AniListList struct {
	Entries   []AniListListEntry `json:"entries"`
	Truncated bool               `json:"truncated"`
}

// A list changes often and must not be served from the forever-kept answer store.
const anilistListMaxAge = 10 * time.Minute

const anilistUserListQuery = `query ($user: String) {
  MediaListCollection(userName: $user, type: ANIME, status_in: [CURRENT, PLANNING, PAUSED, REPEATING]) {
    lists { entries { media { id format isAdult title { romaji english } } } }
  }
}`

// AniListUserList returns the titles a user is watching or plans to watch: one AniList request,
// at most max entries, adult titles dropped, each title once. It spends a single governor token.
func (s *AnimeCatalogService) AniListUserList(ctx context.Context, user string, max int) (*AniListList, error) {
	if !anilistUserName.MatchString(user) {
		return nil, ErrAniListUserNotFound
	}
	var parsed struct {
		Data struct {
			MediaListCollection struct {
				Lists []struct {
					Entries []struct {
						Media *struct {
							ID      int    `json:"id"`
							Format  string `json:"format"`
							IsAdult bool   `json:"isAdult"`
							Title   struct {
								Romaji  string `json:"romaji"`
								English string `json:"english"`
							} `json:"title"`
						} `json:"media"`
					} `json:"entries"`
				} `json:"lists"`
			} `json:"MediaListCollection"`
		} `json:"data"`
	}
	if err := s.anilist.postFresh(ctx, anilistUserListQuery, map[string]any{"user": user}, &parsed, anilistListMaxAge); err != nil {
		var st *anilistStatusError
		if errors.As(err, &st) {
			switch st.Code {
			case http.StatusNotFound:
				return nil, ErrAniListUserNotFound
			case http.StatusForbidden:
				return nil, ErrAniListListPrivate
			}
		}
		return nil, err
	}
	out := &AniListList{Entries: []AniListListEntry{}}
	seen := map[int]bool{}
	for _, list := range parsed.Data.MediaListCollection.Lists {
		for _, e := range list.Entries {
			if e.Media == nil || e.Media.IsAdult || seen[e.Media.ID] {
				continue
			}
			seen[e.Media.ID] = true
			if len(out.Entries) >= max {
				out.Truncated = true
				continue
			}
			title := e.Media.Title.English
			if title == "" {
				title = e.Media.Title.Romaji
			}
			out.Entries = append(out.Entries, AniListListEntry{ID: e.Media.ID, Title: title})
		}
	}
	return out, nil
}
