package client

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"go.uber.org/zap"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
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

	resourceTypeList   = "list"
	resourceTypeRating = "rating"
)

type ImdbClient struct {
	endpoint string
	client   *http.Client
	config   ImdbConfig
	logger   *zap.Logger
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
		endpoint: imdbPathBase,
		client: &http.Client{
			Jar: jar,
		},
		config: config,
		logger: logger,
	}
	if err = client.Hydrate(); err != nil {
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

func (c *ImdbClient) Hydrate() error {
	if c.config.UserId == "" || c.config.UserId == "scrape" {
		if err := c.UserIdScrape(); err != nil {
			return fmt.Errorf("failure scraping imdb user id: %w", err)
		}
	}
	if err := c.WatchlistIdScrape(); err != nil {
		return fmt.Errorf("failure scraping imdb watchlist id: %w", err)
	}
	return nil
}

func (c *ImdbClient) doRequest(params requestParams) (*http.Response, error) {
	req, err := http.NewRequest(params.Method, c.endpoint+params.Path, nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating http request %s %s: %w", params.Method, c.endpoint+params.Path, err)
	}
	if params.Body != nil {
		body, err := json.Marshal(params.Body)
		if err != nil {
			return nil, fmt.Errorf("failure marshalling http request body for %s %s: %w", params.Method, req.URL, err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failure sending http request %s %s: %w", params.Method, res.Request.URL, err)
	}
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		return nil, &ApiError{
			clientName: clientNameImdb,
			httpMethod: req.Method,
			url:        req.URL.String(),
			StatusCode: res.StatusCode,
			details:    "imdb authorization failure - update the imdb cookie values",
		}
	case http.StatusNotFound:
		break // handled individually in various functions
	default:
		return nil, &ApiError{
			clientName: clientNameImdb,
			httpMethod: req.Method,
			url:        req.URL.String(),
			StatusCode: res.StatusCode,
			details:    fmt.Sprintf("unexpected status code %d", res.StatusCode),
		}
	}
	return res, nil
}

func (c *ImdbClient) ListItemsGet(listId string) (*string, []entities.ImdbItem, error) {
	res, err := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(imdbPathListExport, listId),
	})
	if err != nil {
		return nil, nil, err
	}
	defer DrainBody(res.Body)
	if res.StatusCode == http.StatusNotFound {
		return nil, nil, &ApiError{
			clientName: clientNameImdb,
			httpMethod: res.Request.Method,
			url:        res.Request.URL.String(),
			StatusCode: res.StatusCode,
			details:    fmt.Sprintf("list with id %s could not be found", listId),
		}
	}
	return readResponse(res, resourceTypeList)
}

func (c *ImdbClient) WatchlistGet() (*string, []entities.ImdbItem, error) {
	path := fmt.Sprintf(imdbPathListExport, c.config.WatchlistId)
	res, err := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   path,
	})
	if err != nil {
		return nil, nil, err
	}
	defer DrainBody(res.Body)
	if res.StatusCode == http.StatusNotFound {
		return nil, nil, &ApiError{
			clientName: clientNameImdb,
			httpMethod: res.Request.Method,
			url:        res.Request.URL.String(),
			StatusCode: res.StatusCode,
			details:    fmt.Sprintf("list with id %s could not be found", c.config.WatchlistId),
		}
	}
	_, list, err := readResponse(res, resourceTypeList)
	if err != nil {
		return nil, nil, err
	}
	return &c.config.WatchlistId, list, nil
}

func (c *ImdbClient) ListsScrape() (dps []entities.DataPair, err error) {
	res, err := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(imdbPathLists, c.config.UserId),
	})
	if err != nil {
		return nil, err
	}
	defer DrainBody(res.Body)
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failure creating goquery document from imdb response: %w", err)
	}
	doc.Find(".user-list").Each(func(i int, selection *goquery.Selection) {
		imdbListId, ok := selection.Attr("id")
		if !ok {
			c.logger.Info("found no imdb lists")
			return
		}
		imdbListName, imdbList, err := c.ListItemsGet(imdbListId)
		if err != nil {
			c.logger.Error("unexpected error while scraping imdb lists", zap.Error(err))
			return
		}
		dps = append(dps, entities.DataPair{
			ImdbList:     imdbList,
			ImdbListId:   imdbListId,
			ImdbListName: *imdbListName,
			TraktListId:  FormatTraktListName(*imdbListName),
		})
	})
	return dps, nil
}

func (c *ImdbClient) UserIdScrape() error {
	res, err := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   imdbPathProfile,
	})
	if err != nil {
		return err
	}
	defer DrainBody(res.Body)
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return fmt.Errorf("failure creating goquery document from imdb response: %w", err)
	}
	userId, ok := doc.Find(".user-profile.userId").Attr("data-userid")
	if !ok {
		return fmt.Errorf("failure scraping imdb profile: user id not found")
	}
	c.config.UserId = userId
	return nil
}

func (c *ImdbClient) WatchlistIdScrape() error {
	res, err := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   imdbPathWatchlist,
	})
	if err != nil {
		return err
	}
	defer DrainBody(res.Body)
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return fmt.Errorf("failure creating goquery document from imdb response: %w", err)
	}
	watchlistId, ok := doc.Find("meta[property='pageId']").Attr("content")
	if !ok {
		return fmt.Errorf("failure scraping imdb profile: watchlist id not found")
	}
	c.config.WatchlistId = watchlistId
	return nil
}

func (c *ImdbClient) RatingsGet() ([]entities.ImdbItem, error) {
	res, err := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(imdbPathRatingsExport, c.config.UserId),
	})
	if err != nil {
		return nil, err
	}
	defer DrainBody(res.Body)
	_, ratings, err := readResponse(res, resourceTypeRating)
	if err != nil {
		return nil, err
	}
	return ratings, nil
}

func readResponse(res *http.Response, resType string) (imdbListName *string, imdbList []entities.ImdbItem, err error) {
	csvReader := csv.NewReader(res.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading from imdb response: %w", err)
	}
	switch resType {
	case resourceTypeList:
		for i, record := range csvData {
			if i > 0 { // omit header line
				imdbList = append(imdbList, entities.ImdbItem{
					Id:        record[1],
					TitleType: record[7],
				})
			}
		}
		contentDispositionHeader := res.Header.Get(imdbHeaderKeyContentDisposition)
		if contentDispositionHeader == "" {
			return nil, nil, fmt.Errorf("failure reading header %s from imdb response", imdbHeaderKeyContentDisposition)
		}
		_, params, err := mime.ParseMediaType(contentDispositionHeader)
		if err != nil || len(params) == 0 {
			return nil, nil, fmt.Errorf("failure parsing media type from imdb header %s: %w", imdbHeaderKeyContentDisposition, err)
		}
		imdbListName = &strings.Split(params["filename"], ".")[0]
	case resourceTypeRating:
		for i, record := range csvData {
			if i > 0 {
				rating, err := strconv.Atoi(record[1])
				if err != nil {
					return nil, nil, fmt.Errorf("failure parsing imdb rating value to integer: %w", err)
				}
				ratingDate, err := time.Parse("2006-01-02", record[2])
				if err != nil {
					return nil, nil, fmt.Errorf("failure parsing imdb rating date: %w", err)
				}
				imdbList = append(imdbList, entities.ImdbItem{
					Id:         record[0],
					TitleType:  record[5],
					Rating:     &rating,
					RatingDate: &ratingDate,
				})
			}
		}
	default:
		return nil, nil, fmt.Errorf("unknown imdb response type")
	}
	return imdbListName, imdbList, nil
}

func FormatTraktListName(imdbListName string) string {
	formatted := strings.ToLower(strings.Join(strings.Fields(imdbListName), "-"))
	re := regexp.MustCompile(`[^-a-z0-9]+`)
	return re.ReplaceAllString(formatted, "")
}

func DrainBody(body io.ReadCloser) error {
	err := body.Close()
	if err != nil {
		return fmt.Errorf("failure closing response body: %w", err)
	}
	return nil
}
