package main

import (
	"errors"
	_ "github.com/joho/godotenv/autoload"
	"os"
	"regexp"
	"strings"
)

type listPair struct {
	imdbItems   []imdbListItem
	imdbListId  string
	listName    string
	traktItems  []traktListItem
	traktListId string
}

type user struct {
	imdbUserId  string
	traktUserId string
	watchlist   listPair
	customLists []listPair
}

func sync() {
	validateEnvVars()
	imdbClient := newImdbClient()
	traktClient := newTraktClient()
	imdbUserId, imdbWatchlistId := os.Getenv(imdbUserIdKey), os.Getenv(imdbWatchlistIdKey)
	u := user{
		imdbUserId:  imdbUserId,
		traktUserId: os.Getenv(traktUserIdKey),
	}

	// sync imdb watchlist with trakt watchlist
	_, listItems := imdbClient.listItemsGet(imdbWatchlistId)
	u.watchlist = listPair{
		imdbListId: imdbWatchlistId,
		imdbItems:  listItems,
	}
	u.watchlist.traktItems = traktClient.watchlistItemsGet()
	diff := u.watchlist.difference()
	traktClient.watchlistItemsAdd(diff["add"])
	traktClient.watchlistItemsRemove(diff["remove"])

	// sync imdb lists with trakt custom lists
	imdbListIdsString := os.Getenv(imdbCustomListIdsKey)
	if len(imdbListIdsString) > 0 {
		imdbListIds := strings.Split(imdbListIdsString, ",")
		for _, listId := range imdbListIds {
			u.customLists = append(u.customLists, listPair{
				imdbListId: listId,
			})
		}
	}
	u.customLists = cleanupCustomLists(u.customLists, imdbClient)
	for _, cl := range u.customLists {
		traktItems, err := traktClient.customListItemsGet(cl.traktListId)
		if errors.Is(err, errNotFound) {
			traktClient.customListAdd(cl.traktListId, cl.listName)
		}
		cl.traktItems = traktItems
		diff := cl.difference()
		traktClient.customListItemsAdd(cl.traktListId, diff["add"])
		traktClient.customListItemsRemove(cl.traktListId, diff["remove"])
	}
}

// cleanupCustomLists removes invalid imdb lists passed via the IMDB_CUSTOM_LIST_IDS
// env variable and returns only the lists that actually exist
func cleanupCustomLists(customLists []listPair, imdbClient *imdbClient) []listPair {
	n := 0
	for _, customList := range customLists {
		listName, listItems := imdbClient.listItemsGet(customList.imdbListId)
		if listName != "" {
			customLists[n] = listPair{
				imdbListId:  customList.imdbListId,
				imdbItems:   listItems,
				listName:    listName,
				traktListId: formatTraktListName(listName),
			}
			n++
		}
	}
	return customLists[:n]
}

func formatTraktListName(listName string) string {
	formatted := strings.ToLower(strings.Join(strings.Fields(listName), "-"))
	re := regexp.MustCompile(`[^-a-z0-9]+`)
	return re.ReplaceAllString(formatted, "")
}

func (lp *listPair) difference() map[string][]imdbListItem {
	diff := make(map[string][]imdbListItem)
	// add items missing to the trakt list
	temp := make(map[string]struct{}, len(lp.traktItems))
	for _, x := range lp.traktItems {
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
	for _, x := range lp.imdbItems {
		if _, found := temp[x.ID]; !found {
			diff["add"] = append(diff["add"], x)
		}
	}
	// remove out of sync items from the trakt list
	temp = make(map[string]struct{}, len(lp.imdbItems))
	for _, x := range lp.imdbItems {
		temp[x.ID] = struct{}{}
	}
	for _, x := range lp.traktItems {
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
			diff["remove"] = append(diff["remove"], imdbListItem{
				ID:   imdbId,
				Type: imdbType,
			})
		}
	}
	return diff
}
