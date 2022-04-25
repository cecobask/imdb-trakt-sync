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
	"strings"
	"time"
)

const (
	contentDispositionHeaderName = "Content-Disposition"
	imdbCookieAtMain             = "at-main"
	imdbCookieAtMainKey          = "IMDB_COOKIE_AT_MAIN"
	imdbCookieUbidMain           = "ubid-main"
	imdbCookieUbidMainKey        = "IMDB_COOKIE_UBID_MAIN"
	imdbCustomListIdsKey         = "IMDB_CUSTOM_LIST_IDS"
	imdbBasePath                 = "https://www.imdb.com/"
	imdbListExportPath           = "list/%s/export/"
	imdbUserIdKey                = "IMDB_USER_ID"
	imdbWatchlistIdKey           = "IMDB_WATCHLIST_ID"
)

type imdbClient struct {
	endpoint    string
	client      *http.Client
	credentials imdbCredentials
}

type imdbCredentials struct {
	imdbCookieAtMain   string
	imdbCookieUbidMain string
}

type imdbListItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Year  string `json:"year"`
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
		credentials: imdbCredentials{
			imdbCookieAtMain:   os.Getenv(imdbCookieAtMainKey),
			imdbCookieUbidMain: os.Getenv(imdbCookieUbidMainKey),
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

func (ic *imdbClient) getList(listID string) (string, []imdbListItem) {
	res, err := ic.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf(imdbListExportPath, listID),
	})
	if err != nil {
		log.Fatalf("error retrieving imdb list %s: %v", listID, err)
	}
	defer closeBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error retrieving imdb list %s: %v, update the imdb cookie values", listID, res.StatusCode)
	case http.StatusNotFound:
		log.Printf("error retrieving imdb list %s: %v", listID, res.StatusCode)
		return "", nil
	default:
		log.Fatalf("error retrieving imdb list %s: %v", listID, res.StatusCode)
	}
	return readImdbListItems(res)
}

func readImdbListItems(res *http.Response) (string, []imdbListItem) {
	contentDispositionHeader := res.Header.Get(contentDispositionHeaderName)
	if contentDispositionHeader == "" {
		log.Fatalf("error reading header %s from imdb response", contentDispositionHeaderName)
	}
	_, params, err := mime.ParseMediaType(contentDispositionHeader)
	if err != nil || len(params) == 0 {
		log.Fatalf("error parsing media type from header: %v", err)
	}
	listName := strings.Split(params["filename"], ".")[0]
	csvReader := csv.NewReader(res.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		log.Fatalf("error reading imdb response: %v", err)
	}
	var imdbListItems []imdbListItem
	for i, record := range csvData {
		if i > 0 { // omit header line
			imdbListItems = append(imdbListItems, imdbListItem{
				ID:    record[1],
				Title: record[5],
				Type:  record[7],
				Year:  record[10],
			})
		}
	}
	return listName, imdbListItems
}
