package imdb

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/cecobask/imdb-trakt-sync/pkg/client"
	"github.com/cecobask/imdb-trakt-sync/pkg/providers/trakt"
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
	basePath                     = "https://www.imdb.com/"
	cookieAtMain                 = "at-main"
	CookieAtMainKey              = "IMDB_COOKIE_AT_MAIN"
	cookieUbidMain               = "ubid-main"
	CookieUbidMainKey            = "IMDB_COOKIE_UBID_MAIN"
	listExportPath               = "list/%s/export/"
	ListIdsKey                   = "IMDB_LIST_IDS"
	listsPath                    = "user/%s/lists/"
	ratingsExportPath            = "user/%s/ratings/export/"
	profilePath                  = "profile"
	watchlistPath                = "watchlist"
	listResponseType             = iota
	ratingsResponseType
)

type Client struct {
	endpoint string
	client   *http.Client
	Config   config
}

type requestParams struct {
	Method string
	Path   string
	Body   interface{}
}

type config struct {
	cookieAtMain   string
	cookieUbidMain string
	UserId         string
	WatchlistId    string
}

type Item struct {
	Id         string
	TitleType  string
	Rating     *int
	RatingDate *time.Time
}

type DataPair struct {
	ImdbList     []Item
	ImdbListId   string
	ImdbListName string
	TraktList    []trakt.Item
	TraktListId  string
	IsWatchlist  bool
}

func NewClient() *Client {
	return &Client{
		endpoint: basePath,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout: 5 * time.Second,
			},
			CheckRedirect: nil,
		},
		Config: config{
			cookieAtMain:   os.Getenv(CookieAtMainKey),
			cookieUbidMain: os.Getenv(CookieUbidMainKey),
		},
	}
}

func (c *Client) doRequest(params requestParams) *http.Response {
	req, err := http.NewRequest(params.Method, c.endpoint+params.Path, nil)
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", params.Method, c.endpoint+params.Path, err)
	}
	req.AddCookie(&http.Cookie{
		Name:  cookieAtMain,
		Value: os.Getenv(CookieAtMainKey),
	})
	req.AddCookie(&http.Cookie{
		Name:  cookieUbidMain,
		Value: os.Getenv(CookieUbidMainKey),
	})
	if params.Body != nil {
		body, err := json.Marshal(params.Body)
		if err != nil {
			log.Fatalf("error marshalling request body %s, %s: %v", params.Method, c.endpoint+params.Path, err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	res, err := c.client.Do(req)
	if err != nil {
		log.Fatalf("error sending http request %s, %s: %v", params.Method, c.endpoint+params.Path, err)
	}
	return res
}

func (c *Client) ListItemsGet(listId string) (*string, []Item, error) {
	res := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(listExportPath, listId),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error retrieving imdb list %s for user %s: update the imdb cookie values", listId, c.Config.UserId)
	case http.StatusNotFound:
		log.Printf("error retrieving imdb list %s for user %s: %v", listId, c.Config.UserId, res.StatusCode)
		return nil, nil, errors.New("resource not found")
	default:
		log.Fatalf("error retrieving imdb list %s for user %s: %v", listId, c.Config.UserId, res.StatusCode)
	}
	listName, list := readResponse(res, listResponseType)
	return listName, list, nil
}

func (c *Client) ListsScrape() (dp []DataPair) {
	res := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(listsPath, c.Config.UserId),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error scraping imdb lists for user %s: update the imdb cookie values", c.Config.UserId)
	default:
		log.Fatalf("error scraping imdb lists for user %s: %v", c.Config.UserId, res.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalf("error creating goquery document from imdb response: %v", err)
	}
	doc.Find(".user-list").Each(func(i int, selection *goquery.Selection) {
		imdbListId, ok := selection.Attr("id")
		if !ok {
			log.Fatalf("error scraping imdb lists for user %s: none found", c.Config.UserId)
		}
		imdbListName, imdbList, err := c.ListItemsGet(imdbListId)
		if errors.Is(err, errors.New("")) {
			return
		}
		dp = append(dp, DataPair{
			ImdbList:     imdbList,
			ImdbListId:   imdbListId,
			ImdbListName: *imdbListName,
			TraktListId:  FormatTraktListName(*imdbListName),
		})
	})
	return dp
}

func (c *Client) UserIdScrape() string {
	res := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   profilePath,
	})
	defer client.DrainBody(res.Body)
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

func (c *Client) WatchlistIdScrape() string {
	res := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   watchlistPath,
	})
	defer client.DrainBody(res.Body)
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

func (c *Client) RatingsGet() []Item {
	res := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(ratingsExportPath, c.Config.UserId),
	})
	defer client.DrainBody(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden:
		log.Fatalf("error retrieving imdb ratings for user %s: update the imdb cookie values", c.Config.UserId)
	case http.StatusNotFound:
		log.Printf("error retrieving imdb ratings for user %s: none found", c.Config.UserId)
		return nil
	default:
		log.Fatalf("error retrieving imdb ratings for user %s: %v", c.Config.UserId, res.StatusCode)
	}
	_, ratings := readResponse(res, ratingsResponseType)
	return ratings
}

func readResponse(res *http.Response, resType int) (imdbListName *string, imdbList []Item) {
	csvReader := csv.NewReader(res.Body)
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		log.Fatalf("error reading imdb response: %v", err)
	}
	switch resType {
	case listResponseType:
		for i, record := range csvData {
			if i > 0 { // omit header line
				imdbList = append(imdbList, Item{
					Id:        record[1],
					TitleType: record[7],
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
	case ratingsResponseType:
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
				imdbList = append(imdbList, Item{
					Id:         record[0],
					TitleType:  record[5],
					Rating:     &rating,
					RatingDate: &ratingDate,
				})
			}
		}
	default:
		log.Fatalf("unknown imdb response type")
	}
	return imdbListName, imdbList
}

func (dp *DataPair) Difference() map[string][]trakt.Item {
	diff := make(map[string][]trakt.Item)
	// add missing items to trakt
	temp := make(map[string]struct{})
	for _, tlItem := range dp.TraktList {
		switch tlItem.Type {
		case "movie":
			temp[tlItem.Movie.Ids.Imdb] = struct{}{}
		case "show":
			temp[tlItem.Show.Ids.Imdb] = struct{}{}
		case "episode":
			temp[tlItem.Episode.Ids.Imdb] = struct{}{}
		default:
			continue
		}
	}
	for _, ilItem := range dp.ImdbList {
		if _, found := temp[ilItem.Id]; !found {
			ti := trakt.Item{}
			tiSpec := trakt.ItemSpec{
				Ids: trakt.Ids{
					Imdb: ilItem.Id,
				},
			}
			if ilItem.Rating != nil {
				ti.WatchedAt = ilItem.RatingDate.UTC().String()
				tiSpec.RatedAt = ilItem.RatingDate.UTC().String()
				tiSpec.Rating = *ilItem.Rating
			}
			switch ilItem.TitleType {
			case "movie":
				ti.Type = "movie"
				ti.Movie = tiSpec
			case "tvSeries":
				ti.Type = "show"
				ti.Show = tiSpec
			case "tvMiniSeries":
				ti.Type = "show"
				ti.Show = tiSpec
			case "tvEpisode":
				ti.Type = "episode"
				ti.Episode = tiSpec
			default:
				ti.Type = "movie"
				ti.Movie = tiSpec
			}
			diff["add"] = append(diff["add"], ti)
		}
	}
	// remove out of sync items from trakt
	temp = make(map[string]struct{})
	for _, ilItem := range dp.ImdbList {
		temp[ilItem.Id] = struct{}{}
	}
	for _, tlItem := range dp.TraktList {
		var itemId string
		switch tlItem.Type {
		case "movie":
			itemId = tlItem.Movie.Ids.Imdb
		case "show":
			itemId = tlItem.Show.Ids.Imdb
		case "episode":
			itemId = tlItem.Episode.Ids.Imdb
		default:
			continue
		}
		if _, found := temp[itemId]; !found {
			diff["remove"] = append(diff["remove"], tlItem)
		}
	}
	return diff
}

func FormatTraktListName(imdbListName string) string {
	formatted := strings.ToLower(strings.Join(strings.Fields(imdbListName), "-"))
	re := regexp.MustCompile(`[^-a-z0-9]+`)
	return re.ReplaceAllString(formatted, "")
}
