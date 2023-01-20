package client

import (
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"go.uber.org/zap"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	imdbCookieNameAtMain   = "at-main"
	imdbCookieNameUbidMain = "ubid-main"

	imdbHeaderKeyContentDisposition = "Content-Disposition"

	imdbPathBase          = "https://www.imdb.com"
	imdbPathListExport    = "/list/%s/export"
	imdbPathLists         = "/user/%s/lists"
	imdbPathProfile       = "/profile"
	imdbPathRatingsExport = "/user/%s/ratings/export"
	imdbPathWatchlist     = "/watchlist"
)

type ImdbClient struct {
	client *http.Client
	config ImdbConfig
	logger *zap.Logger
}

type ImdbConfig struct {
	CookieAtMain   string
	CookieUbidMain string
	UserId         string
	WatchlistId    string
}

func NewImdbClient(config ImdbConfig, logger *zap.Logger) (ImdbClientInterface, error) {
	jar, err := setupCookieJar(config)
	if err != nil {
		return nil, err
	}
	client := &ImdbClient{
		client: &http.Client{
			Jar: jar,
		},
		config: config,
		logger: logger,
	}
	if err = client.hydrate(); err != nil {
		return nil, fmt.Errorf("failure hydrating imdb client: %w", err)
	}
	return client, nil
}

func setupCookieJar(config ImdbConfig) (http.CookieJar, error) {
	imdbUrl, err := url.Parse(imdbPathBase)
	if err != nil {
		return nil, fmt.Errorf("failure parsing %s as url: %w", imdbPathBase, err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating cookie jar: %w", err)
	}
	jar.SetCookies(imdbUrl, []*http.Cookie{
		{
			Name:  imdbCookieNameAtMain,
			Value: config.CookieAtMain,
		},
		{
			Name:  imdbCookieNameUbidMain,
			Value: config.CookieUbidMain,
		},
	})
	return jar, nil
}

func (c *ImdbClient) hydrate() error {
	if err := c.UserIdScrape(); err != nil {
		return fmt.Errorf("failure scraping imdb user id: %w", err)
	}
	if err := c.WatchlistIdScrape(); err != nil {
		return fmt.Errorf("failure scraping imdb watchlist id: %w", err)
	}
	return nil
}

func (c *ImdbClient) doRequest(requestFields requestFields) (*http.Response, error) {
	request, err := http.NewRequest(requestFields.Method, requestFields.BasePath+requestFields.Endpoint, requestFields.Body)
	if err != nil {
		return nil, fmt.Errorf("failure creating http request %s %s: %w", requestFields.Method, requestFields.BasePath+requestFields.Endpoint, err)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failure sending http request %s %s: %w", request.Method, request.URL, err)
	}
	switch response.StatusCode {
	case http.StatusOK:
		return response, nil
	case http.StatusNotFound:
		return response, nil
	case http.StatusForbidden:
		response.Body.Close()
		return nil, &ApiError{
			httpMethod: request.Method,
			url:        request.URL.String(),
			StatusCode: response.StatusCode,
			details:    "imdb authorization failure - update the imdb cookie values",
		}
	default:
		response.Body.Close()
		return nil, &ApiError{
			httpMethod: request.Method,
			url:        request.URL.String(),
			StatusCode: response.StatusCode,
			details:    fmt.Sprintf("unexpected status code %d", response.StatusCode),
		}
	}
}

func (c *ImdbClient) ListGet(listId string) (*entities.ImdbList, error) {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: imdbPathBase,
		Endpoint: fmt.Sprintf(imdbPathListExport, listId),
		Body:     http.NoBody,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, &ApiError{
			httpMethod: response.Request.Method,
			url:        response.Request.URL.String(),
			StatusCode: response.StatusCode,
			details:    fmt.Sprintf("list with id %s could not be found", listId),
		}
	}
	return readImdbListResponse(response, listId)
}

func (c *ImdbClient) WatchlistGet() (*entities.ImdbList, error) {
	list, err := c.ListGet(c.config.WatchlistId)
	if err != nil {
		return nil, err
	}
	list.IsWatchlist = true
	return list, nil
}

func (c *ImdbClient) ListsGetAll() ([]entities.ImdbList, error) {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: imdbPathBase,
		Endpoint: fmt.Sprintf(imdbPathLists, c.config.UserId),
		Body:     http.NoBody,
	})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failure creating goquery document from imdb response: %w", err)
	}
	var ids []string
	doc.Find(".user-list").Each(func(i int, selection *goquery.Selection) {
		id, ok := selection.Attr("id")
		if !ok {
			c.logger.Info("found no imdb lists")
			return
		}
		ids = append(ids, id)
	})
	return c.ListsGet(ids)
}

func (c *ImdbClient) ListsGet(listIds []string) ([]entities.ImdbList, error) {
	var (
		outChan  = make(chan entities.ImdbList, len(listIds))
		errChan  = make(chan error, 1)
		doneChan = make(chan struct{})
		lists    = make([]entities.ImdbList, 0, len(listIds))
	)
	go func() {
		waitGroup := new(sync.WaitGroup)
		for _, listId := range listIds {
			waitGroup.Add(1)
			go func(id string) {
				defer waitGroup.Done()
				imdbList, err := c.ListGet(id)
				if err != nil {
					var apiError *ApiError
					if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
						c.logger.Debug("silencing not found error while fetching imdb lists", zap.Error(apiError))
						return
					}
					errChan <- fmt.Errorf("unexpected error while fetching imdb lists: %w", err)
					return
				}
				imdbList.TraktListSlug = buildTraktListName(imdbList.ListName)
				outChan <- *imdbList
			}(listId)
		}
		waitGroup.Wait()
		close(doneChan)
	}()
	for {
		select {
		case list := <-outChan:
			lists = append(lists, list)
		case err := <-errChan:
			return nil, err
		case <-doneChan:
			return lists, nil
		}
	}
}

func (c *ImdbClient) UserIdScrape() error {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: imdbPathBase,
		Endpoint: imdbPathProfile,
		Body:     http.NoBody,
	})
	if err != nil {
		return err
	}
	userId, err := scrapeSelectionAttribute(response.Body, clientNameImdb, ".user-profile.userId", "data-userid")
	if err != nil {
		return fmt.Errorf("imdb user id not found: %w", err)
	}
	c.config.UserId = *userId
	return nil
}

func (c *ImdbClient) WatchlistIdScrape() error {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: imdbPathBase,
		Endpoint: imdbPathWatchlist,
		Body:     http.NoBody,
	})
	if err != nil {
		return err
	}
	watchlistId, err := scrapeSelectionAttribute(response.Body, clientNameImdb, "meta[property='pageId']", "content")
	if err != nil {
		return fmt.Errorf("imdb watchlist id not found: %w", err)
	}
	c.config.WatchlistId = *watchlistId
	return nil
}

func (c *ImdbClient) RatingsGet() ([]entities.ImdbItem, error) {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: imdbPathBase,
		Endpoint: fmt.Sprintf(imdbPathRatingsExport, c.config.UserId),
		Body:     http.NoBody,
	})
	if err != nil {
		return nil, err
	}
	return readImdbRatingsResponse(response)
}

func readImdbListResponse(response *http.Response, listId string) (*entities.ImdbList, error) {
	defer response.Body.Close()
	csvReader := csv.NewReader(response.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failure reading from imdb response: %w", err)
	}
	var listItems []entities.ImdbItem
	for i, record := range csvData {
		if i > 0 { // omit header line
			listItems = append(listItems, entities.ImdbItem{
				Id:        record[1],
				TitleType: record[7],
			})
		}
	}
	contentDispositionHeader := response.Header.Get(imdbHeaderKeyContentDisposition)
	if contentDispositionHeader == "" {
		return nil, fmt.Errorf("failure reading header %s from imdb response", imdbHeaderKeyContentDisposition)
	}
	_, params, err := mime.ParseMediaType(contentDispositionHeader)
	if err != nil || len(params) == 0 {
		return nil, fmt.Errorf("failure parsing media type from imdb header %s: %w", imdbHeaderKeyContentDisposition, err)
	}
	listName := strings.Split(params["filename"], ".")[0]
	return &entities.ImdbList{
		ListName:      listName,
		ListId:        listId,
		ListItems:     listItems,
		TraktListSlug: buildTraktListName(listName),
	}, nil
}

func readImdbRatingsResponse(response *http.Response) ([]entities.ImdbItem, error) {
	defer response.Body.Close()
	csvReader := csv.NewReader(response.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failure reading from imdb response: %w", err)
	}
	var ratings []entities.ImdbItem
	for i, record := range csvData {
		if i > 0 {
			rating, err := strconv.Atoi(record[1])
			if err != nil {
				return nil, fmt.Errorf("failure parsing imdb rating value to integer: %w", err)
			}
			ratingDate, err := time.Parse("2006-01-02", record[2])
			if err != nil {
				return nil, fmt.Errorf("failure parsing imdb rating date: %w", err)
			}
			ratings = append(ratings, entities.ImdbItem{
				Id:         record[0],
				TitleType:  record[5],
				Rating:     &rating,
				RatingDate: &ratingDate,
			})
		}
	}
	return ratings, nil
}

func buildTraktListName(imdbListName string) string {
	formatted := strings.ToLower(strings.Join(strings.Fields(imdbListName), "-"))
	re := regexp.MustCompile(`[^-a-z0-9]+`)
	return re.ReplaceAllString(formatted, "")
}
