package client

import "github.com/cecobask/imdb-trakt-sync/pkg/entities"

type ImdbClientInterface interface {
	ListGet(listId string) (*entities.ImdbList, error)
	WatchlistGet() (*entities.ImdbList, error)
	ListsGetAll() ([]entities.ImdbList, error)
	RatingsGet() ([]entities.ImdbItem, error)
	UserIdScrape() error
	WatchlistIdScrape() error
}

type TraktClientInterface interface {
	BrowseSignIn() (*string, error)
	SignIn(authenticityToken string) error
	BrowseActivate() (*string, error)
	Activate(userCode, authenticityToken string) (*string, error)
	ActivateAuthorize(authenticityToken string) error
	GetAccessToken(deviceCode string) (*entities.TraktAuthTokensResponse, error)
	GetAuthCodes() (*entities.TraktAuthCodesResponse, error)
	WatchlistGet() (*entities.TraktList, error)
	WatchlistItemsAdd(items []entities.TraktItem) error
	WatchlistItemsRemove(items []entities.TraktItem) error
	ListGet(listId string) (*entities.TraktList, error)
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
