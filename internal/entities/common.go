package entities

import (
	"regexp"
	"strings"
)

func ListDifference(imdbList IMDbList, traktList TraktList) map[string]TraktItems {
	imdbItems := make(map[string]IMDbItem)
	for _, item := range imdbList.ListItems {
		imdbItems[item.ID] = item
	}
	traktItems := make(map[string]TraktItem)
	for _, item := range traktList.ListItems {
		id, err := item.GetItemID()
		if err != nil || id == nil {
			continue
		}
		traktItems[*id] = item
	}
	return ItemsDifference(imdbItems, traktItems)
}

func ItemsDifference(imdbItems map[string]IMDbItem, traktItems map[string]TraktItem) map[string]TraktItems {
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

func InferTraktListSlug(imdbListName string) string {
	result := strings.ToLower(strings.Join(strings.Fields(imdbListName), "-"))
	regex := regexp.MustCompile(`[^-_a-z0-9]+`)
	result = removeDuplicateAdjacentCharacters(regex.ReplaceAllString(result, ""), '-')
	return result
}

func removeDuplicateAdjacentCharacters(value string, target rune) string {
	var sb strings.Builder
	for i, char := range value {
		if i == 0 || char != target || rune(value[i-1]) != target {
			sb.WriteRune(char)
		}
	}
	return sb.String()
}
