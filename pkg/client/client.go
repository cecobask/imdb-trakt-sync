package client

import "github.com/cecobask/imdb-trakt-sync/pkg/entities"

type ImdbClientInterface interface {
	ListItemsGet(listId string) (*string, []entities.ImdbItem, error)
	ListsScrape() (dp []entities.DataPair)
	UserIdScrape() string
	WatchlistGet() (*string, []entities.ImdbItem, error)
	WatchlistIdScrape() string
	RatingsGet() []entities.ImdbItem
	Hydrate()
}

type TraktClientInterface interface {
	BrowseSignIn() string
	SignIn(authenticityToken string)
	BrowseActivate() string
	Activate(userCode, authenticityToken string) string
	ActivateAuthorize(authenticityToken string)
	GetAccessToken(deviceCode string) string
	GetAuthCodes() entities.TraktAuthCodesResponse
	WatchlistItemsGet() []entities.TraktItem
	WatchlistItemsAdd(items []entities.TraktItem)
	WatchlistItemsRemove(items []entities.TraktItem)
	ListItemsGet(listId string) ([]entities.TraktItem, error)
	ListItemsAdd(listId string, items []entities.TraktItem)
	ListItemsRemove(listId string, items []entities.TraktItem)
	ListsGet() []entities.TraktList
	ListAdd(listId, listName string)
	ListRemove(listId string)
	RatingsGet() []entities.TraktItem
	RatingsAdd(items []entities.TraktItem)
	RatingsRemove(items []entities.TraktItem)
	HistoryGet(item entities.TraktItem) []entities.TraktItem
	HistoryAdd(items []entities.TraktItem)
	HistoryRemove(items []entities.TraktItem)
}

const (
	resourceTypeList      = "list"
	resourceTypeRating    = "rating"
	resourceTypeWatchlist = "watchlist"
)
