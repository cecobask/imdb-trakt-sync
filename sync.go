package main

import (
	"errors"
	_ "github.com/joho/godotenv/autoload"
	"os"
	"regexp"
	"strings"
)

type user struct {
	lists   []dataPair
	ratings dataPair
}

type dataPair struct {
	imdbList     []imdbItem
	imdbListId   string
	imdbListName string
	traktList    []traktItem
	traktListId  string
	isWatchlist  bool
}

func sync() {
	ic := newImdbClient()
	tc := newTraktClient()
	u := &user{}
	u.populateData(ic, tc)
	u.syncLists(tc)
	u.syncRatings(tc)
}

func (u *user) populateData(ic *imdbClient, tc *traktClient) {
	ic.config.imdbUserId = ic.userIdScrape()
	ic.config.imdbWatchlistId = ic.watchlistIdScrape()
	tc.config.traktUserId = tc.userIdGet()
	imdbListIdsString := os.Getenv(imdbListIdsKey)
	switch imdbListIdsString {
	case "all":
		u.lists = ic.listsScrape()
	default:
		imdbListIds := strings.Split(imdbListIdsString, ",")
		u.lists = cleanupLists(ic, imdbListIds)
	}
	_, imdbList, _ := ic.listItemsGet(ic.config.imdbWatchlistId)
	u.lists = append(u.lists, dataPair{
		imdbList:     imdbList,
		imdbListId:   ic.config.imdbWatchlistId,
		imdbListName: "watchlist",
		isWatchlist:  true,
	})
	for i := range u.lists {
		list := &u.lists[i]
		if list.isWatchlist {
			list.traktList = tc.watchlistItemsGet()
			continue
		}
		traktList, err := tc.listItemsGet(list.traktListId)
		if errors.Is(err, errNotFound) {
			tc.listAdd(list.traktListId, list.imdbListName)
		}
		list.traktList = traktList
	}
	u.ratings = dataPair{
		imdbList:  ic.ratingsGet(),
		traktList: tc.ratingsGet(),
	}
}

func (u *user) syncLists(tc *traktClient) {
	for _, list := range u.lists {
		diff := list.difference()
		if len(diff["add"]) > 0 {
			if list.isWatchlist {
				tc.watchlistItemsAdd(diff["add"])
				continue
			}
			tc.listItemsAdd(list.traktListId, diff["add"])
		}
		if len(diff["remove"]) > 0 {
			if list.isWatchlist {
				tc.watchlistItemsRemove(diff["remove"])
				continue
			}
			tc.listItemsRemove(list.traktListId, diff["remove"])
		}
	}
}

func (u *user) syncRatings(tc *traktClient) {
	diff := u.ratings.difference()
	if len(diff["add"]) > 0 {
		tc.ratingsAdd(diff["add"])
	}
	if len(diff["remove"]) > 0 {
		tc.ratingsRemove(diff["remove"])
	}
}

// cleanupLists removes invalid imdb lists passed via the IMDB_LIST_IDS
// env variable and returns only the lists that actually exist
func cleanupLists(ic *imdbClient, imdbListIds []string) []dataPair {
	lists := make([]dataPair, len(imdbListIds))
	n := 0
	for _, imdbListId := range imdbListIds {
		imdbListName, imdbList, err := ic.listItemsGet(imdbListId)
		if errors.Is(err, errNotFound) {
			continue
		}
		lists[n] = dataPair{
			imdbList:     imdbList,
			imdbListId:   imdbListId,
			imdbListName: *imdbListName,
			traktListId:  formatTraktListName(*imdbListName),
		}
		n++
	}
	return lists[:n]
}

func formatTraktListName(imdbListName string) string {
	formatted := strings.ToLower(strings.Join(strings.Fields(imdbListName), "-"))
	re := regexp.MustCompile(`[^-a-z0-9]+`)
	return re.ReplaceAllString(formatted, "")
}

func (dp *dataPair) difference() map[string][]imdbItem {
	diff := make(map[string][]imdbItem)
	// add missing items to trakt
	temp := make(map[string]struct{}, len(dp.traktList))
	for _, x := range dp.traktList {
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
	for _, x := range dp.imdbList {
		if _, found := temp[x.id]; !found {
			diff["add"] = append(diff["add"], x)
		}
	}
	// remove out of sync items from trakt
	temp = make(map[string]struct{}, len(dp.imdbList))
	for _, x := range dp.imdbList {
		temp[x.id] = struct{}{}
	}
	for _, x := range dp.traktList {
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
