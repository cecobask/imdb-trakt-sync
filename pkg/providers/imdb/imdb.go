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
	EnvVarKeyCookieAtMain       = "IMDB_COOKIE_AT_MAIN"
	EnvVarKeyCookieUbidMain     = "IMDB_COOKIE_UBID_MAIN"
	EnvVarKeyListIds            = "IMDB_LIST_IDS"
	cookieNameAtMain            = "at-main"
	cookieNameUbidMain          = "ubid-main"
	headerKeyContentDisposition = "Content-Disposition"
	itemTypeMovie               = "movie"
	itemTypeTvEpisode           = "tvEpisode"
	itemTypeTvMiniSeries        = "tvMiniSeries"
	itemTypeTvSeries            = "tvSeries"
	pathBase                    = "https://www.imdb.com"
	pathListExport              = "/list/%s/export/"
	pathLists                   = "/user/%s/lists/"
	pathProfile                 = "/profile"
	pathRatingsExport           = "/user/%s/ratings/export/"
	pathWatchlist               = "/watchlist"
	responseTypeList            = iota
	responseTypeRatings
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
		endpoint: pathBase,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout: 5 * time.Second,
			},
			CheckRedirect: nil,
		},
		Config: config{
			cookieAtMain:   os.Getenv(EnvVarKeyCookieAtMain),
			cookieUbidMain: os.Getenv(EnvVarKeyCookieUbidMain),
		},
	}
}

func (c *Client) doRequest(params requestParams) *http.Response {
	req, err := http.NewRequest(params.Method, c.endpoint+params.Path, nil)
	if err != nil {
		log.Fatalf("error creating http request %s, %s: %v", params.Method, c.endpoint+params.Path, err)
	}
	req.AddCookie(&http.Cookie{
		Name:  cookieNameAtMain,
		Value: os.Getenv(EnvVarKeyCookieAtMain),
	})
	req.AddCookie(&http.Cookie{
		Name:  cookieNameUbidMain,
		Value: os.Getenv(EnvVarKeyCookieUbidMain),
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
		Path:   fmt.Sprintf(pathListExport, listId),
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
	listName, list := readResponse(res, responseTypeList)
	return listName, list, nil
}

func (c *Client) ListsScrape() (dp []DataPair) {
	res := c.doRequest(requestParams{
		Method: http.MethodGet,
		Path:   fmt.Sprintf(pathLists, c.Config.UserId),
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
		Path:   pathProfile,
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
		Path:   pathWatchlist,
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
		Path:   fmt.Sprintf(pathRatingsExport, c.Config.UserId),
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
	_, ratings := readResponse(res, responseTypeRatings)
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
	case responseTypeList:
		for i, record := range csvData {
			if i > 0 { // omit header line
				imdbList = append(imdbList, Item{
					Id:        record[1],
					TitleType: record[7],
				})
			}
		}
		contentDispositionHeader := res.Header.Get(headerKeyContentDisposition)
		if contentDispositionHeader == "" {
			log.Fatalf("error reading header %s from imdb response", headerKeyContentDisposition)
		}
		_, params, err := mime.ParseMediaType(contentDispositionHeader)
		if err != nil || len(params) == 0 {
			log.Fatalf("error parsing media type from header: %v", err)
		}
		imdbListName = &strings.Split(params["filename"], ".")[0]
	case responseTypeRatings:
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
	// items to be added to trakt
	temp := make(map[string]struct{})
	for _, tlItem := range dp.TraktList {
		switch tlItem.Type {
		case trakt.ItemTypeMovie:
			temp[tlItem.Movie.Ids.Imdb] = struct{}{}
		case trakt.ItemTypeShow:
			temp[tlItem.Show.Ids.Imdb] = struct{}{}
		case trakt.ItemTypeEpisode:
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
			case itemTypeMovie:
				ti.Type = trakt.ItemTypeMovie
				ti.Movie = tiSpec
			case itemTypeTvSeries:
				ti.Type = trakt.ItemTypeShow
				ti.Show = tiSpec
			case itemTypeTvMiniSeries:
				ti.Type = trakt.ItemTypeShow
				ti.Show = tiSpec
			case itemTypeTvEpisode:
				ti.Type = trakt.ItemTypeEpisode
				ti.Episode = tiSpec
			default:
				ti.Type = trakt.ItemTypeMovie
				ti.Movie = tiSpec
			}
			diff["add"] = append(diff["add"], ti)
		}
	}
	// out of sync items to be removed from trakt
	temp = make(map[string]struct{})
	for _, ilItem := range dp.ImdbList {
		temp[ilItem.Id] = struct{}{}
	}
	for _, tlItem := range dp.TraktList {
		var itemId string
		switch tlItem.Type {
		case trakt.ItemTypeMovie:
			itemId = tlItem.Movie.Ids.Imdb
		case trakt.ItemTypeShow:
			itemId = tlItem.Show.Ids.Imdb
		case trakt.ItemTypeEpisode:
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
