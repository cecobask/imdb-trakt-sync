package entities

import "time"

const (
	imdbItemTypeMovie        = "movie"
	imdbItemTypeTvEpisode    = "tvEpisode"
	imdbItemTypeTvMiniSeries = "tvMiniSeries"
	imdbItemTypeTvSeries     = "tvSeries"
)

type ImdbItem struct {
	Id         string
	TitleType  string
	Rating     *int
	RatingDate *time.Time
}
