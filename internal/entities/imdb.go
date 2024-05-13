package entities

import (
	"time"
)

const (
	imdbItemTypeMovie        = "movie"
	imdbItemTypeTvEpisode    = "tvEpisode"
	imdbItemTypeTvMiniSeries = "tvMiniSeries"
	imdbItemTypeTvSeries     = "tvSeries"
)

type IMDbItem struct {
	ID         string
	TitleType  string
	Rating     *int
	RatingDate *time.Time
}

func (i *IMDbItem) toTraktItem() TraktItem {
	ti := TraktItem{}
	tiSpec := TraktItemSpec{
		IDMeta: TraktIDMeta{
			IMDb: i.ID,
		},
	}
	if i.Rating != nil {
		ratedAt := i.RatingDate.UTC().String()
		tiSpec.RatedAt = &ratedAt
		tiSpec.WatchedAt = &ratedAt
		tiSpec.Rating = i.Rating
	}
	switch i.TitleType {
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
