package main

import (
	"fmt"
	_ "github.com/joho/godotenv/autoload"
	"os"
	"strings"
)

type listPair struct {
	imdbListId string
	imdbItems  []imdbListItem
	listName   string
	traktItems []traktListItem
}

type user struct {
	imdbUserId  string
	traktUserId string
	watchlist   listPair
	customLists []listPair
}

func run() {
	validateEnvVars()
	imdbClient := newImdbClient()
	traktClient := newTraktClient()
	imdbUserId, imdbWatchlistId := os.Getenv(imdbUserIdKey), os.Getenv(imdbWatchlistIdKey)
	u := user{
		imdbUserId:  imdbUserId,
		traktUserId: os.Getenv(traktUserIdKey),
	}
	_, listItems := imdbClient.getList(imdbWatchlistId)
	u.watchlist = listPair{
		imdbListId: imdbWatchlistId,
		imdbItems:  listItems,
	}
	imdbListIdsString := os.Getenv(imdbCustomListIdsKey)
	if len(imdbListIdsString) > 0 {
		imdbListIds := strings.Split(imdbListIdsString, ",")
		for _, imdbListID := range imdbListIds {
			u.customLists = append(u.customLists, listPair{
				imdbListId: imdbListID,
			})
		}
	}
	u.customLists = cleanupCustomLists(u.customLists, imdbClient)
	u.watchlist.traktItems = traktClient.getWatchlist()
	fmt.Println(len(u.customLists))
	fmt.Println(len(u.watchlist.imdbItems))
	fmt.Println(len(u.watchlist.traktItems))
}

func cleanupCustomLists(customLists []listPair, imdbClient *imdbClient) []listPair {
	n := 0
	for _, customList := range customLists {
		listName, listItems := imdbClient.getList(customList.imdbListId)
		if listName != "" {
			customLists[n] = listPair{
				imdbListId: customList.imdbListId,
				imdbItems:  listItems,
				listName:   listName,
			}
			n++
		}
	}
	return customLists[:n]
}
