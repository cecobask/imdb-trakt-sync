package trakt

import (
	"fmt"
	"time"
)

const (
	ItemTypeEpisode = "episode"
	ItemTypeMovie   = "movie"
	ItemTypeSeason  = "season"
	ItemTypeShow    = "show"
	ItemTypePerson  = "person"
)

type CrudItem struct {
	Movies   int `json:"movies,omitempty"`
	Shows    int `json:"shows,omitempty"`
	Episodes int `json:"episodes,omitempty"`
	People   int `json:"people,omitempty"`
}

type IDMeta struct {
	IMDb     string  `json:"imdb,omitempty"`
	Slug     string  `json:"slug,omitempty"`
	ListName *string `json:"-"`
}

type IDMetas []IDMeta

func (idms IDMetas) GetListNameFromSlug(slug string) string {
	for _, idm := range idms {
		if idm.Slug == slug {
			return *idm.ListName
		}
	}
	return ""
}

type Item struct {
	Type    string    `json:"type"`
	RatedAt string    `json:"rated_at,omitempty"`
	Rating  int       `json:"rating,omitempty"`
	Movie   ItemSpec  `json:"movie,omitempty"`
	Show    ItemSpec  `json:"show,omitempty"`
	Episode ItemSpec  `json:"episode,omitempty"`
	Person  ItemSpec  `json:"person,omitempty"`
	Created time.Time `json:"-"`
}

func (it *Item) GetItemID() (*string, error) {
	switch it.Type {
	case ItemTypeMovie:
		return &it.Movie.IDMeta.IMDb, nil
	case ItemTypeShow:
		return &it.Show.IDMeta.IMDb, nil
	case ItemTypeEpisode:
		return &it.Episode.IDMeta.IMDb, nil
	case ItemTypeSeason:
		return nil, nil
	case ItemTypePerson:
		return &it.Person.IDMeta.IMDb, nil
	default:
		return nil, fmt.Errorf("unknown trakt item type %s", it.Type)
	}
}

type Items []Item

func (its Items) toListBody() listBody {
	lb := listBody{}
	for _, item := range its {
		switch item.Type {
		case ItemTypeMovie:
			lb.Movies = append(lb.Movies, item.Movie)
		case ItemTypeShow:
			lb.Shows = append(lb.Shows, item.Show)
		case ItemTypeEpisode:
			lb.Episodes = append(lb.Episodes, item.Episode)
		case ItemTypePerson:
			lb.People = append(lb.People, item.Person)
		}
	}
	return lb
}

type ItemSpec struct {
	IDMeta    IDMeta  `json:"ids"`
	RatedAt   *string `json:"rated_at,omitempty"`
	Rating    *int    `json:"rating,omitempty"`
	WatchedAt *string `json:"watched_at,omitempty"`
}

type ItemSpecs []ItemSpec

type List struct {
	Name        *string `json:"name,omitempty"`
	IDMeta      IDMeta  `json:"ids"`
	ListItems   Items
	IsWatchlist bool
}

type Lists []List

type listAddBody struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Privacy        string `json:"privacy"`
	DisplayNumbers bool   `json:"display_numbers"`
	AllowComments  bool   `json:"allow_comments"`
	SortBy         string `json:"sort_by"`
	SortHow        string `json:"sort_how"`
}

type listBody struct {
	Movies   ItemSpecs `json:"movies,omitempty"`
	Shows    ItemSpecs `json:"shows,omitempty"`
	Episodes ItemSpecs `json:"episodes,omitempty"`
	People   ItemSpecs `json:"people,omitempty"`
}

type response struct {
	Added    *CrudItem `json:"added,omitempty"`
	Deleted  *CrudItem `json:"deleted,omitempty"`
	Existing *CrudItem `json:"existing,omitempty"`
	NotFound *listBody `json:"not_found,omitempty"`
}

type userInfo struct {
	Username string `json:"username"`
}
