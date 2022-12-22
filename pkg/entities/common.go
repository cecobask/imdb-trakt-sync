package entities

type DataPair struct {
	ImdbList     []ImdbItem
	ImdbListId   string
	ImdbListName string
	TraktList    []TraktItem
	TraktListId  string
	IsWatchlist  bool
}

func (dp *DataPair) Difference() map[string][]TraktItem {
	diff := make(map[string][]TraktItem)
	// items to be added to trakt
	temp := make(map[string]struct{})
	for _, tlItem := range dp.TraktList {
		switch tlItem.Type {
		case TraktItemTypeMovie:
			temp[tlItem.Movie.Ids.Imdb] = struct{}{}
		case TraktItemTypeShow:
			temp[tlItem.Show.Ids.Imdb] = struct{}{}
		case TraktItemTypeEpisode:
			temp[tlItem.Episode.Ids.Imdb] = struct{}{}
		default:
			continue
		}
	}
	for _, ilItem := range dp.ImdbList {
		if _, found := temp[ilItem.Id]; !found {
			ti := TraktItem{}
			tiSpec := TraktItemSpec{
				Ids: TraktIds{
					Imdb: ilItem.Id,
				},
			}
			if ilItem.Rating != nil {
				ti.WatchedAt = ilItem.RatingDate.UTC().String()
				tiSpec.RatedAt = ilItem.RatingDate.UTC().String()
				tiSpec.Rating = *ilItem.Rating
			}
			switch ilItem.TitleType {
			case imdbItemTypeMovie:
				ti.Type = TraktItemTypeMovie
				ti.Movie = tiSpec
			case imdbItemTypeTvSeries:
				ti.Type = TraktItemTypeShow
				ti.Show = tiSpec
			case imdbItemTypeTvMiniSeries:
				ti.Type = TraktItemTypeShow
				ti.Show = tiSpec
			case imdbItemTypeTvEpisode:
				ti.Type = TraktItemTypeEpisode
				ti.Episode = tiSpec
			default:
				ti.Type = TraktItemTypeMovie
				ti.Movie = tiSpec
			}
			diff["add"] = append(diff["add"], ti)
		}
	}
	// out of sync items to be removed from trakt
	temp = make(map[string]struct{})
	for _, ilItem := range dp.ImdbList {
		temp[ilItem.Id] = struct{}{}
	}
	for _, tlItem := range dp.TraktList {
		var itemId string
		switch tlItem.Type {
		case TraktItemTypeMovie:
			itemId = tlItem.Movie.Ids.Imdb
		case TraktItemTypeShow:
			itemId = tlItem.Show.Ids.Imdb
		case TraktItemTypeEpisode:
			itemId = tlItem.Episode.Ids.Imdb
		default:
			continue
		}
		if _, found := temp[itemId]; !found {
			diff["remove"] = append(diff["remove"], tlItem)
		}
	}
	return diff
}
