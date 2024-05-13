package entities

import (
	"fmt"
)

const (
	TraktItemTypeEpisode = "episode"
	TraktItemTypeMovie   = "movie"
	TraktItemTypeSeason  = "season"
	TraktItemTypeShow    = "show"
)

type TraktAuthCodesBody struct {
	ClientID string `json:"client_id"`
}

type TraktAuthCodesResponse struct {
	DeviceCode string `json:"device_code"`
	UserCode   string `json:"user_code"`
}

type TraktAuthTokensBody struct {
	Code         string `json:"code"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type TraktAuthTokensResponse struct {
	AccessToken string `json:"access_token"`
}

type TraktIDMeta struct {
	IMDb     string  `json:"imdb,omitempty"`
	Slug     string  `json:"slug,omitempty"`
	ListName *string `json:"-"`
}

type TraktIDMetas []TraktIDMeta

func (tidm TraktIDMetas) GetListNameFromSlug(slug string) string {
	for _, idm := range tidm {
		if idm.Slug == slug {
			return *idm.ListName
		}
	}
	return ""
}

type TraktItemSpec struct {
	IDMeta    TraktIDMeta `json:"ids"`
	RatedAt   *string     `json:"rated_at,omitempty"`
	Rating    *int        `json:"rating,omitempty"`
	WatchedAt *string     `json:"watched_at,omitempty"`
}

type TraktItemSpecs []TraktItemSpec

type TraktItem struct {
	Type    string        `json:"type"`
	RatedAt string        `json:"rated_at,omitempty"`
	Rating  int           `json:"rating,omitempty"`
	Movie   TraktItemSpec `json:"movie,omitempty"`
	Show    TraktItemSpec `json:"show,omitempty"`
	Episode TraktItemSpec `json:"episode,omitempty"`
}

type TraktItems []TraktItem

func (item *TraktItem) GetItemID() (*string, error) {
	switch item.Type {
	case TraktItemTypeMovie:
		return &item.Movie.IDMeta.IMDb, nil
	case TraktItemTypeShow:
		return &item.Show.IDMeta.IMDb, nil
	case TraktItemTypeEpisode:
		return &item.Episode.IDMeta.IMDb, nil
	case TraktItemTypeSeason:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown trakt item type %s", item.Type)
	}
}

type TraktListBody struct {
	Movies   TraktItemSpecs `json:"movies,omitempty"`
	Shows    TraktItemSpecs `json:"shows,omitempty"`
	Episodes TraktItemSpecs `json:"episodes,omitempty"`
}

type TraktListAddBody struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Privacy        string `json:"privacy"`
	DisplayNumbers bool   `json:"display_numbers"`
	AllowComments  bool   `json:"allow_comments"`
	SortBy         string `json:"sort_by"`
	SortHow        string `json:"sort_how"`
}

type TraktCrudItem struct {
	Movies   int `json:"movies,omitempty"`
	Shows    int `json:"shows,omitempty"`
	Episodes int `json:"episodes,omitempty"`
}

type TraktResponse struct {
	Added    *TraktCrudItem `json:"added,omitempty"`
	Deleted  *TraktCrudItem `json:"deleted,omitempty"`
	Existing *TraktCrudItem `json:"existing,omitempty"`
	NotFound *TraktListBody `json:"not_found,omitempty"`
}

type TraktList struct {
	Name        *string     `json:"name,omitempty"`
	IDMeta      TraktIDMeta `json:"ids"`
	ListItems   TraktItems
	IsWatchlist bool
}
