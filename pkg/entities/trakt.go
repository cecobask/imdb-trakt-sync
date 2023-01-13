package entities

import (
	"fmt"
	"go.uber.org/zap/zapcore"
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

type TraktIds struct {
	Imdb string `json:"imdb,omitempty" zap:"imdb,omitempty"`
	Slug string `json:"slug,omitempty"`
}

func (ti TraktIds) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("imdb", ti.Imdb)
	return nil
}

type TraktItemSpec struct {
	Ids       TraktIds `json:"ids" zap:"ids"`
	RatedAt   *string  `json:"rated_at,omitempty"`
	Rating    *int     `json:"rating,omitempty"`
	WatchedAt *string  `json:"watched_at,omitempty"`
}

func (spec *TraktItemSpec) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	_ = encoder.AddObject("ids", spec.Ids)
	return nil
}

type TraktItemSpecs []TraktItemSpec

func (specs TraktItemSpecs) MarshalLogArray(encoder zapcore.ArrayEncoder) error {
	for i := range specs {
		_ = encoder.AppendObject(&specs[i])
	}
	return nil
}

type TraktItem struct {
	Type    string        `json:"type"`
	RatedAt string        `json:"rated_at,omitempty"`
	Rating  int           `json:"rating,omitempty"`
	Movie   TraktItemSpec `json:"movie,omitempty"`
	Show    TraktItemSpec `json:"show,omitempty"`
	Episode TraktItemSpec `json:"episode,omitempty"`
}

type TraktItems []TraktItem

func (items TraktItems) MarshalLogArray(encoder zapcore.ArrayEncoder) error {
	for i := range items {
		id, err := items[i].GetItemId()
		if err != nil {
			return err
		}
		encoder.AppendString(*id)
	}
	return nil
}

func (item *TraktItem) GetItemId() (*string, error) {
	switch item.Type {
	case TraktItemTypeMovie:
		return &item.Movie.Ids.Imdb, nil
	case TraktItemTypeShow:
		return &item.Show.Ids.Imdb, nil
	case TraktItemTypeEpisode:
		return &item.Episode.Ids.Imdb, nil
	case TraktItemTypeSeason:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown trakt item type %s", item.Type)
	}
}

type TraktListBody struct {
	Movies   TraktItemSpecs `json:"movies,omitempty" zap:"movies,omitempty"`
	Shows    TraktItemSpecs `json:"shows,omitempty" zap:"shows,omitempty"`
	Episodes TraktItemSpecs `json:"episodes,omitempty" zap:"episodes,omitempty"`
}

func (tlb *TraktListBody) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	if len(tlb.Movies) != 0 {
		_ = encoder.AddArray("movies", tlb.Movies)
	}
	if len(tlb.Shows) != 0 {
		_ = encoder.AddArray("shows", tlb.Shows)
	}
	if len(tlb.Episodes) != 0 {
		_ = encoder.AddArray("episodes", tlb.Episodes)
	}
	return nil
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
	Movies   int `json:"movies,omitempty" zap:"movies,omitempty"`
	Shows    int `json:"shows,omitempty" zap:"shows,omitempty"`
	Episodes int `json:"episodes,omitempty" zap:"episodes,omitempty"`
}

func (tci *TraktCrudItem) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	if tci.Movies != 0 {
		encoder.AddInt("movies", tci.Movies)
	}
	if tci.Shows != 0 {
		encoder.AddInt("shows", tci.Shows)
	}
	if tci.Episodes != 0 {
		encoder.AddInt("episodes", tci.Episodes)
	}
	return nil
}

type TraktResponse struct {
	Added    *TraktCrudItem `json:"added,omitempty" zap:"added,omitempty"`
	Deleted  *TraktCrudItem `json:"deleted,omitempty" zap:"deleted,omitempty"`
	Existing *TraktCrudItem `json:"existing,omitempty" zap:"existing,omitempty"`
	NotFound *TraktListBody `json:"not_found,omitempty" zap:"not_found,omitempty"`
}

func (tr *TraktResponse) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	if tr.Added != nil {
		if tr.Added.Movies != 0 || tr.Added.Shows != 0 || tr.Added.Episodes != 0 {
			_ = encoder.AddObject("added", tr.Added)
		}
	}
	if tr.Deleted != nil {
		if tr.Deleted.Movies != 0 || tr.Deleted.Shows != 0 || tr.Deleted.Episodes != 0 {
			_ = encoder.AddObject("deleted", tr.Deleted)
		}
	}
	if tr.Existing != nil {
		if tr.Existing.Movies != 0 || tr.Existing.Shows != 0 || tr.Existing.Episodes != 0 {
			_ = encoder.AddObject("existing", tr.Existing)
		}
	}
	if tr.NotFound != nil {
		if len(tr.NotFound.Movies) != 0 || len(tr.NotFound.Shows) != 0 || len(tr.NotFound.Episodes) != 0 {
			_ = encoder.AddObject("not_found", tr.NotFound)
		}
	}
	return nil
}

type TraktList struct {
	Name        *string `json:"name,omitempty"`
	Ids         TraktIds
	ListItems   TraktItems
	IsWatchlist bool
}
