package client

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	appconfig "github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/entities"
	"github.com/cecobask/imdb-trakt-sync/pkg/logger"
)

const (
	imdbCookieNameAtMain            = "at-main"
	imdbCookieNameUbidMain          = "ubid-main"
	imdbHeaderKeyContentDisposition = "Content-Disposition"
	imdbPathBase                    = "https://www.imdb.com"
	imdbPathListExport              = "/list/%s/export"
	imdbPathLists                   = "/user/%s/lists"
	imdbPathProfile                 = "/profile"
	imdbPathRatingsExport           = "/user/%s/ratings/export"
	imdbPathWatchlist               = "/watchlist"
)

type IMDbClient struct {
	client *http.Client
	config imdbConfig
	logger *slog.Logger
}

type imdbConfig struct {
	appconfig.IMDb
	basePath    string
	userID      string
	watchlistID string
}

func NewIMDbClient(conf appconfig.IMDb, logger *slog.Logger) (IMDbClientInterface, error) {
	config := imdbConfig{
		IMDb:     conf,
		basePath: imdbPathBase,
	}
	jar, err := setupCookieJar(config)
	if err != nil {
		return nil, err
	}
	client := &IMDbClient{
		client: &http.Client{
			Jar: jar,
		},
		config: config,
		logger: logger,
	}
	return client, nil
}

func setupCookieJar(config imdbConfig) (http.CookieJar, error) {
	imdbUrl, err := url.Parse(config.basePath)
	if err != nil {
		return nil, fmt.Errorf("failure parsing %s as url: %w", config.basePath, err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating cookie jar: %w", err)
	}
	jar.SetCookies(imdbUrl, []*http.Cookie{
		{
			Name:  imdbCookieNameAtMain,
			Value: *config.CookieAtMain,
		},
		{
			Name:  imdbCookieNameUbidMain,
			Value: *config.CookieUbidMain,
		},
	})
	return jar, nil
}

func (c *IMDbClient) Hydrate() error {
	if err := c.UserIDScrape(); err != nil {
		return fmt.Errorf("failure scraping imdb user id: %w", err)
	}
	if err := c.WatchlistIDScrape(); err != nil {
		return fmt.Errorf("failure scraping imdb watchlist id: %w", err)
	}
	return nil
}

func (c *IMDbClient) doRequest(requestFields requestFields) (*http.Response, error) {
	request, err := http.NewRequest(requestFields.Method, requestFields.BasePath+requestFields.Endpoint, requestFields.Body)
	if err != nil {
		return nil, fmt.Errorf("failure creating http request %s %s: %w", requestFields.Method, requestFields.BasePath+requestFields.Endpoint, err)
	}
	request.Header.Set("User-Agent", "PostmanRuntime/7.37.3") // workaround for https://github.com/cecobask/imdb-trakt-sync/issues/33
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failure sending http request %s %s: %w", request.Method, request.URL, err)
	}
	switch response.StatusCode {
	case http.StatusOK, http.StatusNotFound:
		return response, nil
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

func (c *IMDbClient) ListGet(listID string) (*entities.IMDbList, error) {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: c.config.basePath,
		Endpoint: fmt.Sprintf(imdbPathListExport, listID),
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
			details:    fmt.Sprintf("list with id %s could not be found", listID),
		}
	}
	return readIMDbListResponse(response, listID)
}

func (c *IMDbClient) WatchlistGet() (*entities.IMDbList, error) {
	list, err := c.ListGet(c.config.watchlistID)
	if err != nil {
		return nil, err
	}
	list.IsWatchlist = true
	return list, nil
}

func (c *IMDbClient) ListsGetAll() ([]entities.IMDbList, error) {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: c.config.basePath,
		Endpoint: fmt.Sprintf(imdbPathLists, c.config.userID),
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
	var (
		ids             = make([]string, 0)
		itemsSelector   = "li[data-testid='user-ll-item']"
		summarySelector = ".ipc-metadata-list-summary-item__t"
	)
	selection := doc.Find(itemsSelector).Each(func(i int, selection *goquery.Selection) {
		value, ok := selection.Find(summarySelector).Attr("href")
		if !ok {
			c.logger.Error(fmt.Sprintf("failure scraping selector %s", summarySelector))
			return
		}
		listID, err := extractListID(value)
		if err != nil {
			c.logger.Error("failure extracting imdb list id", err)
			return
		}
		ids = append(ids, listID)
	})
	if selection.Length() == 0 {
		return nil, fmt.Errorf("failure finding imdb lists in html response")
	}
	return c.ListsGet(ids)
}

func (c *IMDbClient) ListsGet(listIDs []string) ([]entities.IMDbList, error) {
	var (
		outChan  = make(chan entities.IMDbList, len(listIDs))
		errChan  = make(chan error, 1)
		doneChan = make(chan struct{})
		lists    = make([]entities.IMDbList, 0, len(listIDs))
	)
	go func() {
		waitGroup := new(sync.WaitGroup)
		for _, listID := range listIDs {
			waitGroup.Add(1)
			go func(id string) {
				defer waitGroup.Done()
				imdbList, err := c.ListGet(id)
				if err != nil {
					var apiError *ApiError
					if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
						c.logger.Debug("silencing not found error while fetching imdb lists", logger.Error(apiError))
						return
					}
					errChan <- fmt.Errorf("unexpected error while fetching imdb lists: %w", err)
					return
				}
				outChan <- *imdbList
			}(listID)
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

func (c *IMDbClient) UserIDScrape() error {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: c.config.basePath,
		Endpoint: imdbPathProfile,
		Body:     http.NoBody,
	})
	if err != nil {
		return err
	}
	userID, err := scrapeSelectionAttribute(response.Body, clientNameIMDb, ".user-profile.userId", "data-userid")
	if err != nil {
		return fmt.Errorf("imdb user id not found: %w", err)
	}
	c.config.userID = *userID
	return nil
}

func (c *IMDbClient) WatchlistIDScrape() error {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: c.config.basePath,
		Endpoint: imdbPathWatchlist,
		Body:     http.NoBody,
	})
	if err != nil {
		return err
	}
	href, err := scrapeSelectionAttribute(response.Body, clientNameIMDb, "a[data-testid='hero-list-subnav-edit-button']", "href")
	if err != nil {
		return fmt.Errorf("imdb watchlist href not found: %w", err)
	}
	watchlistID, err := extractListID(*href)
	if err != nil {
		return fmt.Errorf("failure extracting imdb watchlist id: %w", err)
	}
	c.config.watchlistID = watchlistID
	return nil
}

func (c *IMDbClient) RatingsGet() ([]entities.IMDbItem, error) {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: c.config.basePath,
		Endpoint: fmt.Sprintf(imdbPathRatingsExport, c.config.userID),
		Body:     http.NoBody,
	})
	if err != nil {
		return nil, err
	}
	return readIMDbRatingsResponse(response)
}

func readIMDbListResponse(response *http.Response, listID string) (*entities.IMDbList, error) {
	defer response.Body.Close()
	csvReader := csv.NewReader(response.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failure reading from imdb response: %w", err)
	}
	var listItems []entities.IMDbItem
	for i, record := range csvData {
		if i > 0 { // omit header line
			listItems = append(listItems, entities.IMDbItem{
				ID:        record[1],
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
	return &entities.IMDbList{
		ListName:  listName,
		ListID:    listID,
		ListItems: listItems,
	}, nil
}

func readIMDbRatingsResponse(response *http.Response) ([]entities.IMDbItem, error) {
	defer response.Body.Close()
	csvReader := csv.NewReader(response.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failure reading from imdb response: %w", err)
	}
	var ratings []entities.IMDbItem
	for i, record := range csvData {
		if i > 0 {
			rating, err := strconv.Atoi(record[1])
			if err != nil {
				return nil, fmt.Errorf("failure parsing imdb rating value to integer: %w", err)
			}
			ratingDate, err := time.Parse(time.DateOnly, record[2])
			if err != nil {
				return nil, fmt.Errorf("failure parsing imdb rating date: %w", err)
			}
			ratings = append(ratings, entities.IMDbItem{
				ID:         record[0],
				TitleType:  record[5],
				Rating:     &rating,
				RatingDate: &ratingDate,
			})
		}
	}
	return ratings, nil
}

func extractListID(href string) (string, error) {
	pieces := strings.Split(href, "/")
	if len(pieces) < 3 {
		return "", fmt.Errorf("imdb watchlist href has unexpected format: %s", href)
	}
	return pieces[2], nil
}
