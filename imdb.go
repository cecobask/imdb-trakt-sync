package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	contentDispositionHeaderName = "Content-Disposition"
	imdbBasePath                 = "https://www.imdb.com/"
	imdbCookieAtMain             = "at-main"
	imdbCookieAtMainKey          = "IMDB_COOKIE_AT_MAIN"
	imdbCookieUbidMain           = "ubid-main"
	imdbCookieUbidMainKey        = "IMDB_COOKIE_UBID_MAIN"
	imdbListExportPath           = "list/%s/export/"
	imdbListIdsKey               = "IMDB_LIST_IDS"
	imdbListsPath                = "user/%s/lists/"
	imdbRatingsExportPath        = "user/%s/ratings/export/"
	imdbProfilePath              = "profile"
	imdbWatchlistPath            = "watchlist"
	imdbListResponseType         = iota
	imdbRatingsResponseType
)

type imdbClient struct {
	endpoint string
	client   *http.Client
	config   imdbConfig
}

type imdbConfig struct {
	imdbCookieAtMain   string
	imdbCookieUbidMain string
	imdbUserId         string
	imdbWatchlistId    string
}

type imdbItem struct {
	id         string
	titleType  string
	rating     *int
	ratingDate *time.Time
}

func newImdbClient() *imdbClient {
	return &imdbClient{
		endpoint: imdbBasePath,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout: 5 * time.Second,
			},
			CheckRedirect: nil,
		},
		config: imdbConfig{
			imdbCookieAtMain:   os.Getenv(imdbCookieAtMainKey),
			imdbCookieUbidMain: os.Getenv(imdbCookieUbidMainKey),
		},
	}
}

func (ic *imdbClient) doRequest(params requestParams) *http.Response {
	req, err := http.NewRequest(params.method, ic.endpoint+params.path, nil)
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", params.method, ic.endpoint+params.path, err)
	}
	req.AddCookie(&http.Cookie{
		Name:  imdbCookieAtMain,
		Value: os.Getenv(imdbCookieAtMainKey),
	})
	req.AddCookie(&http.Cookie{
		Name:  imdbCookieUbidMain,
		Value: os.Getenv(imdbCookieUbidMainKey),
	})
	if params.body != nil {
		body, err := json.Marshal(params.body)
		if err != nil {
			log.Fatalf("error marshalling request body %s, %s: %v", params.method, ic.endpoint+params.path, err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	res, err := ic.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", params.method, ic.endpoint+params.path, err)
	}
	return res
}

func (ic *imdbClient) listItemsGet(listId string) (*string, []imdbItem, error) {
	res := ic.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(imdbListExportPath, listId),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error retrieving imdb list %s for user %s: update the imdb cookie values", listId, ic.config.imdbUserId)
	case http.StatusNotFound:
		log.Printf("error retrieving imdb list %s for user %s: %v", listId, ic.config.imdbUserId, res.StatusCode)
		return nil, nil, errNotFound
	default:
		log.Fatalf("error retrieving imdb list %s for user %s: %v", listId, ic.config.imdbUserId, res.StatusCode)
	}
	listName, list := readImdbResponse(res, imdbListResponseType)
	return listName, list, nil
}

func (ic *imdbClient) listsScrape() (dp []dataPair) {
	res := ic.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(imdbListsPath, ic.config.imdbUserId),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error scraping imdb lists for user %s: update the imdb cookie values", ic.config.imdbUserId)
	default:
		log.Fatalf("error scraping imdb lists for user %s: %v", ic.config.imdbUserId, res.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalf("error creating goquery document from imdb response: %v", err)
	}
	doc.Find(".user-list").Each(func(i int, selection *goquery.Selection) {
		imdbListId, ok := selection.Attr("id")
		if !ok {
			log.Fatalf("error scraping imdb lists for user %s: none found", ic.config.imdbUserId)
		}
		imdbListName, imdbList, err := ic.listItemsGet(imdbListId)
		if errors.Is(err, errNotFound) {
			return
		}
		dp = append(dp, dataPair{
			imdbList:     imdbList,
			imdbListId:   imdbListId,
			imdbListName: *imdbListName,
			traktListId:  formatTraktListName(*imdbListName),
		})
	})
	return dp
}

func (ic *imdbClient) userIdScrape() string {
	res := ic.doRequest(requestParams{
		method: http.MethodGet,
		path:   imdbProfilePath,
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error scraping imdb profile: update the imdb cookie values")
	default:
		log.Fatalf("error scraping imdb profile: %v", res.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalf("error creating goquery document from imdb response: %v", err)
	}
	userId, ok := doc.Find(".user-profile.userId").Attr("data-userid")
	if !ok {
		log.Fatalf("error scraping imdb profile: user id not found")
	}
	return userId
}

func (ic *imdbClient) watchlistIdScrape() string {
	res := ic.doRequest(requestParams{
		method: http.MethodGet,
		path:   imdbWatchlistPath,
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error scraping imdb watchlist id: update the imdb cookie values")
	default:
		log.Fatalf("error scraping imdb watchlist id: %v", res.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalf("error creating goquery document from imdb response: %v", err)
	}
	watchlistId, ok := doc.Find("meta[property='pageId']").Attr("content")
	if !ok {
		log.Fatalf("error scraping imdb watchlist id: watchlist id not found")
	}
	return watchlistId
}

func (ic *imdbClient) ratingsGet() []imdbItem {
	res := ic.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(imdbRatingsExportPath, ic.config.imdbUserId),
	})
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error retrieving imdb ratings for user %s: update the imdb cookie values", ic.config.imdbUserId)
	case http.StatusNotFound:
		log.Printf("error retrieving imdb ratings for user %s: none found", ic.config.imdbUserId)
		return nil
	default:
		log.Fatalf("error retrieving imdb ratings for user %s: %v", ic.config.imdbUserId, res.StatusCode)
	}
	_, ratings := readImdbResponse(res, imdbRatingsResponseType)
	return ratings
}

func readImdbResponse(res *http.Response, resType int) (imdbListName *string, imdbList []imdbItem) {
	csvReader := csv.NewReader(res.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		log.Fatalf("error reading imdb response: %v", err)
	}
	switch resType {
	case imdbListResponseType:
		for i, record := range csvData {
			if i > 0 { // omit header line
				imdbList = append(imdbList, imdbItem{
					id:        record[1],
					titleType: record[7],
				})
			}
		}
		contentDispositionHeader := res.Header.Get(contentDispositionHeaderName)
		if contentDispositionHeader == "" {
			log.Fatalf("error reading header %s from imdb response", contentDispositionHeaderName)
		}
		_, params, err := mime.ParseMediaType(contentDispositionHeader)
		if err != nil || len(params) == 0 {
			log.Fatalf("error parsing media type from header: %v", err)
		}
		imdbListName = &strings.Split(params["filename"], ".")[0]
	case imdbRatingsResponseType:
		for i, record := range csvData {
			if i > 0 {
				rating, err := strconv.Atoi(record[1])
				if err != nil {
					log.Fatalf("error parsing imdb rating value: %v", err)
				}
				ratingDate, err := time.Parse("2006-01-02", record[2])
				if err != nil {
					log.Fatalf("error parsing imdb rating date: %v", err)
				}
				imdbList = append(imdbList, imdbItem{
					id:         record[0],
					titleType:  record[5],
					rating:     &rating,
					ratingDate: &ratingDate,
				})
			}
		}
	default:
		log.Fatalf("unknown imdb response type")
	}
	return imdbListName, imdbList
}

func formatTraktListName(imdbListName string) string {
	formatted := strings.ToLower(strings.Join(strings.Fields(imdbListName), "-"))
	re := regexp.MustCompile(`[^-a-z0-9]+`)
	return re.ReplaceAllString(formatted, "")
}
