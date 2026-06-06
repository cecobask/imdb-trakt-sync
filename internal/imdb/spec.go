package imdb

import (
	"time"

	"github.com/cecobask/imdb-trakt-sync/internal/trakt"
)

const (
	itemTypeMovie        = "Movie"
	itemTypeTvEpisode    = "TV Episode"
	itemTypeTvMiniSeries = "TV Mini Series"
	itemTypeTvSeries     = "TV Series"
	itemTypePerson       = "Person"
)

type Item struct {
	ID      string
	Kind    string
	Created time.Time
	Rating  *float64
}

func (it *Item) ToTraktItem() trakt.Item {
	ti := trakt.Item{
		Created: it.Created,
	}
	tiSpec := trakt.ItemSpec{
		IDMeta: trakt.IDMeta{
			IMDb: it.ID,
		},
	}
	if it.Rating != nil {
		ratedAt := it.Created.UTC().String()
		tiSpec.RatedAt = &ratedAt
		tiSpec.WatchedAt = &ratedAt
		tiSpec.Rating = it.Rating
	}
	switch it.Kind {
	case itemTypeMovie:
		ti.Type = trakt.ItemTypeMovie
		ti.Movie = tiSpec
	case itemTypeTvSeries:
		ti.Type = trakt.ItemTypeShow
		ti.Show = tiSpec
	case itemTypeTvMiniSeries:
		ti.Type = trakt.ItemTypeShow
		ti.Show = tiSpec
	case itemTypeTvEpisode:
		ti.Type = trakt.ItemTypeEpisode
		ti.Episode = tiSpec
	case itemTypePerson:
		ti.Type = trakt.ItemTypePerson
		ti.Person = tiSpec
	default:
		ti.Type = trakt.ItemTypeMovie
		ti.Movie = tiSpec
	}
	return ti
}

type Items []Item

type List struct {
	ListID      string
	ListName    string
	ListItems   []Item
	IsWatchlist bool
}

type Lists []List

type NextData struct {
	Props Props `json:"props"`
}

type Props struct {
	PageProps PageProps `json:"pageProps"`
}

type PageProps struct {
	AboveTheFoldData AboveTheFoldData `json:"aboveTheFoldData"`
}

type AboveTheFoldData struct {
	AuthorID        string `json:"authorId"`
	AuthorProfileID string `json:"authorProfileId"`
	ListID          string `json:"listId"`
}
