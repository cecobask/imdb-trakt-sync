package client

import (
	"bytes"
	"fmt"
	"io"

	"github.com/PuerkitoBio/goquery"

	"github.com/cecobask/imdb-trakt-sync/internal/entities"
)

type IMDbClientInterface interface {
	ListsExport(ids ...string) error
	ListsGet(ids ...string) ([]entities.IMDbList, error)
	WatchlistExport() error
	WatchlistGet() (*entities.IMDbList, error)
	RatingsExport() error
	RatingsGet() ([]entities.IMDbItem, error)
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
	ListGet(listID string) (*entities.TraktList, error)
	ListsGet(idMeta entities.TraktIDMetas) ([]entities.TraktList, []error)
	ListItemsAdd(listID string, items entities.TraktItems) error
	ListItemsRemove(listID string, items entities.TraktItems) error
	ListAdd(listID, listName string) error
	RatingsGet() (entities.TraktItems, error)
	RatingsAdd(items entities.TraktItems) error
	RatingsRemove(items entities.TraktItems) error
	HistoryGet(itemType, itemID string) (entities.TraktItems, error)
	HistoryAdd(items entities.TraktItems) error
	HistoryRemove(items entities.TraktItems) error
	UserInfoGet() (*entities.TraktUserInfo, error)
}

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
	_, _ = readBuf.ReadFrom(r)
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
		_, _ = io.Copy(r.readBuf, r.backBuf)
	}
	return n, err
}

func selectorExists(body io.ReadCloser, selector string) error {
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return fmt.Errorf("failure creating goquery document from response: %w", err)
	}
	if doc.Find(selector).Length() == 0 {
		return fmt.Errorf("failure finding selector %s", selector)
	}
	return nil
}

func selectorAttributeScrape(body io.ReadCloser, selector, attribute string) (*string, error) {
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("failure creating goquery document from response: %w", err)
	}
	value, ok := doc.Find(selector).Attr(attribute)
	if !ok {
		return nil, fmt.Errorf("failure scraping response for selector %s and attribute %s", selector, attribute)
	}
	return &value, nil
}

type ApiError struct {
	httpMethod string
	url        string
	StatusCode int
	details    string
}

func (e *ApiError) Error() string {
	return fmt.Sprintf("http request %s %s returned status code %d: %s", e.httpMethod, e.url, e.StatusCode, e.details)
}

type TraktListNotFoundError struct {
	Slug string
}

func (e *TraktListNotFoundError) Error() string {
	return fmt.Sprintf("list with id %s could not be found", e.Slug)
}

func pointer[T any](v T) *T {
	return &v
}
