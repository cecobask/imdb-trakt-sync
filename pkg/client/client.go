package client

import "github.com/cecobask/imdb-trakt-sync/pkg/entities"

type ImdbClientInterface interface {
	ListItemsGet(listId string) (*string, []entities.ImdbItem, error)
	ListsScrape() ([]entities.DataPair, error)
	UserIdScrape() error
	WatchlistGet() (*string, []entities.ImdbItem, error)
	WatchlistIdScrape() error
	RatingsGet() ([]entities.ImdbItem, error)
	Hydrate() error
}

type TraktClientInterface interface {
	BrowseSignIn() (*string, error)
	SignIn(authenticityToken string) error
	BrowseActivate() (*string, error)
	Activate(userCode, authenticityToken string) (*string, error)
	ActivateAuthorize(authenticityToken string) error
	GetAccessToken(deviceCode string) (*entities.TraktAuthTokensResponse, error)
	GetAuthCodes() (*entities.TraktAuthCodesResponse, error)
	WatchlistItemsGet() ([]entities.TraktItem, error)
	WatchlistItemsAdd(items []entities.TraktItem) error
	WatchlistItemsRemove(items []entities.TraktItem) error
	ListItemsGet(listId string) ([]entities.TraktItem, error)
	ListItemsAdd(listId string, items []entities.TraktItem) error
	ListItemsRemove(listId string, items []entities.TraktItem) error
	ListsGet() ([]entities.TraktList, error)
	ListAdd(listId, listName string) error
	ListRemove(listId string) error
	RatingsGet() ([]entities.TraktItem, error)
	RatingsAdd(items []entities.TraktItem) error
	RatingsRemove(items []entities.TraktItem) error
	HistoryGet(itemType, itemId string) ([]entities.TraktItem, error)
	HistoryAdd(items []entities.TraktItem) error
	HistoryRemove(items []entities.TraktItem) error
}

const (
	clientNameImdb  = "imdb"
	clientNameTrakt = "trakt"
)
