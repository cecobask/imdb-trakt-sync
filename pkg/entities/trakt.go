package entities

import "go.uber.org/zap/zapcore"

const (
	TraktItemTypeEpisode = "episode"
	TraktItemTypeMovie   = "movie"
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
	Imdb string `json:"imdb" zap:"imdb"`
	Slug string `json:"slug,omitempty"`
}

func (ti TraktIds) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("imdb", ti.Imdb)
	return nil
}

type TraktItemSpec struct {
	Ids     TraktIds `json:"ids" zap:"ids"`
	RatedAt string   `json:"rated_at,omitempty"`
	Rating  int      `json:"rating,omitempty"`
}

func (spec TraktItemSpec) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	_ = encoder.AddObject("ids", spec.Ids)
	return nil
}

type TraktItemSpecs []TraktItemSpec

func (specs TraktItemSpecs) MarshalLogArray(encoder zapcore.ArrayEncoder) error {
	for i := range specs {
		_ = encoder.AppendObject(specs[i])
	}
	return nil
}

type TraktItem struct {
	Type      string        `json:"type"`
	RatedAt   string        `json:"rated_at,omitempty"`
	Rating    int           `json:"rating,omitempty"`
	WatchedAt string        `json:"watched_at,omitempty"`
	Movie     TraktItemSpec `json:"movie,omitempty"`
	Show      TraktItemSpec `json:"show,omitempty"`
	Episode   TraktItemSpec `json:"episode,omitempty"`
}

type TraktListBody struct {
	Movies   TraktItemSpecs `json:"movies,omitempty" zap:"movies"`
	Shows    TraktItemSpecs `json:"shows,omitempty" zap:"shows"`
	Episodes TraktItemSpecs `json:"episodes,omitempty" zap:"episodes"`
}

func (tlb TraktListBody) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	_ = encoder.AddArray("movies", tlb.Movies)
	_ = encoder.AddArray("shows", tlb.Movies)
	_ = encoder.AddArray("episodes", tlb.Movies)
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
	Movies   int `json:"movies,omitempty" zap:"movies"`
	Shows    int `json:"shows,omitempty" zap:"shows"`
	Episodes int `json:"episodes,omitempty" zap:"episodes"`
}

func (tci TraktCrudItem) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddInt("movies", tci.Movies)
	encoder.AddInt("shows", tci.Shows)
	encoder.AddInt("episodes", tci.Episodes)
	return nil
}

type TraktResponse struct {
	Added    TraktCrudItem `json:"added,omitempty" zap:"added"`
	Deleted  TraktCrudItem `json:"deleted,omitempty" zap:"deleted"`
	Existing TraktCrudItem `json:"existing,omitempty" zap:"existing"`
	NotFound TraktListBody `json:"not_found,omitempty" zap:"not_found"`
}

type TraktList struct {
	Name string `json:"name"`
	Ids  TraktIds
}

func (tr TraktResponse) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	_ = encoder.AddObject("added", tr.Added)
	_ = encoder.AddObject("deleted", tr.Deleted)
	_ = encoder.AddObject("existing", tr.Existing)
	_ = encoder.AddObject("not_found", tr.NotFound)
	return nil
}
