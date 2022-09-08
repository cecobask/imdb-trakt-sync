package providers

import (
	"errors"
	"github.com/cecobask/imdb-trakt-sync/pkg/client"
	"github.com/cecobask/imdb-trakt-sync/pkg/providers/imdb"
	"github.com/cecobask/imdb-trakt-sync/pkg/providers/trakt"
	"os"
	"strings"
)

type user struct {
	lists   []imdb.DataPair
	ratings imdb.DataPair
}

func Sync() {
	ic := imdb.NewClient()
	tc := trakt.NewClient()
	u := &user{}
	u.populateData(ic, tc)
	u.syncLists(tc)
	u.syncRatings(tc)
}

func (u *user) populateData(ic *imdb.Client, tc *trakt.Client) {
	ic.Config.UserId = os.Getenv("IMDB_USER_ID")
	if ic.Config.UserId == "" || ic.Config.UserId == "scrape" {
		ic.Config.UserId = ic.UserIdScrape()
	}
	ic.Config.WatchlistId = ic.WatchlistIdScrape()
	tc.Config.UserId = tc.UserIdGet()
	imdbListIdsString := os.Getenv(imdb.ListIdsKey)
	switch imdbListIdsString {
	case "all":
		u.lists = ic.ListsScrape()
	default:
		imdbListIds := strings.Split(imdbListIdsString, ",")
		u.lists = cleanupLists(ic, imdbListIds)
	}
	_, imdbList, _ := ic.ListItemsGet(ic.Config.WatchlistId)
	u.lists = append(u.lists, imdb.DataPair{
		ImdbList:     imdbList,
		ImdbListId:   ic.Config.WatchlistId,
		ImdbListName: "watchlist",
		IsWatchlist:  true,
	})
	for i := range u.lists {
		list := &u.lists[i]
		if list.IsWatchlist {
			list.TraktList = tc.WatchlistItemsGet()
			continue
		}
		traktList, err := tc.ListItemsGet(list.TraktListId)
		if errors.Is(err, client.ErrNotFound) {
			tc.ListAdd(list.TraktListId, list.ImdbListName)
		}
		list.TraktList = traktList
	}
	u.ratings = imdb.DataPair{
		ImdbList:  ic.RatingsGet(),
		TraktList: tc.RatingsGet(),
	}
}

func (u *user) syncLists(tc *trakt.Client) {
	for _, list := range u.lists {
		diff := list.Difference()
		if len(diff["add"]) > 0 {
			if list.IsWatchlist {
				tc.WatchlistItemsAdd(diff["add"])
				continue
			}
			tc.ListItemsAdd(list.TraktListId, diff["add"])
		}
		if len(diff["remove"]) > 0 {
			if list.IsWatchlist {
				tc.WatchlistItemsRemove(diff["remove"])
				continue
			}
			tc.ListItemsRemove(list.TraktListId, diff["remove"])
		}
	}
	// Remove lists that only exist in Trakt
	/*
	traktLists := tc.ListsGet()
	for _, tl := range traktLists {
		if !contains(u.lists, tl.Name) {
			tc.ListRemove(tl.Ids.Slug)
		}
	}
	*/
}

func (u *user) syncRatings(tc *trakt.Client) {
	diff := u.ratings.Difference()
	if len(diff["add"]) > 0 {
		tc.RatingsAdd(diff["add"])
		for _, ti := range diff["add"] {
			history := tc.HistoryGet(ti)
			if len(history) > 0 {
				continue
			}
			tc.HistoryAdd([]trakt.Item{ti})
		}
	}
	if len(diff["remove"]) > 0 {
		tc.RatingsRemove(diff["remove"])
		for _, ti := range diff["remove"] {
			history := tc.HistoryGet(ti)
			if len(history) == 0 {
				continue
			}
			tc.HistoryRemove([]trakt.Item{ti})
		}
	}
	var ratingsToUpdate []trakt.Item
	for _, imdbItem := range u.ratings.ImdbList {
		if imdbItem.Rating != nil {
			for _, traktItem := range u.ratings.TraktList {
				switch traktItem.Type {
				case "movie":
					if imdbItem.Id == traktItem.Movie.Ids.Imdb && *imdbItem.Rating != traktItem.Rating {
						traktItem.Movie.Rating = *imdbItem.Rating
						traktItem.Movie.RatedAt = imdbItem.RatingDate.UTC().String()
						ratingsToUpdate = append(ratingsToUpdate, traktItem)
					}
				case "show":
					if imdbItem.Id == traktItem.Show.Ids.Imdb && *imdbItem.Rating != traktItem.Rating {
						traktItem.Show.Rating = *imdbItem.Rating
						traktItem.Show.RatedAt = imdbItem.RatingDate.UTC().String()
						ratingsToUpdate = append(ratingsToUpdate, traktItem)
					}
				case "episode":
					if imdbItem.Id == traktItem.Episode.Ids.Imdb && *imdbItem.Rating != traktItem.Rating {
						traktItem.Episode.Rating = *imdbItem.Rating
						traktItem.Episode.RatedAt = imdbItem.RatingDate.UTC().String()
						ratingsToUpdate = append(ratingsToUpdate, traktItem)
					}
				}
			}
		}
	}
	if len(ratingsToUpdate) > 0 {
		tc.RatingsAdd(ratingsToUpdate)
	}
}

// cleanupLists removes invalid imdb lists passed via the IMDB_LIST_IDS
// env variable and returns only the lists that actually exist
func cleanupLists(ic *imdb.Client, imdbListIds []string) []imdb.DataPair {
	lists := make([]imdb.DataPair, len(imdbListIds))
	n := 0
	for _, imdbListId := range imdbListIds {
		imdbListName, imdbList, err := ic.ListItemsGet(imdbListId)
		if errors.Is(err, errors.New("resource not found")) {
			continue
		}
		lists[n] = imdb.DataPair{
			ImdbList:     imdbList,
			ImdbListId:   imdbListId,
			ImdbListName: *imdbListName,
			TraktListId:  imdb.FormatTraktListName(*imdbListName),
		}
		n++
	}
	return lists[:n]
}

func contains(dps []imdb.DataPair, traktListName string) bool {
	for _, dp := range dps {
		if dp.ImdbListName == traktListName {
			return true
		}
	}
	return false
}
