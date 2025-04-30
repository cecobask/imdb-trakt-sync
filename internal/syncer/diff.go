package syncer

import (
	"regexp"
	"slices"
	"strings"

	"github.com/cecobask/imdb-trakt-sync/internal/imdb"
	"github.com/cecobask/imdb-trakt-sync/internal/trakt"
)

type diff struct {
	Add    []trakt.Item
	Remove []trakt.Item
}

func newDiff() diff {
	return diff{
		Add:    make(trakt.Items, 0),
		Remove: make(trakt.Items, 0),
	}
}

func (d *diff) Sort() {
	sortFunc := func(a, b trakt.Item) int { return a.Created.Compare(b.Created) }
	slices.SortFunc(d.Add, sortFunc)
	slices.SortFunc(d.Remove, sortFunc)
}

func listDiff(imdbList imdb.List, traktList trakt.List) diff {
	imdbItems := make(map[string]imdb.Item)
	for _, item := range imdbList.ListItems {
		imdbItems[item.ID] = item
	}
	traktItems := make(map[string]trakt.Item)
	for _, item := range traktList.ListItems {
		id, err := item.GetItemID()
		if err != nil || id == nil {
			continue
		}
		traktItems[*id] = item
	}
	return itemsDifference(imdbItems, traktItems)
}

func itemsDifference(imdbItems map[string]imdb.Item, traktItems map[string]trakt.Item) diff {
	diff := newDiff()
	for id, imdbItem := range imdbItems {
		traktItem := imdbItem.ToTraktItem()
		if _, found := traktItems[id]; !found {
			diff.Add = append(diff.Add, traktItem)
			continue
		}
		if imdbItem.Rating != nil && *imdbItem.Rating != traktItems[id].Rating {
			diff.Add = append(diff.Add, traktItem)
		}
	}
	for id, traktItem := range traktItems {
		if _, found := imdbItems[id]; !found {
			traktItem.Created = imdbItems[id].Created
			diff.Remove = append(diff.Remove, traktItem)
		}
	}
	diff.Sort()
	return diff
}

func inferTraktListSlug(imdbListName string) string {
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
