package imdb

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

	"github.com/cecobask/imdb-trakt-sync/internal/config"
)

type API interface {
	ListsExport(ids ...string) error
	ListsGet(ids ...string) (Lists, error)
	WatchlistExport() error
	WatchlistGet() (*List, error)
	RatingsExport() error
	RatingsGet() (Items, error)
}

const (
	pathBase      = "https://imdb.com"
	pathExports   = "/exports"
	pathList      = "/list/%s"
	pathLists     = "/profile/lists"
	pathRatings   = "/user/%s/ratings"
	pathSignIn    = "/registration/ap-signin-handler/imdb_us"
	pathWatchlist = "/list/watchlist"

	cookieNameAtMain   = "at-main"
	cookieNameUbidMain = "ubid-main"
	cookieNameDomain   = ".imdb.com"
)

type client struct {
	*config.IMDb
	baseURL     string
	browser     *rod.Browser
	logger      *slog.Logger
	userID      string
	watchlistID string
}

func NewAPI(ctx context.Context, conf *config.IMDb, logger *slog.Logger) (API, error) {
	l := launcher.New().Headless(*conf.Headless).Bin(getBrowserPathOrFallback(*conf.BrowserPath)).
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
	logger.Info("launched new browser instance",
		slog.String("url", browserURL),
		slog.Bool("headless", *conf.Headless),
		slog.Bool("trace", *conf.Trace),
		slog.String("path", *conf.BrowserPath),
	)
	c := &client{
		baseURL: pathBase,
		IMDb:    conf,
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

func (c *client) authenticateUser() error {
	if *c.Auth == config.IMDbAuthMethodNone {
		return nil
	}
	if *c.Auth == config.IMDbAuthMethodCookies {
		if err := setBrowserCookies(c.browser, *c.CookieAtMain); err != nil {
			return err
		}
		tab, err := c.navigateAndValidateResponse(c.baseURL)
		if err != nil {
			return fmt.Errorf("failure navigating and validating response: %w", err)
		}
		authenticated, _, err := tab.Has("#navUserMenu")
		if err != nil {
			return fmt.Errorf("failure finding logout div")
		}
		if !authenticated {
			return fmt.Errorf("failure authenticating with the provided cookies")
		}
		return nil
	}
	tab, err := c.navigateAndValidateResponse(c.baseURL + pathSignIn)
	if err != nil {
		return fmt.Errorf("failure navigating and validating response: %w", err)
	}
	emailField, err := tab.Element("#ap_email")
	if err != nil {
		return fmt.Errorf("failure finding email field: %w", err)
	}
	if err = emailField.Input(*c.Email); err != nil {
		return fmt.Errorf("failure inputting value in email field: %w", err)
	}
	passwordField, err := tab.Element("#ap_password")
	if err != nil {
		return fmt.Errorf("failure finding password field: %w", err)
	}
	if err = passwordField.Input(*c.Password); err != nil {
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

func (c *client) hydrate() error {
	if *c.Auth == config.IMDbAuthMethodNone {
		return nil
	}
	tab, err := c.navigateAndValidateResponse(c.baseURL + pathWatchlist)
	if err != nil {
		return fmt.Errorf("failure navigating and validating response: %w", err)
	}
	hyperlink, err := tab.Element("a[data-testid='list-author-link']")
	if err != nil {
		return fmt.Errorf("failure finding hyperlink element: %w", err)
	}
	href, err := hyperlink.Attribute("href")
	if err != nil {
		return fmt.Errorf("failure extracting href from hyperlink: %w", err)
	}
	userID, err := idExtract(*href)
	if err != nil {
		return fmt.Errorf("failure extracting user id from href: %w", err)
	}
	c.userID = userID
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
	c.watchlistID = watchlistID
	lids := slices.DeleteFunc(*c.Lists, func(lid string) bool {
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
	c.Lists = &lids
	c.logger.Info("hydrated imdb client", slog.String("userID", userID), slog.String("watchlistID", watchlistID), slog.Any("lists", lids))
	return nil
}

func (c *client) WatchlistExport() error {
	if *c.Auth == config.IMDbAuthMethodNone {
		return nil
	}
	return c.ListExport(c.watchlistID)
}

func (c *client) WatchlistGet() (*List, error) {
	lists, err := c.ListsGet(c.watchlistID)
	if err != nil {
		return nil, fmt.Errorf("failure downloading watchlist: %w", err)
	}
	return &lists[0], nil
}

func (c *client) ListExport(id string) error {
	listURL := c.baseURL + fmt.Sprintf(pathList, id)
	if err := c.exportResource(listURL); err != nil {
		return fmt.Errorf("failure exporting list %s: %w", id, err)
	}
	c.logger.Info("exported list", slog.String("id", id))
	return nil
}

func (c *client) ListsExport(ids ...string) error {
	for _, id := range ids {
		if err := c.ListExport(id); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) ListsGet(ids ...string) (Lists, error) {
	if len(ids) == 0 {
		return make(Lists, 0), nil
	}
	resources, err := c.getExportedResources(ids...)
	if err != nil {
		return nil, fmt.Errorf("failure fetching exported resources: %w", err)
	}
	filteredResources, err := c.filterResources(resources, ids...)
	if err != nil {
		return nil, fmt.Errorf("failure filtering resources: %w", err)
	}
	lists := make(Lists, len(ids))
	for i, listResource := range filteredResources {
		list, err := c.listDownload(listResource)
		if err != nil {
			return nil, fmt.Errorf("failure downloading list: %w", err)
		}
		lists[i] = *list
	}
	return lists, nil
}

func (c *client) RatingsExport() error {
	if *c.Auth == config.IMDbAuthMethodNone {
		return nil
	}
	ratingsURL := c.baseURL + fmt.Sprintf(pathRatings, c.userID)
	if err := c.exportResource(ratingsURL); err != nil {
		return fmt.Errorf("failure exporting ratings resource: %w", err)
	}
	c.logger.Info("exported ratings")
	return nil
}

func (c *client) RatingsGet() (Items, error) {
	resources, err := c.getExportedResources(c.userID)
	if err != nil {
		return nil, fmt.Errorf("failure fetching exported resources: %w", err)
	}
	filteredResources, err := c.filterResources(resources, c.userID)
	if err != nil {
		return nil, fmt.Errorf("failure filtering resources: %w", err)
	}
	return c.ratingsDownload(filteredResources[0])
}

func (c *client) ratingsDownload(resource *rod.Element) (Items, error) {
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

func (c *client) listDownload(resource *rod.Element) (*List, error) {
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
	return &List{
		ListID:      lid,
		ListName:    listName,
		ListItems:   items,
		IsWatchlist: lid == c.watchlistID,
	}, nil
}

func (c *client) getExportedResources(ids ...string) (rod.Elements, error) {
	tab, err := c.navigateAndValidateResponse(c.baseURL + pathExports)
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

func (c *client) exportResource(url string) error {
	tab, err := c.navigateAndValidateResponse(url)
	if err != nil {
		return fmt.Errorf("failure navigating and validating response: %w", err)
	}
	race, err := tab.Race().
		Element("div[data-testid='hero-list-subnav-export-button'] button").
		Element("div[data-testid='list-page-mc-private-list-content']").
		Element("h1[data-testid='error-page-title']").
		Do()
	if err != nil {
		return fmt.Errorf("failure doing selector race: %w", err)
	}
	isPrivate, err := race.Matches("div[data-testid='list-page-mc-private-list-content']")
	if err != nil {
		return fmt.Errorf("failure checking for private resource match: %w", err)
	}
	if isPrivate {
		return fmt.Errorf("resource at url %s is private, cannot proceed", url)
	}
	isMissing, err := race.Matches("h1[data-testid='error-page-title']")
	if err != nil {
		return fmt.Errorf("failure checking for missing resource match: %w", err)
	}
	if isMissing {
		return fmt.Errorf("resource at url %s is missing, cannot proceed", url)
	}
	wait := tab.WaitRequestIdle(time.Second, []string{"pageAction=start-export"}, nil, nil)
	if err = race.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("failure clicking on export resource button: %w", err)
	}
	wait()
	return nil
}

func (c *client) waitExportsReady(tab *rod.Page, ids ...string) error {
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
		c.logger.Info(
			"resources are still processing, reloading exports tab",
			slog.Int("attempt", attempt),
			slog.String("backoff", duration.String()),
		)
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

func (c *client) filterResources(resources rod.Elements, ids ...string) (rod.Elements, error) {
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
		if isRatingsHyperlink(*href, c.userID) {
			return rod.Elements{r}, nil
		}
	}
	if len(filteredResources) != len(ids) {
		return nil, fmt.Errorf("expected the number of filtered resources to be %d, but got %d", len(ids), len(filteredResources))
	}
	return filteredResources, nil
}

func (c *client) lidsScrape() ([]string, error) {
	tab, err := c.navigateAndValidateResponse(c.baseURL + pathLists)
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

func (c *client) scrollUntilAllElementsVisible(tab *rod.Page, selector string, count int) error {
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

func (c *client) navigateAndValidateResponse(url string) (*rod.Page, error) {
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

func transformData(data []byte) (Items, error) {
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
		items   = make(Items, len(records))
	)
	if isTitlesList(header) {
		for i, record := range records {
			created, err := time.Parse(time.DateOnly, record[2])
			if err != nil {
				return nil, fmt.Errorf("failure parsing created date: %w", err)
			}
			items[i] = Item{
				ID:      record[1],
				Kind:    record[8],
				Created: created,
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
			created, err := time.Parse(time.DateOnly, record[2])
			if err != nil {
				return nil, fmt.Errorf("failure parsing created date: %w", err)
			}
			items[i] = Item{
				ID:      record[0],
				Kind:    record[6],
				Created: created,
				Rating:  &rating,
			}
		}
		return items, nil
	}
	if isPeopleList(header) {
		for i, record := range records {
			created, err := time.Parse(time.DateOnly, record[2])
			if err != nil {
				return nil, fmt.Errorf("failure parsing created date: %w", err)
			}
			items[i] = Item{
				ID:      record[1],
				Kind:    "Person",
				Created: created,
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

func setBrowserCookies(browser *rod.Browser, cookieAtMain string) error {
	cookies := []*proto.NetworkCookieParam{
		{
			Name:   cookieNameAtMain,
			Value:  cookieAtMain,
			Domain: cookieNameDomain,
		},
		{
			Name:   cookieNameUbidMain,
			Value:  "dummy", // value does not matter
			Domain: cookieNameDomain,
		},
	}
	if err := browser.SetCookies(cookies); err != nil {
		return fmt.Errorf("failure setting browser cookies: %w", err)
	}
	return nil
}

func getBrowserPathOrFallback(path string) string {
	if path != "" {
		return path
	}
	if browserPath, found := launcher.LookPath(); found {
		return browserPath
	}
	return ""
}
