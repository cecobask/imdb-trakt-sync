package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
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
	imdbListIdsKey               = "IMDB_LIST_IDS"
	imdbListExportPath           = "list/%s/export/"
	imdbListResponseType         = iota
	imdbRatingsResponseType
	imdbRatingsExportPath = "user/%s/ratings/export/"
	imdbUserIdKey         = "IMDB_USER_ID"
	imdbWatchlistIdKey    = "IMDB_WATCHLIST_ID"
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
		},
		config: imdbConfig{
			imdbCookieAtMain:   os.Getenv(imdbCookieAtMainKey),
			imdbCookieUbidMain: os.Getenv(imdbCookieUbidMainKey),
			imdbUserId:         os.Getenv(imdbUserIdKey),
			imdbWatchlistId:    os.Getenv(imdbWatchlistIdKey),
		},
	}
}

func (ic *imdbClient) doRequest(params requestParams) (*http.Response, error) {
	req, err := http.NewRequest(params.method, ic.endpoint+params.path, nil)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	resp, err := ic.client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (ic *imdbClient) listItemsGet(listId string) (string, []imdbItem) {
	res, err := ic.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(imdbListExportPath, listId),
	})
	if err != nil {
		log.Fatalf("error retrieving imdb list %s: %v", listId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error retrieving imdb list %s: %v, update the imdb cookie values", listId, res.StatusCode)
	case http.StatusNotFound:
		log.Printf("error retrieving imdb list %s: %v", listId, res.StatusCode)
		return "", nil
	default:
		log.Fatalf("error retrieving imdb list %s: %v", listId, res.StatusCode)
	}
	return readImdbResponse(res, imdbListResponseType)
}

func (ic *imdbClient) ratingsGet() []imdbItem {
	res, err := ic.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(imdbRatingsExportPath, ic.config.imdbUserId),
	})
	if err != nil {
		log.Fatalf("error retrieving imdb ratings for user %s: %v", ic.config.imdbUserId, err)
	}
	defer drainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error retrieving imdb ratings for user %s: %v, update the imdb cookie values", ic.config.imdbUserId, res.StatusCode)
	case http.StatusNotFound:
		log.Printf("error retrieving imdb ratings for user %s: %v", ic.config.imdbUserId, res.StatusCode)
		return nil
	default:
		log.Fatalf("error retrieving imdb ratings for user %s: %v", ic.config.imdbUserId, res.StatusCode)
	}
	_, ratings := readImdbResponse(res, imdbRatingsResponseType)
	return ratings
}

func readImdbResponse(res *http.Response, resType int) (string, []imdbItem) {
	csvReader := csv.NewReader(res.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		log.Fatalf("error reading imdb response: %v", err)
	}
	var imdbItems []imdbItem
	listName := ""
	switch resType {
	case imdbListResponseType:
		for i, record := range csvData {
			if i > 0 { // omit header line
				imdbItems = append(imdbItems, imdbItem{
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
		listName = strings.Split(params["filename"], ".")[0]
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
				imdbItems = append(imdbItems, imdbItem{
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
	return listName, imdbItems
}
