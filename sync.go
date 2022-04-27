package main

import (
	"errors"
	_ "github.com/joho/godotenv/autoload"
	"os"
	"regexp"
	"strings"
)

type listInfo struct {
	imdbItems    []imdbItem
	imdbListId   string
	imdbListName string
	traktListId  string
}

func sync() {
	ic := newImdbClient()
	tc := newTraktClient()
	syncWatchlist(ic, tc)
	syncLists(ic, tc)
	syncRatings(ic, tc)
}

func syncWatchlist(ic *imdbClient, tc *traktClient) {
	_, imdbItems, err := ic.listItemsGet(ic.config.imdbWatchlistId)
	if errors.Is(err, errNotFound) {
		return
	}
	traktItems := tc.watchlistItemsGet()
	diff := difference(imdbItems, traktItems)
	if len(diff["add"]) > 0 {
		tc.watchlistItemsAdd(diff["add"])
	}
	if len(diff["remove"]) > 0 {
		tc.watchlistItemsRemove(diff["remove"])
	}
}

func syncLists(ic *imdbClient, tc *traktClient) {
	var listInfos []listInfo
	imdbListIdsString := os.Getenv(imdbListIdsKey)
	switch imdbListIdsString {
	case "all":
		listInfos = ic.listsScrape()
	default:
		imdbListIds := strings.Split(imdbListIdsString, ",")
		listInfos = cleanupLists(ic, imdbListIds)
	}
	for _, li := range listInfos {
		traktItems, err := tc.listItemsGet(li.traktListId)
		if errors.Is(err, errNotFound) {
			tc.listAdd(li.traktListId, li.imdbListName)
		}
		diff := difference(li.imdbItems, traktItems)
		if len(diff["add"]) > 0 {
			tc.listItemsAdd(li.traktListId, diff["add"])
		}
		if len(diff["remove"]) > 0 {
			tc.listItemsRemove(li.traktListId, diff["remove"])
		}
	}
}

func syncRatings(ic *imdbClient, tc *traktClient) {
	imdbItems := ic.ratingsGet()
	traktItems := tc.ratingsGet()
	diff := difference(imdbItems, traktItems)
	if len(diff["add"]) > 0 {
		tc.ratingsAdd(diff["add"])
	}
	if len(diff["remove"]) > 0 {
		tc.ratingsRemove(diff["remove"])
	}
}

// cleanupLists removes invalid imdb lists passed via the IMDB_LIST_IDS
// env variable and returns only the lists that actually exist
func cleanupLists(ic *imdbClient, imdbListIds []string) []listInfo {
	li := make([]listInfo, len(imdbListIds))
	n := 0
	for _, imdbListId := range imdbListIds {
		imdbListName, imdbItems, err := ic.listItemsGet(imdbListId)
		if errors.Is(err, errNotFound) {
			continue
		}
		li[n] = listInfo{
			imdbItems:    imdbItems,
			imdbListId:   imdbListId,
			imdbListName: *imdbListName,
			traktListId:  formatTraktListName(*imdbListName),
		}
		n++
	}
	return li[:n]
}

func formatTraktListName(imdbListName string) string {
	formatted := strings.ToLower(strings.Join(strings.Fields(imdbListName), "-"))
	re := regexp.MustCompile(`[^-a-z0-9]+`)
	return re.ReplaceAllString(formatted, "")
}

func difference(imdbItems []imdbItem, traktItems []traktItem) map[string][]imdbItem {
	diff := make(map[string][]imdbItem)
	// add items missing to trakt
	temp := make(map[string]struct{}, len(traktItems))
	for _, x := range traktItems {
		switch x.Type {
		case "movie":
			temp[x.Movie.Ids.Imdb] = struct{}{}
		case "show":
			temp[x.Show.Ids.Imdb] = struct{}{}
		case "episode":
			temp[x.Episode.Ids.Imdb] = struct{}{}
		default:
			continue
		}
	}
	for _, x := range imdbItems {
		if _, found := temp[x.id]; !found {
			diff["add"] = append(diff["add"], x)
		}
	}
	// remove out of sync items from trakt
	temp = make(map[string]struct{}, len(imdbItems))
	for _, x := range imdbItems {
		temp[x.id] = struct{}{}
	}
	for _, x := range traktItems {
		var imdbId string
		var imdbType string
		switch x.Type {
		case "movie":
			imdbId = x.Movie.Ids.Imdb
			imdbType = "movie"
		case "show":
			imdbId = x.Show.Ids.Imdb
			imdbType = "tvSeries"
		case "episode":
			imdbId = x.Episode.Ids.Imdb
			imdbType = "tvEpisode"
		default:
			continue
		}
		if _, found := temp[imdbId]; !found {
			diff["remove"] = append(diff["remove"], imdbItem{
				id:        imdbId,
				titleType: imdbType,
			})
		}
	}
	return diff
}
