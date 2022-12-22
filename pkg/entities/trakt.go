package entities

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
	Imdb string `json:"imdb"`
	Slug string `json:"slug,omitempty"`
}

type TraktItemSpec struct {
	Ids     TraktIds `json:"ids"`
	RatedAt string   `json:"rated_at,omitempty"`
	Rating  int      `json:"rating,omitempty"`
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
	Movies   []TraktItemSpec `json:"movies,omitempty"`
	Shows    []TraktItemSpec `json:"shows,omitempty"`
	Episodes []TraktItemSpec `json:"episodes,omitempty"`
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
	Item     string        `json:"item,omitempty"`
	Added    TraktCrudItem `json:"added,omitempty"`
	Deleted  TraktCrudItem `json:"deleted,omitempty"`
	Existing TraktCrudItem `json:"existing,omitempty"`
	NotFound TraktListBody `json:"not_found,omitempty"`
}

type TraktList struct {
	Name string `json:"name"`
	Ids  TraktIds
}
