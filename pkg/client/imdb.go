package client

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"golang.org/x/sync/errgroup"

	appconfig "github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/entities"
)

const (
	imdbCookieNameAtMain   = "at-main"
	imdbCookieNameUbidMain = "ubid-main"
	imdbPathBase           = "https://www.imdb.com"
	imdbPathExports        = "/exports"
	imdbPathList           = "/list/%s"
	imdbPathLists          = "/profile/lists"
	imdbPathProfile        = "/profile"
	imdbPathRatings        = "/list/ratings"
	imdbPathSignIn         = "/registration/ap-signin-handler/imdb_us"
	imdbPathWatchlist      = "/list/watchlist"
)

type IMDbClient struct {
	client  *http.Client
	config  *imdbConfig
	logger  *slog.Logger
	browser *rod.Browser
}

type imdbConfig struct {
	*appconfig.IMDb
	basePath    string
	userID      string
	username    string
	watchlistID string
}

func NewIMDbClient(ctx context.Context, conf *appconfig.IMDb, logger *slog.Logger) (IMDbClientInterface, error) {
	config := imdbConfig{
		IMDb:     conf,
		basePath: imdbPathBase,
	}
	//uncomment below for local debugging
	//debugURL := launcher.New().Headless(false).MustLaunch()
	//browser := rod.New().ControlURL(debugURL).Trace(true)
	browser := rod.New().Context(ctx).Trace(conf.Trace)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failure connecting to browser: %w", err)
	}
	cookies, err := authenticateUser(browser, config)
	if err != nil {
		return nil, fmt.Errorf("failure authenticating user: %w", err)
	}
	logger.Info("authenticated user", slog.String("email", *conf.Email))
	jar, err := setupCookieJar(config.basePath, cookies)
	if err != nil {
		return nil, fmt.Errorf("failure setting up cookie jar: %w", err)
	}
	c := &IMDbClient{
		client: &http.Client{
			Jar: jar,
		},
		config:  &config,
		logger:  logger,
		browser: browser,
	}
	if err = c.hydrate(); err != nil {
		return nil, fmt.Errorf("failure hydrating client: %w", err)
	}
	return c, nil
}

func setupCookieJar(basePath string, cookies []*http.Cookie) (http.CookieJar, error) {
	imdbUrl, err := url.Parse(basePath)
	if err != nil {
		return nil, fmt.Errorf("failure parsing %s as url: %w", basePath, err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failure creating cookie jar: %w", err)
	}
	jar.SetCookies(imdbUrl, cookies)
	return jar, nil
}

func authenticateUser(browser *rod.Browser, config imdbConfig) ([]*http.Cookie, error) {
	signInURL := imdbPathBase + imdbPathSignIn
	tab, err := browser.Page(proto.TargetCreateTarget{
		URL: signInURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failure opening browser tab: %w", err)
	}
	defer tab.MustClose()
	if err = tab.WaitLoad(); err != nil {
		return nil, fmt.Errorf("failure waiting for sign in tab to load: %w", err)
	}
	emailField, err := tab.Element("#ap_email")
	if err != nil {
		return nil, fmt.Errorf("failure finding email field: %w", err)
	}
	if err = emailField.Input(*config.Email); err != nil {
		return nil, fmt.Errorf("failure inputting value in email field: %w", err)
	}
	passwordField, err := tab.Element("#ap_password")
	if err != nil {
		return nil, fmt.Errorf("failure finding password field: %w", err)
	}
	if err = passwordField.Input(*config.Password); err != nil {
		return nil, fmt.Errorf("failure inputting value in password field: %w", err)
	}
	submitButton, err := tab.Element("#signInSubmit")
	if err != nil {
		return nil, fmt.Errorf("failure finding submit button: %w", err)
	}
	if err = submitButton.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("failure clicking on submit button: %w", err)
	}
	authResult, err := tab.Race().Element("#nblogout").Element("#auth-error-message-box").Do()
	if err != nil {
		return nil, fmt.Errorf("failure doing selector race: %w", err)
	}
	authFailed, err := authResult.Matches("#auth-error-message-box")
	if err != nil {
		return nil, fmt.Errorf("failure checking for authentication error match: %w", err)
	}
	if authFailed {
		return nil, fmt.Errorf("failure authenticating with the provided credentials")
	}
	return getRequiredCookies(tab)
}

func (c *IMDbClient) hydrate() error {
	if err := c.userIdentityScrape(); err != nil {
		return fmt.Errorf("failure scraping imdb user identity: %w", err)
	}
	if err := c.watchlistIDScrape(); err != nil {
		return fmt.Errorf("failure scraping imdb watchlist id: %w", err)
	}
	if len(*c.config.Lists) == 0 {
		lids, err := c.lidsScrape()
		if err != nil {
			return fmt.Errorf("failure scraping list ids: %w", err)
		}
		c.config.Lists = &lids
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

func (c *IMDbClient) WatchlistExport() error {
	return c.ListExport(c.config.watchlistID)
}

func (c *IMDbClient) WatchlistGet() (*entities.IMDbList, error) {
	lists, err := c.ListsGet(c.config.watchlistID)
	if err != nil {
		return nil, fmt.Errorf("failure downloading watchlist: %w", err)
	}
	return &lists[0], nil
}

func (c *IMDbClient) ListExport(id string) error {
	listURL := imdbPathBase + fmt.Sprintf(imdbPathList, id)
	if err := c.exportResource(listURL); err != nil {
		return fmt.Errorf("failure exporting list %s", id)
	}
	c.logger.Info("exported list", slog.String("id", id))
	return nil
}

func (c *IMDbClient) ListsExport(ids ...string) error {
	errGroup, _ := errgroup.WithContext(c.browser.GetContext())
	for _, id := range ids {
		errGroup.Go(func() error {
			return c.ListExport(id)
		})
	}
	return errGroup.Wait()
}

func (c *IMDbClient) ListsGet(ids ...string) ([]entities.IMDbList, error) {
	resources, cleanupFunc, err := c.getExportedResources(ids...)
	if err != nil {
		return nil, fmt.Errorf("failure fetching exported resources: %w", err)
	}
	defer cleanupFunc()
	filteredResources, err := c.filterResources(resources, ids...)
	if err != nil {
		return nil, fmt.Errorf("failure filtering resources: %w", err)
	}
	lists := make([]entities.IMDbList, len(ids))
	for i, listResource := range filteredResources {
		list, err := c.listDownload(listResource)
		if err != nil {
			return nil, fmt.Errorf("failure downloading list: %w", err)
		}
		lists[i] = *list
	}
	return lists, nil
}

func (c *IMDbClient) RatingsExport() error {
	ratingsURL := imdbPathBase + imdbPathRatings
	if err := c.exportResource(ratingsURL); err != nil {
		return fmt.Errorf("failure exporting ratings resource: %w", err)
	}
	c.logger.Info("exported ratings")
	return nil
}

func (c *IMDbClient) RatingsGet() ([]entities.IMDbItem, error) {
	resources, cleanupFunc, err := c.getExportedResources(c.config.userID)
	if err != nil {
		return nil, fmt.Errorf("failure fetching exported resources: %w", err)
	}
	defer cleanupFunc()
	filteredResources, err := c.filterResources(resources, c.config.userID)
	if err != nil {
		return nil, fmt.Errorf("failure filtering resources: %w", err)
	}
	return c.ratingsDownload(filteredResources[0])
}

func (c *IMDbClient) ratingsDownload(resource *rod.Element) ([]entities.IMDbItem, error) {
	downloadButton, err := resource.Element("button[data-testid='export-status-button']")
	if err != nil {
		return nil, fmt.Errorf("failure finding download button: %w", err)
	}
	wait := c.browser.MustWaitDownload()
	if err = downloadButton.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("failure clicking on download button: %w", err)
	}
	items, err := transformRatingsData(wait())
	if err != nil {
		return nil, fmt.Errorf("failure transforming ratings data: %w", err)
	}
	c.logger.Info("downloaded ratings", slog.Int("count", len(items)))
	return items, nil
}

func (c *IMDbClient) listDownload(resource *rod.Element) (*entities.IMDbList, error) {
	hyperlink, err := resource.Element("a.ipc-metadata-list-summary-item__t")
	if err != nil {
		return nil, fmt.Errorf("failure finding resource hyperlink: %w", err)
	}
	href, err := hyperlink.Attribute("href")
	if err != nil {
		return nil, fmt.Errorf("failure extracting href from hyperlink: %w", err)
	}
	lid, err := lidExtract(*href)
	if err != nil {
		return nil, fmt.Errorf("failure extracting list id from href: %w", err)
	}
	listName, err := hyperlink.Text()
	if err != nil {
		return nil, fmt.Errorf("failure extracting list name from hyperlink: %w", err)
	}
	downloadButton, err := resource.Element("button[data-testid='export-status-button']")
	if err != nil {
		return nil, fmt.Errorf("failure finding download button: %w", err)
	}
	wait := c.browser.MustWaitDownload()
	if err = downloadButton.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("failure clicking on download button: %w", err)
	}
	items, err := transformListData(wait())
	if err != nil {
		return nil, fmt.Errorf("failure transforming list data: %w", err)
	}
	c.logger.Info("downloaded list", slog.String("id", lid), slog.String("name", listName), slog.Int("count", len(items)))
	return &entities.IMDbList{
		ListID:      lid,
		ListName:    listName,
		ListItems:   items,
		IsWatchlist: lid == c.config.watchlistID,
	}, nil
}

func (c *IMDbClient) getExportedResources(ids ...string) (rod.Elements, func(), error) {
	exportsURL := imdbPathBase + imdbPathExports
	tab, err := c.browser.Page(proto.TargetCreateTarget{
		URL: exportsURL,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failure opening browser tab: %w", err)
	}
	if err = tab.WaitLoad(); err != nil {
		return nil, nil, fmt.Errorf("failure waiting for exports tab to load: %w", err)
	}
	if err = c.waitExportsReady(tab, ids...); err != nil {
		return nil, nil, fmt.Errorf("failure waiting for exports to become available: %w", err)
	}
	resources, err := tab.Elements("li[data-testid='user-ll-item']")
	if err != nil {
		return nil, nil, fmt.Errorf("failure finding exported resources: %w", err)
	}
	cleanupFunc := func() {
		tab.MustClose()
	}
	return resources, cleanupFunc, nil
}

func (c *IMDbClient) exportResource(url string) error {
	tab, err := c.browser.Page(proto.TargetCreateTarget{
		URL: url,
	})
	if err != nil {
		return fmt.Errorf("failure opening browser tab: %w", err)
	}
	defer tab.MustClose()
	if err = tab.WaitLoad(); err != nil {
		return fmt.Errorf("failure waiting for resource tab to load: %w", err)
	}
	exportButton, err := tab.Element("div[data-testid='hero-list-subnav-export-button'] button")
	if err != nil {
		return fmt.Errorf("failure finding export resource button: %w", err)
	}
	if err = exportButton.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("failure clicking on export resource button: %w", err)
	}
	if _, err = tab.Element("a.exp-pmpt__btn"); err != nil {
		return fmt.Errorf("failure finding exports page button: %w", err)
	}
	return nil
}

func buildSelector(ids ...string) string {
	var selectors strings.Builder
	for i, id := range ids {
		selectors.WriteString(fmt.Sprintf(`a.ipc-metadata-list-summary-item__t[href*='%s']`, id))
		if i != len(ids)-1 {
			selectors.WriteString(",")
		}
	}
	// () => [...document.querySelectorAll("a.ipc-metadata-list-summary-item__t[href*='ls123456789'],a.ipc-metadata-list-summary-item__t[href*='ls987654321']")].map(el => el.closest('li'));
	format := `() => [...document.querySelectorAll("%s")].map(hyperlink => hyperlink.closest("li"));`
	return fmt.Sprintf(format, selectors.String())

}

func (c *IMDbClient) waitExportsReady(tab *rod.Page, ids ...string) error {
	maxRetries := 15
	for attempts := 1; attempts <= maxRetries; attempts++ {
		if attempts == maxRetries {
			return fmt.Errorf("reached max retry attempts waiting for resources %s to become available", ids)
		}
		evalOpts := rod.Eval(buildSelector(ids...))
		elements, err := tab.ElementsByJS(evalOpts)
		if err != nil {
			return fmt.Errorf("failure filtering elements by js selector: %w", err)
		}
		var processingCount int
		for _, e := range elements {
			processing, _, err := e.Has("span[data-testid='export-status-button'].PROCESSING")
			if err != nil {
				return fmt.Errorf("failure looking up element export status: %w", err)
			}
			if processing {
				processingCount++
			}
		}
		if processingCount == 0 {
			c.logger.Info("exports are ready for download", slog.Any("ids", ids), slog.Int("count", len(ids)))
			break
		}
		time.Sleep(time.Second * 20)
		c.logger.Info("reloading exports tab to check the latest status")
		if err = tab.Reload(); err != nil {
			return fmt.Errorf("failure reloading exports tab: %w", err)
		}
		if err = tab.WaitLoad(); err != nil {
			return fmt.Errorf("failure waiting for exports tab to load: %w", err)
		}
	}
	return nil
}

func (c *IMDbClient) filterResources(resources rod.Elements, ids ...string) (rod.Elements, error) {
	uniqueResources := make(map[string]bool, len(ids))
	for _, id := range ids {
		uniqueResources[id] = false
	}
	filteredResources := make(rod.Elements, 0, len(ids))
	for _, r := range resources {
		if len(filteredResources) == len(ids) {
			break
		}
		hyperlink, err := r.Element("a.ipc-metadata-list-summary-item__t")
		if err != nil {
			return nil, fmt.Errorf("failure finding resource hyperlink: %w", err)
		}
		href, err := hyperlink.Attribute("href")
		if err != nil {
			return nil, fmt.Errorf("failure extracting href from resource hyperlink: %w", err)
		}
		if isListHyperlink(*href) {
			lid, err := lidExtract(*href)
			if err != nil {
				return nil, fmt.Errorf("failure extracting list id: %w", err)
			}
			if !uniqueResources[lid] && slices.Contains(ids, lid) {
				uniqueResources[lid] = true
				filteredResources = append(filteredResources, r)
			}
		}
		if isRatingsHyperlink(*href, c.config.userID) {
			return rod.Elements{r}, nil
		}
	}
	if len(filteredResources) != len(ids) {
		return nil, fmt.Errorf("expected the number of filtered resources to be %d, but got %d", len(ids), len(filteredResources))
	}
	return filteredResources, nil
}

func getRequiredCookies(tab *rod.Page) ([]*http.Cookie, error) {
	allCookies, err := tab.Cookies([]string{imdbPathBase})
	if err != nil {
		return nil, fmt.Errorf("failure retrieving imdb cookies: %w", err)
	}
	var cookies []*http.Cookie
	for _, c := range allCookies {
		if c.Name == imdbCookieNameUbidMain || c.Name == imdbCookieNameAtMain {
			cookies = append(cookies, &http.Cookie{
				Name:  c.Name,
				Value: c.Value,
			})
		}
	}
	if len(cookies) != 2 {
		return nil, fmt.Errorf("failure finding %s and %s cookies", imdbCookieNameUbidMain, imdbCookieNameAtMain)
	}
	return cookies, nil
}

func isListHyperlink(href string) bool {
	return strings.HasPrefix(href, "/list/ls")
}

func isRatingsHyperlink(href, userID string) bool {
	prefix := fmt.Sprintf("/user/%s/ratings", userID)
	return strings.HasPrefix(href, prefix)
}

func transformListData(data []byte) ([]entities.IMDbItem, error) {
	csvReader := csv.NewReader(bytes.NewReader(data))
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failure reading list csv records: %w", err)
	}
	var items []entities.IMDbItem
	for i, record := range csvData {
		if i > 0 {
			items = append(items, entities.IMDbItem{
				ID:        record[1],
				TitleType: record[8],
			})
		}
	}
	return items, nil
}

func transformRatingsData(data []byte) ([]entities.IMDbItem, error) {
	csvReader := csv.NewReader(bytes.NewReader(data))
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failure reading ratings csv records: %w", err)
	}
	var ratings []entities.IMDbItem
	for i, record := range csvData {
		if i > 0 {
			rating, err := strconv.Atoi(record[1])
			if err != nil {
				return nil, fmt.Errorf("failure parsing rating value to integer: %w", err)
			}
			ratingDate, err := time.Parse(time.DateOnly, record[2])
			if err != nil {
				return nil, fmt.Errorf("failure parsing rating date: %w", err)
			}
			ratings = append(ratings, entities.IMDbItem{
				ID:         record[0],
				TitleType:  record[6],
				Rating:     &rating,
				RatingDate: &ratingDate,
			})
		}
	}
	return ratings, nil
}

func (c *IMDbClient) userIdentityScrape() error {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: c.config.basePath,
		Endpoint: imdbPathProfile,
		Body:     http.NoBody,
	})
	if err != nil {
		return err
	}
	body := io.NopCloser(ReusableReader(response.Body))
	userID, err := selectorAttributeScrape(body, clientNameIMDb, ".user-profile.userId", "data-userid")
	if err != nil {
		return fmt.Errorf("imdb user id not found: %w", err)
	}
	c.config.userID = *userID
	username, err := selectorTextScrape(body, clientNameIMDb, "div.header h1")
	if err != nil {
		return fmt.Errorf("imdb username not found: %w", err)
	}
	c.config.username = *username
	return nil
}

func (c *IMDbClient) watchlistIDScrape() error {
	response, err := c.doRequest(requestFields{
		Method:   http.MethodGet,
		BasePath: c.config.basePath,
		Endpoint: imdbPathWatchlist,
		Body:     http.NoBody,
	})
	if err != nil {
		return err
	}
	href, err := selectorAttributeScrape(response.Body, clientNameIMDb, "a[data-testid='hero-list-subnav-edit-button']", "href")
	if err != nil {
		return fmt.Errorf("imdb watchlist href not found: %w", err)
	}
	watchlistID, err := lidExtract(*href)
	if err != nil {
		return fmt.Errorf("failure extracting imdb watchlist id: %w", err)
	}
	c.config.watchlistID = watchlistID
	return nil
}

func lidExtract(href string) (string, error) {
	pieces := strings.Split(href, "/")
	if len(pieces) < 3 {
		return "", fmt.Errorf("imdb list href has unexpected format: %s", href)
	}
	return pieces[2], nil
}

func (c *IMDbClient) lidsScrape() ([]string, error) {
	tab, err := c.browser.Page(proto.TargetCreateTarget{
		URL: imdbPathBase + imdbPathLists,
	})
	if err != nil {
		return nil, fmt.Errorf("failure opening browser tab: %w", err)
	}
	defer tab.MustClose()
	if err = tab.WaitLoad(); err != nil {
		return nil, fmt.Errorf("failure waiting for lists tab to load: %w", err)
	}
	listCountDiv, err := tab.Element("div[data-testid='list-page-mc-total-items']")
	if err != nil {
		return nil, fmt.Errorf("failure finding list count div: %w", err)
	}
	listCountText, err := listCountDiv.Text()
	if err != nil {
		return nil, fmt.Errorf("failure extracting list count text from div: %w", err)
	}
	listCountPieces := strings.Split(listCountText, " ")
	if len(listCountPieces) != 2 {
		return nil, fmt.Errorf("expected 2 list count text pieces, but got %d", len(listCountPieces))
	}
	listCount, err := strconv.Atoi(listCountPieces[0])
	if err != nil {
		return nil, fmt.Errorf("failure parsing list count string to integer: %w", err)
	}
	if err = c.scrollUntilAllElementsVisible(tab, "a.ipc-metadata-list-summary-item__t", listCount); err != nil {
		return nil, fmt.Errorf("failure scrolling until all list elements are visible: %w", err)
	}
	hyperlinks, err := tab.Elements("a.ipc-metadata-list-summary-item__t")
	if err != nil {
		return nil, fmt.Errorf("failure finding list resource hyperlinks: %w", err)
	}
	lids := make([]string, len(hyperlinks))
	for i, hyperlink := range hyperlinks {
		href, err := hyperlink.Attribute("href")
		if err != nil {
			return nil, fmt.Errorf("failure extracting href from hyperlink: %w", err)
		}
		lid, err := lidExtract(*href)
		if err != nil {
			return nil, fmt.Errorf("failure extracting list id from href: %w", err)
		}
		lids[i] = lid
	}
	return lids, nil
}

func (c *IMDbClient) scrollUntilAllElementsVisible(tab *rod.Page, selector string, count int) error {
	elements, err := tab.Elements(selector)
	if err != nil {
		return fmt.Errorf("failure finding elements: %w", err)
	}
	if err = elements.Last().ScrollIntoView(); err != nil {
		return fmt.Errorf("failure scrolling to the last element: %w", err)
	}
	if err = tab.WaitStable(time.Second); err != nil {
		return fmt.Errorf("failure waiting for tab to become stable: %w", err)
	}
	elements, err = tab.Elements(selector)
	if err != nil {
		return fmt.Errorf("failure finding elements: %w", err)
	}
	if len(elements) < count {
		return c.scrollUntilAllElementsVisible(tab, selector, count)
	}
	return nil
}
