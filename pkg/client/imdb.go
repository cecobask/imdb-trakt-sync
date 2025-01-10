package client

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	appconfig "github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/entities"
)

const (
	imdbPathBase           = "https://www.imdb.com"
	imdbPathExports        = "/exports"
	imdbPathList           = "/list/%s"
	imdbPathLists          = "/profile/lists"
	imdbPathRatings        = "/user/%s/ratings"
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
	userID      string
	username    string
	watchlistID string
}

func NewIMDbClient(ctx context.Context, conf *appconfig.IMDb, logger *slog.Logger) (IMDbClientInterface, error) {
	l := launcher.New().Headless(*conf.Headless).Bin(getBrowserPathOrFallback(conf)).
		Set("allow-running-insecure-content").
		Set("autoplay-policy", "user-gesture-required").
		Set("disable-component-update").
		Set("disable-domain-reliability").
		Set("disable-features", "AudioServiceOutOfProcess", "IsolateOrigins", "site-per-process").
		Set("disable-print-preview").
		Set("disable-search-engine-choice-screen").
		Set("disable-setuid-sandbox").
		Set("disable-site-isolation-trials").
		Set("disable-speech-api").
		Set("disable-web-security").
		Set("disk-cache-size", "33554432").
		Set("enable-features", "SharedArrayBuffer").
		Set("hide-scrollbars").
		Set("ignore-gpu-blocklist").
		Set("in-process-gpu").
		Set("mute-audio").
		Set("no-default-browser-check").
		Set("no-pings").
		Set("no-sandbox").
		Set("no-zygote").
		Set("single-process")
	browserURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failure launching browser: %w", err)
	}
	browser := rod.New().Context(ctx).ControlURL(browserURL).Trace(*conf.Trace)
	if err = browser.Connect(); err != nil {
		return nil, fmt.Errorf("failure connecting to browser: %w", err)
	}
	logger.Info("launched new browser instance", slog.String("url", browserURL), slog.Bool("headless", *conf.Headless), slog.Bool("trace", *conf.Trace))
	c := &IMDbClient{
		config: &imdbConfig{
			IMDb: conf,
		},
		logger:  logger,
		browser: browser,
	}
	if err = c.authenticateUser(); err != nil {
		return nil, fmt.Errorf("failure authenticating user: %w", err)
	}
	if err = c.hydrate(); err != nil {
		return nil, fmt.Errorf("failure hydrating client: %w", err)
	}
	return c, nil
}

func (c *IMDbClient) authenticateUser() error {
	if *c.config.Auth == appconfig.IMDbAuthMethodNone {
		return nil
	}
	if *c.config.Auth == appconfig.IMDbAuthMethodCookies {
		if err := setBrowserCookies(c.browser, c.config.IMDb); err != nil {
			return err
		}
		tab, err := c.navigateAndValidateResponse(imdbPathBase)
		if err != nil {
			return fmt.Errorf("failure navigating and validating response: %w", err)
		}
		authenticated, _, err := tab.Has("#nblogout")
		if err != nil {
			return fmt.Errorf("failure finding logout div")
		}
		if !authenticated {
			return fmt.Errorf("failure authenticating with the provided cookies")
		}
		return nil
	}
	tab, err := c.navigateAndValidateResponse(imdbPathBase + imdbPathSignIn)
	if err != nil {
		return fmt.Errorf("failure navigating and validating response: %w", err)
	}
	emailField, err := tab.Element("#ap_email")
	if err != nil {
		return fmt.Errorf("failure finding email field: %w", err)
	}
	if err = emailField.Input(*c.config.Email); err != nil {
		return fmt.Errorf("failure inputting value in email field: %w", err)
	}
	passwordField, err := tab.Element("#ap_password")
	if err != nil {
		return fmt.Errorf("failure finding password field: %w", err)
	}
	if err = passwordField.Input(*c.config.Password); err != nil {
		return fmt.Errorf("failure inputting value in password field: %w", err)
	}
	submitButton, err := tab.Element("#signInSubmit")
	if err != nil {
		return fmt.Errorf("failure finding submit button: %w", err)
	}
	if err = submitButton.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("failure clicking on submit button: %w", err)
	}
	race, err := tab.Race().Element("#nblogout").Element("#auth-error-message-box").Element("img[alt='captcha']").Do()
	if err != nil {
		return fmt.Errorf("failure doing selector race: %w", err)
	}
	authFailed, err := race.Matches("#auth-error-message-box")
	if err != nil {
		return fmt.Errorf("failure checking for authentication error match: %w", err)
	}
	if authFailed {
		return fmt.Errorf("failure authenticating with the provided credentials")
	}
	captcha, err := race.Matches("img[alt='captcha']")
	if err != nil {
		return fmt.Errorf("failure checking for captcha match: %w", err)
	}
	if captcha {
		return fmt.Errorf("failure authenticating as captcha prompt appeared")
	}
	return nil
}

func (c *IMDbClient) hydrate() error {
	if *c.config.Auth == appconfig.IMDbAuthMethodNone {
		return nil
	}
	tab, err := c.navigateAndValidateResponse(imdbPathBase + imdbPathWatchlist)
	if err != nil {
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
	lids := slices.DeleteFunc(*c.config.Lists, func(lid string) bool {
		if lid == watchlistID {
			c.logger.Warn("removing watchlist id from provided lists; please use config option SYNC_WATCHLIST instead")
			return true
		}
		return false
	})
	if len(lids) == 0 {
		lids, err = c.lidsScrape()
		if err != nil {
			return fmt.Errorf("failure scraping list ids: %w", err)
		}
	}
	c.config.Lists = &lids
	c.logger.Info("hydrated imdb client", slog.String("username", username), slog.String("userID", userID), slog.String("watchlistID", watchlistID), slog.Any("lists", lids))
	return nil
}

func (c *IMDbClient) WatchlistExport() error {
	if *c.config.Auth == appconfig.IMDbAuthMethodNone {
		return nil
	}
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
	for _, id := range ids {
		if err := c.ListExport(id); err != nil {
			return err
		}
	}
	return nil
}

func (c *IMDbClient) ListsGet(ids ...string) ([]entities.IMDbList, error) {
	if len(ids) == 0 {
		return make([]entities.IMDbList, 0), nil
	}
	resources, err := c.getExportedResources(ids...)
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
	if *c.config.Auth == appconfig.IMDbAuthMethodNone {
		return nil
	}
	ratingsURL := imdbPathBase + fmt.Sprintf(imdbPathRatings, c.config.userID)
	if err := c.exportResource(ratingsURL); err != nil {
		return fmt.Errorf("failure exporting ratings resource: %w", err)
	}
	c.logger.Info("exported ratings")
	return nil
}

func (c *IMDbClient) RatingsGet() ([]entities.IMDbItem, error) {
	resources, err := c.getExportedResources(c.config.userID)
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
	items, err := transformData(wait())
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
	items, err := transformData(wait())
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

func (c *IMDbClient) getExportedResources(ids ...string) (rod.Elements, error) {
	tab, err := c.navigateAndValidateResponse(imdbPathBase + imdbPathExports)
	if err != nil {
		return nil, fmt.Errorf("failure navigating and validating response: %w", err)
	}
	if err = c.waitExportsReady(tab, ids...); err != nil {
		return nil, fmt.Errorf("failure waiting for exports to become available: %w", err)
	}
	resources, err := tab.Elements("li[data-testid='user-ll-item']")
	if err != nil {
		return nil, fmt.Errorf("failure finding exported resources: %w", err)
	}
	return resources, nil
}

func (c *IMDbClient) exportResource(url string) error {
	tab, err := c.navigateAndValidateResponse(url)
	if err != nil {
		return fmt.Errorf("failure navigating and validating response: %w", err)
	}
	race, err := tab.Race().Element("div[data-testid='hero-list-subnav-export-button'] button").Element("div[data-testid='list-page-mc-private-list-content']").Do()
	if err != nil {
		return fmt.Errorf("failure doing selector race: %w", err)
	}
	resourcePrivate, err := race.Matches("div[data-testid='list-page-mc-private-list-content']")
	if err != nil {
		return fmt.Errorf("failure checking for private resource match: %w", err)
	}
	if resourcePrivate {
		return fmt.Errorf("resource at url %s is private, cannot proceed", url)
	}
	wait := tab.WaitRequestIdle(time.Second, []string{"pageAction=start-export"}, nil, nil)
	if err = race.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("failure clicking on export resource button: %w", err)
	}
	wait()
	return nil
}

func (c *IMDbClient) waitExportsReady(tab *rod.Page, ids ...string) error {
	maxRetries := 30
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt == maxRetries {
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
		duration := 30 * time.Second
		c.logger.Info(fmt.Sprintf("waiting %s before reloading exports tab to check the latest status", duration), slog.Int("attempt", attempt))
		time.Sleep(duration)
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
	tab, err := c.navigateAndValidateResponse(imdbPathBase + imdbPathLists)
	if err != nil {
		return nil, fmt.Errorf("failure navigating and validating response: %w", err)
	}
	hasLists, listCountDiv, err := tab.Has("ul[data-testid='list-page-mc-total-items'] li")
	if err != nil {
		return nil, fmt.Errorf("failure finding list count div: %w", err)
	}
	if !hasLists {
		return make([]string, 0), nil
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

func (c *IMDbClient) navigateAndValidateResponse(url string) (*rod.Page, error) {
	pages, err := c.browser.Pages()
	if err != nil {
		return nil, fmt.Errorf("failure retrieving browser pages: %w", err)
	}
	tab := pages.First()
	if pages.Empty() {
		tab = c.browser.MustPage()
	}
	if err = tab.Navigate(url); err != nil {
		return nil, fmt.Errorf("failure navigating to url %s: %w", url, err)
	}
	if err = tab.WaitLoad(); err != nil {
		return nil, fmt.Errorf("failure waiting for tab %s to load: %w", url, err)
	}
	return tab, nil
}

func isListHyperlink(href string) bool {
	return strings.HasPrefix(href, "/list/ls")
}

func isRatingsHyperlink(href, userID string) bool {
	prefix := fmt.Sprintf("/user/%s/ratings", userID)
	return strings.HasPrefix(href, prefix)
}

func isPeopleList(header []string) bool {
	return slices.Equal(header, []string{
		"Position",
		"Const",
		"Created",
		"Modified",
		"Description",
		"Name",
		"Known For",
		"Birth Date",
	})
}

func isTitlesList(header []string) bool {
	return slices.Equal(header, []string{
		"Position",
		"Const",
		"Created",
		"Modified",
		"Description",
		"Title",
		"Original Title",
		"URL",
		"Title Type",
		"IMDb Rating",
		"Runtime (mins)",
		"Year",
		"Genres",
		"Num Votes",
		"Release Date",
		"Directors",
		"Your Rating",
		"Date Rated",
	})
}

func isRatingsList(header []string) bool {
	return slices.Equal(header, []string{
		"Const",
		"Your Rating",
		"Date Rated",
		"Title",
		"Original Title",
		"URL",
		"Title Type",
		"IMDb Rating",
		"Runtime (mins)",
		"Year",
		"Genres",
		"Num Votes",
		"Release Date",
		"Directors",
	})
}

func transformData(data []byte) ([]entities.IMDbItem, error) {
	csvReader := csv.NewReader(bytes.NewReader(data))
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1
	csvData, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failure reading csv records: %w", err)
	}
	if len(csvData) == 0 {
		return nil, fmt.Errorf("expected csv records to have at least header row, but got empty result")
	}
	var (
		header  = csvData[0]
		records = csvData[1:]
		items   = make([]entities.IMDbItem, len(records))
	)
	if isTitlesList(header) {
		for i, record := range records {
			items[i] = entities.IMDbItem{
				ID:   record[1],
				Kind: record[8],
			}
		}
		return items, nil
	}
	if isRatingsList(header) {
		for i, record := range records {
			rating, err := strconv.Atoi(record[1])
			if err != nil {
				return nil, fmt.Errorf("failure parsing rating value to integer: %w", err)
			}
			ratingDate, err := time.Parse(time.DateOnly, record[2])
			if err != nil {
				return nil, fmt.Errorf("failure parsing rating date: %w", err)
			}
			items[i] = entities.IMDbItem{
				ID:         record[0],
				Kind:       record[6],
				Rating:     &rating,
				RatingDate: &ratingDate,
			}
		}
		return items, nil
	}
	if isPeopleList(header) {
		for i, record := range records {
			items[i] = entities.IMDbItem{
				ID:   record[1],
				Kind: "Person",
			}
		}
		return items, nil
	}
	return nil, fmt.Errorf("unrecognized list type with header %s", header)
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

func getBrowserPathOrFallback(conf *appconfig.IMDb) string {
	if browserPath := conf.BrowserPath; *browserPath != "" {
		return *browserPath
	}
	if browserPath, found := launcher.LookPath(); found {
		return browserPath
	}
	return ""
}
