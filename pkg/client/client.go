package client

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"io"
)

type ImdbClientInterface interface {
	ListGet(listId string) (*entities.ImdbList, error)
	ListsGet(listIds []string) ([]entities.ImdbList, error)
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
	WatchlistItemsAdd(items entities.TraktItems) error
	WatchlistItemsRemove(items entities.TraktItems) error
	ListGet(listId string) (*entities.TraktList, error)
	ListsGet(idMeta []entities.TraktIdMeta) ([]entities.TraktList, error)
	ListItemsAdd(listId string, items entities.TraktItems) error
	ListItemsRemove(listId string, items entities.TraktItems) error
	ListAdd(listId, listName string) error
	ListRemove(listId string) error
	RatingsGet() (entities.TraktItems, error)
	RatingsAdd(items entities.TraktItems) error
	RatingsRemove(items entities.TraktItems) error
	HistoryGet(itemType, itemId string) (entities.TraktItems, error)
	HistoryAdd(items entities.TraktItems) error
	HistoryRemove(items entities.TraktItems) error
}

const (
	clientNameImdb  = "imdb"
	clientNameTrakt = "trakt"
)

type requestFields struct {
	Method   string
	BasePath string
	Endpoint string
	Body     io.Reader
	Headers  map[string]string
}

type reusableReader struct {
	io.Reader
	readBuf *bytes.Buffer
	backBuf *bytes.Buffer
}

func ReusableReader(r io.Reader) io.Reader {
	readBuf := bytes.Buffer{}
	readBuf.ReadFrom(r)
	backBuf := bytes.Buffer{}
	return reusableReader{
		Reader:  io.TeeReader(&readBuf, &backBuf),
		readBuf: &readBuf,
		backBuf: &backBuf,
	}
}

func (r reusableReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		io.Copy(r.readBuf, r.backBuf)
	}
	return n, err
}

func scrapeSelectionAttribute(body io.ReadCloser, clientName, selector, attribute string) (*string, error) {
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("failure creating goquery document from %s response: %w", clientName, err)
	}
	value, ok := doc.Find(selector).Attr(attribute)
	if !ok {
		return nil, fmt.Errorf("failure scraping %s response for selector %s and attribute %s", clientName, selector, attribute)
	}
	return &value, nil
}
