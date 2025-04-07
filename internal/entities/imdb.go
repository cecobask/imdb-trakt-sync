package entities

import (
	"time"
)

const (
	imdbItemTypeMovie        = "Movie"
	imdbItemTypeTvEpisode    = "TV Episode"
	imdbItemTypeTvMiniSeries = "TV Mini Series"
	imdbItemTypeTvSeries     = "TV Series"
	imdbItemTypePerson       = "Person"
)

type IMDbItem struct {
	ID      string
	Kind    string
	Created time.Time
	Rating  *int
}

func (i *IMDbItem) toTraktItem() TraktItem {
	ti := TraktItem{
		created: i.Created,
	}
	tiSpec := TraktItemSpec{
		IDMeta: TraktIDMeta{
			IMDb: i.ID,
		},
	}
	if i.Rating != nil {
		ratedAt := i.Created.UTC().String()
		tiSpec.RatedAt = &ratedAt
		tiSpec.WatchedAt = &ratedAt
		tiSpec.Rating = i.Rating
	}
	switch i.Kind {
	case imdbItemTypeMovie:
		ti.Type = TraktItemTypeMovie
		ti.Movie = tiSpec
	case imdbItemTypeTvSeries:
		ti.Type = TraktItemTypeShow
		ti.Show = tiSpec
	case imdbItemTypeTvMiniSeries:
		ti.Type = TraktItemTypeShow
		ti.Show = tiSpec
	case imdbItemTypeTvEpisode:
		ti.Type = TraktItemTypeEpisode
		ti.Episode = tiSpec
	case imdbItemTypePerson:
		ti.Type = TraktItemTypePerson
		ti.Person = tiSpec
	default:
		ti.Type = TraktItemTypeMovie
		ti.Movie = tiSpec
	}
	return ti
}

type IMDbList struct {
	ListID      string
	ListName    string
	ListItems   []IMDbItem
	IsWatchlist bool
}
