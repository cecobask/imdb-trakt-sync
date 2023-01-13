package entities

func ListDifference(imdbList ImdbList, traktList TraktList) map[string]TraktItems {
	imdbItems := make(map[string]ImdbItem)
	for _, item := range imdbList.ListItems {
		imdbItems[item.Id] = item
	}
	traktItems := make(map[string]TraktItem)
	for _, item := range traktList.ListItems {
		id, err := item.GetItemId()
		if err != nil || id == nil {
			continue
		}
		traktItems[*id] = item
	}
	return ItemsDifference(imdbItems, traktItems)
}

func ItemsDifference(imdbItems map[string]ImdbItem, traktItems map[string]TraktItem) map[string]TraktItems {
	diff := make(map[string]TraktItems)
	for id, imdbItem := range imdbItems {
		traktItem := imdbItem.toTraktItem()
		if _, found := traktItems[id]; !found {
			diff["add"] = append(diff["add"], traktItem)
			continue
		}
		if imdbItem.Rating != nil && *imdbItem.Rating != traktItems[id].Rating {
			diff["add"] = append(diff["add"], traktItem)
			continue
		}
	}
	for id, traktItem := range traktItems {
		if _, found := imdbItems[id]; !found {
			diff["remove"] = append(diff["remove"], traktItem)
		}
	}
	return diff
}
