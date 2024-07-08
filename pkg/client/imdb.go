package client

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"golang.org/x/sync/errgroup"

	appconfig "github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/entities"
)

const (
	imdbPathBase           = "https://www.imdb.com"
	imdbPathExports        = "/exports"
	imdbPathList           = "/list/%s"
	imdbPathLists          = "/profile/lists"
	imdbPathRatings        = "/list/ratings"
	imdbPathSignIn         = "/registration/ap-signin-handler/imdb_us"
	imdbPathWatchlist      = "/list/watchlist"
	imdbCookieNameAtMain   = "at-main"
	imdbCookieNameUbidMain = "ubid-main"
	imdbCookieDomain       = ".imdb.com"
)

type IMDbClient struct {
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
	browserPath, ok := launcher.LookPath()
	if !ok {
		return nil, fmt.Errorf("failure looking up browser path")
	}
	browserURL, err := launcher.New().Bin(browserPath).Headless(*conf.Headless).Launch()
	if err != nil {
		return nil, fmt.Errorf("failure launching browser: %w", err)
	}
	browser := rod.New().Context(ctx).ControlURL(browserURL).Trace(*conf.Trace)
	if err = browser.Connect(); err != nil {
		return nil, fmt.Errorf("failure connecting to browser: %w", err)
	}
	logger.Info("launched new browser instance", slog.String("url", browserURL), slog.Bool("headless", *conf.Headless), slog.Bool("trace", *conf.Trace))
	if err = authenticateUser(browser, config.IMDb); err != nil {
		return nil, fmt.Errorf("failure authenticating user: %w", err)
	}
	c := &IMDbClient{
		config:  &config,
		logger:  logger,
		browser: browser,
	}
	if err = c.hydrate(); err != nil {
		return nil, fmt.Errorf("failure hydrating client: %w", err)
	}
	return c, nil
}

func authenticateUser(browser *rod.Browser, config *appconfig.IMDb) error {
	if *config.Auth == appconfig.IMDbAuthMethodCookies {
		return setBrowserCookies(browser, config)
	}
	tab, err := stealth.Page(browser)
	if err != nil {
		return fmt.Errorf("failure opening browser tab: %w", err)
	}
	defer tab.MustClose()
	if tab, err = navigateAndValidateResponse(tab, imdbPathBase+imdbPathSignIn); err != nil {
		return fmt.Errorf("failure navigating and validating response: %w", err)
	}
	emailField, err := tab.Element("#ap_email")
	if err != nil {
		return fmt.Errorf("failure finding email field: %w", err)
	}
	if err = emailField.Input(*config.Email); err != nil {
		return fmt.Errorf("failure inputting value in email field: %w", err)
	}
	passwordField, err := tab.Element("#ap_password")
	if err != nil {
		return fmt.Errorf("failure finding password field: %w", err)
	}
	if err = passwordField.Input(*config.Password); err != nil {
		return fmt.Errorf("failure inputting value in password field: %w", err)
	}
	submitButton, err := tab.Element("#signInSubmit")
	if err != nil {
		return fmt.Errorf("failure finding submit button: %w", err)
	}
	if err = submitButton.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("failure clicking on submit button: %w", err)
	}
	authResult, err := tab.Timeout(time.Minute).Race().Element("#nblogout").Element("#auth-error-message-box").Element("img[alt='captcha']").Do()
	if err != nil {
		return fmt.Errorf("failure doing selector race: %w", err)
	}
	authFailed, err := authResult.Matches("#auth-error-message-box")
	if err != nil {
		return fmt.Errorf("failure checking for authentication error match: %w", err)
	}
	if authFailed {
		return fmt.Errorf("failure authenticating with the provided credentials")
	}
	captcha, err := authResult.Matches("img[alt='captcha']")
	if err != nil {
		return fmt.Errorf("failure checking for captcha match: %w", err)
	}
	if captcha {
		return fmt.Errorf("failure authenticating as captcha prompt appeared")
	}
	return nil
}

func setBrowserCookies(browser *rod.Browser, config *appconfig.IMDb) error {
	cookies := []*proto.NetworkCookieParam{
		{
			Name:   imdbCookieNameAtMain,
			Value:  *config.CookieAtMain,
			Domain: imdbCookieDomain,
		},
		{
			Name:   imdbCookieNameUbidMain,
			Value:  *config.CookieUbidMain,
			Domain: imdbCookieDomain,
		},
	}
	if err := browser.SetCookies(cookies); err != nil {
		return fmt.Errorf("failure setting browser cookies: %w", err)
	}
	return nil
}

func (c *IMDbClient) hydrate() error {
	tab, err := stealth.Page(c.browser)
	if err != nil {
		return fmt.Errorf("failure opening browser tab: %w", err)
	}
	defer tab.MustClose()
	if tab, err = navigateAndValidateResponse(tab, imdbPathBase+imdbPathWatchlist); err != nil {
		return fmt.Errorf("failure navigating and validating response: %w", err)
	}
	hyperlink, err := tab.Element("a[data-testid='list-author-link']")
	if err != nil {
		return fmt.Errorf("failure finding hyperlink element: %w", err)
	}
	username, err := hyperlink.Text()
	if err != nil {
		return fmt.Errorf("failure extracting username from hyperlink: %w", err)
	}
	c.config.username = username
	href, err := hyperlink.Attribute("href")
	if err != nil {
		return fmt.Errorf("failure extracting href from hyperlink: %w", err)
	}
	userID, err := idExtract(*href)
	if err != nil {
		return fmt.Errorf("failure extracting user id from href: %w", err)
	}
	c.config.userID = userID
	hyperlink, err = tab.Element("a[data-testid='hero-list-subnav-edit-button']")
	if err != nil {
		return fmt.Errorf("failure finding hyperlink element: %w", err)
	}
	href, err = hyperlink.Attribute("href")
	if err != nil {
		return fmt.Errorf("failure extracting href from hyperlink: %w", err)
	}
	watchlistID, err := idExtract(*href)
	if err != nil {
		return fmt.Errorf("failure extracting watchlist id from href: %w", err)
	}
	c.config.watchlistID = watchlistID
	if len(*c.config.Lists) == 0 {
		lids, err := c.lidsScrape()
		if err != nil {
			return fmt.Errorf("failure scraping list ids: %w", err)
		}
		c.config.Lists = &lids
	}
	c.logger.Info("hydrated imdb client", slog.String("username", username), slog.String("userID", userID), slog.String("watchlistID", watchlistID), slog.Any("lists", c.config.Lists))
	return nil
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
		return fmt.Errorf("failure exporting list %s: %w", id, err)
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
	defer cleanupFunc()
	if err != nil {
		return nil, fmt.Errorf("failure fetching exported resources: %w", err)
	}
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
	defer cleanupFunc()
	if err != nil {
		return nil, fmt.Errorf("failure fetching exported resources: %w", err)
	}
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
	lid, err := idExtract(*href)
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
	tab, err := stealth.Page(c.browser)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failure opening browser tab: %w", err)
	}
	cleanupFunc := func() {
		tab.MustClose()
	}
	if tab, err = navigateAndValidateResponse(tab, imdbPathBase+imdbPathExports); err != nil {
		return nil, cleanupFunc, fmt.Errorf("failure navigating and validating response: %w", err)
	}
	if err = c.waitExportsReady(tab, ids...); err != nil {
		return nil, cleanupFunc, fmt.Errorf("failure waiting for exports to become available: %w", err)
	}
	resources, err := tab.Elements("li[data-testid='user-ll-item']")
	if err != nil {
		return nil, cleanupFunc, fmt.Errorf("failure finding exported resources: %w", err)
	}
	return resources, cleanupFunc, nil
}

func (c *IMDbClient) exportResource(url string) error {
	tab, err := stealth.Page(c.browser)
	if err != nil {
		return fmt.Errorf("failure opening browser tab: %w", err)
	}
	defer tab.MustClose()
	if tab, err = navigateAndValidateResponse(tab, url); err != nil {
		return fmt.Errorf("failure navigating and validating response: %w", err)
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
			lid, err := idExtract(*href)
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

func (c *IMDbClient) lidsScrape() ([]string, error) {
	tab, err := stealth.Page(c.browser)
	if err != nil {
		return nil, fmt.Errorf("failure opening browser tab: %w", err)
	}
	defer tab.MustClose()
	if tab, err = navigateAndValidateResponse(tab, imdbPathBase+imdbPathLists); err != nil {
		return nil, fmt.Errorf("failure navigating and validating response: %w", err)
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
		lid, err := idExtract(*href)
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

func idExtract(href string) (string, error) {
	pieces := strings.Split(href, "/")
	if len(pieces) < 3 {
		return "", fmt.Errorf("hyperlink href has unexpected format: %s", href)
	}
	return pieces[2], nil
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

func navigateAndValidateResponse(tab *rod.Page, url string) (*rod.Page, error) {
	var event proto.NetworkResponseReceived
	wait := tab.WaitEvent(&event)
	if err := tab.Navigate(url); err != nil {
		return nil, fmt.Errorf("failure navigating to url: %w", err)
	}
	wait()
	if status := event.Response.Status; status != http.StatusOK {
		return nil, fmt.Errorf("navigating to %s produced %d status", url, status)
	}
	if err := tab.WaitStable(time.Second); err != nil {
		return nil, fmt.Errorf("failure waiting for tab to load: %w", err)
	}
	return tab, nil
}
