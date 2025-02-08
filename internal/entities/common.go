package entities

import (
	"regexp"
	"slices"
	"strings"
)

type Diff struct {
	Add    []TraktItem
	Remove []TraktItem
}

func newDiff() Diff {
	return Diff{
		Add:    make([]TraktItem, 0),
		Remove: make([]TraktItem, 0),
	}
}

func (d *Diff) Sort() {
	sortFunc := func(a, b TraktItem) int { return a.created.Compare(b.created) }
	slices.SortFunc(d.Add, sortFunc)
	slices.SortFunc(d.Remove, sortFunc)
}

func ListDiff(imdbList IMDbList, traktList TraktList) Diff {
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

func ItemsDifference(imdbItems map[string]IMDbItem, traktItems map[string]TraktItem) Diff {
	diff := newDiff()
	for id, imdbItem := range imdbItems {
		traktItem := imdbItem.toTraktItem()
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
			traktItem.created = imdbItems[id].Created
			diff.Remove = append(diff.Remove, traktItem)
		}
	}
	diff.Sort()
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
