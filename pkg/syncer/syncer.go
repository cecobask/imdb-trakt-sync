package syncer

import (
	"errors"
	"fmt"
	"github.com/cecobask/imdb-trakt-sync/pkg/client"
	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"github.com/cecobask/imdb-trakt-sync/pkg/logger"
	_ "github.com/joho/godotenv/autoload"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strings"
)

const (
	EnvVarKeyCookieAtMain      = "IMDB_COOKIE_AT_MAIN"
	EnvVarKeyCookieUbidMain    = "IMDB_COOKIE_UBID_MAIN"
	EnvVarKeyListIds           = "IMDB_LIST_IDS"
	EnvVarKeyTraktClientId     = "TRAKT_CLIENT_ID"
	EnvVarKeyTraktClientSecret = "TRAKT_CLIENT_SECRET"
	EnvVarKeyTraktEmail        = "TRAKT_EMAIL"
	EnvVarKeyTraktPassword     = "TRAKT_PASSWORD"
	EnvVarKeyUserId            = "IMDB_USER_ID"
)

type Syncer struct {
	logger      *zap.Logger
	imdbClient  client.ImdbClientInterface
	traktClient client.TraktClientInterface
	user        *user
}

type user struct {
	imdbLists    map[string]entities.ImdbList
	imdbRatings  map[string]entities.ImdbItem
	traktLists   map[string]entities.TraktList
	traktRatings map[string]entities.TraktItem
}

func NewSyncer() *Syncer {
	syncer := &Syncer{
		logger: logger.NewLogger(),
		user: &user{
			imdbLists:    make(map[string]entities.ImdbList),
			imdbRatings:  make(map[string]entities.ImdbItem),
			traktLists:   make(map[string]entities.TraktList),
			traktRatings: make(map[string]entities.TraktItem),
		},
	}
	if err := validateEnvVars(); err != nil {
		syncer.logger.Fatal("failure validating environment variables", zap.Error(err))
	}
	imdbClient, err := client.NewImdbClient(
		client.ImdbConfig{
			CookieAtMain:   os.Getenv(EnvVarKeyCookieAtMain),
			CookieUbidMain: os.Getenv(EnvVarKeyCookieUbidMain),
			UserId:         os.Getenv(EnvVarKeyUserId),
		},
		syncer.logger,
	)
	if err != nil {
		syncer.logger.Fatal("failure initialising imdb client", zap.Error(err))
	}
	syncer.imdbClient = imdbClient
	traktClient, err := client.NewTraktClient(
		client.TraktConfig{
			ClientId:     os.Getenv(EnvVarKeyTraktClientId),
			ClientSecret: os.Getenv(EnvVarKeyTraktClientSecret),
			Email:        os.Getenv(EnvVarKeyTraktEmail),
			Password:     os.Getenv(EnvVarKeyTraktPassword),
		},
		syncer.logger,
	)
	if err != nil {
		syncer.logger.Fatal("failure initialising trakt client", zap.Error(err))
	}
	syncer.traktClient = traktClient
	if imdbListIdsString := os.Getenv(EnvVarKeyListIds); imdbListIdsString != "" && imdbListIdsString != "all" {
		imdbListIds := strings.Split(imdbListIdsString, ",")
		for i := range imdbListIds {
			listId := strings.ReplaceAll(imdbListIds[i], " ", "")
			syncer.user.imdbLists[listId] = entities.ImdbList{ListId: listId}
		}
	}
	return syncer
}

func (s *Syncer) Run() {
	if err := s.hydrate(); err != nil {
		s.logger.Fatal("failure hydrating imdb client", zap.Error(err))
	}
	if err := s.syncLists(); err != nil {
		s.logger.Fatal("failure syncing lists", zap.Error(err))
	}
	if err := s.syncRatings(); err != nil {
		s.logger.Fatal("failure syncing ratings", zap.Error(err))
	}
	s.logger.Info("successfully synced trakt with imdb")
}

func (s *Syncer) hydrate() error {
	if len(s.user.imdbLists) != 0 {
		if err := s.cleanupLists(); err != nil {
			return fmt.Errorf("failure cleaning up imdb lists: %w", err)
		}
	} else {
		imdbLists, err := s.imdbClient.ListsGetAll()
		if err != nil {
			return fmt.Errorf("failure fetching all imdb lists: %w", err)
		}
		for i := range imdbLists {
			imdbList := imdbLists[i]
			s.user.imdbLists[imdbList.ListId] = imdbList
		}
	}
	imdbWatchlist, err := s.imdbClient.WatchlistGet()
	if err != nil {
		return fmt.Errorf("failure fetching imdb watchlist: %w", err)
	}
	s.user.imdbLists[imdbWatchlist.ListId] = *imdbWatchlist
	for imdbListId := range s.user.imdbLists {
		currentList := s.user.imdbLists[imdbListId]
		if currentList.IsWatchlist {
			traktWatchlist, err := s.traktClient.WatchlistGet()
			if err != nil {
				return fmt.Errorf("failure fetching trakt watchlist: %w", err)
			}
			s.user.traktLists[currentList.ListId] = *traktWatchlist
			continue
		}
		traktList, err := s.traktClient.ListGet(currentList.TraktListSlug)
		if err != nil {
			var apiError *client.ApiError
			if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
				s.logger.Warn("silencing not found error while hydrating the syncer with trakt lists", zap.Error(apiError))
				if err = s.traktClient.ListAdd(currentList.TraktListSlug, currentList.ListName); err != nil {
					return fmt.Errorf("failure creating trakt list %s: %w", currentList.TraktListSlug, err)
				}
				s.user.traktLists[currentList.ListId] = *traktList
				continue
			}
			return fmt.Errorf("unexpected error while fetching contents of trakt list %s: %w", currentList.TraktListSlug, err)
		}
		s.user.traktLists[currentList.ListId] = *traktList
	}
	imdbRatings, err := s.imdbClient.RatingsGet()
	if err != nil {
		return fmt.Errorf("failure fetching imdb ratings: %w", err)
	}
	for i := range imdbRatings {
		imdbRating := imdbRatings[i]
		s.user.imdbRatings[imdbRating.Id] = imdbRating
	}
	traktRatings, err := s.traktClient.RatingsGet()
	if err != nil {
		return fmt.Errorf("failure fetching trakt ratings: %w", err)
	}
	for i := range traktRatings {
		traktRating := traktRatings[i]
		id, err := traktRating.GetItemId()
		if err != nil {
			return fmt.Errorf("failure fetching trakt item id: %w", err)
		}
		s.user.traktRatings[*id] = traktRating
	}
	return nil
}

func (s *Syncer) syncLists() error {
	for _, list := range s.user.imdbLists {
		diff := entities.ListDifference(list, s.user.traktLists[list.ListId])
		if list.IsWatchlist {
			if len(diff["add"]) > 0 {
				if err := s.traktClient.WatchlistItemsAdd(diff["add"]); err != nil {
					return fmt.Errorf("failure adding items to trakt watchlist: %w", err)
				}
			}
			if len(diff["remove"]) > 0 {
				if err := s.traktClient.WatchlistItemsRemove(diff["remove"]); err != nil {
					return fmt.Errorf("failure removing items from trakt watchlist: %w", err)
				}
			}
			continue
		}
		if len(diff["add"]) > 0 {
			if err := s.traktClient.ListItemsAdd(list.TraktListSlug, diff["add"]); err != nil {
				return fmt.Errorf("failure adding items to trakt list %s: %w", list.TraktListSlug, err)
			}
		}
		if len(diff["remove"]) > 0 {
			if err := s.traktClient.ListItemsRemove(list.TraktListSlug, diff["remove"]); err != nil {
				return fmt.Errorf("failure removing items from trakt list %s: %w", list.TraktListSlug, err)
			}
		}
	}
	// remove lists that only exist in Trakt
	traktLists, err := s.traktClient.ListsGet()
	if err != nil {
		return fmt.Errorf("failure fetching trakt lists: %w", err)
	}
	for i := range traktLists {
		if !contains(s.user.imdbLists, *traktLists[i].Name) {
			if err = s.traktClient.ListRemove(traktLists[i].Ids.Slug); err != nil {
				return fmt.Errorf("failure removing trakt list %s: %w", *traktLists[i].Name, err)
			}
		}
	}
	return nil
}

func (s *Syncer) syncRatings() error {
	diff := entities.ItemsDifference(s.user.imdbRatings, s.user.traktRatings)
	if len(diff["add"]) > 0 {
		if err := s.traktClient.RatingsAdd(diff["add"]); err != nil {
			return fmt.Errorf("failure adding trakt ratings: %w", err)
		}
		var historyToAdd []entities.TraktItem
		for i := range diff["add"] {
			traktItemId, err := diff["add"][i].GetItemId()
			if err != nil {
				return fmt.Errorf("failure fetching trakt item id: %w", err)
			}
			history, err := s.traktClient.HistoryGet(diff["add"][i].Type, *traktItemId)
			if err != nil {
				return fmt.Errorf("failure fetching trakt history for %s %s: %w", diff["add"][i].Type, *traktItemId, err)
			}
			if len(history) > 0 {
				continue
			}
			historyToAdd = append(historyToAdd, diff["add"][i])
		}
		if len(historyToAdd) > 0 {
			if err := s.traktClient.HistoryAdd(historyToAdd); err != nil {
				return fmt.Errorf("failure adding trakt history: %w", err)
			}
		}
	}
	if len(diff["remove"]) > 0 {
		if err := s.traktClient.RatingsRemove(diff["remove"]); err != nil {
			return fmt.Errorf("failure removing trakt ratings: %w", err)
		}
		var historyToRemove []entities.TraktItem
		for i := range diff["remove"] {
			traktItemId, err := diff["remove"][i].GetItemId()
			if err != nil {
				return fmt.Errorf("failure fetching trakt item id: %w", err)
			}
			history, err := s.traktClient.HistoryGet(diff["remove"][i].Type, *traktItemId)
			if err != nil {
				return fmt.Errorf("failure fetching trakt history for %s %s: %w", diff["remove"][i].Type, *traktItemId, err)
			}
			if len(history) == 0 {
				continue
			}
			historyToRemove = append(historyToRemove, diff["remove"][i])
		}
		if len(historyToRemove) > 0 {
			if err := s.traktClient.HistoryRemove(historyToRemove); err != nil {
				return fmt.Errorf("failure removing trakt history: %w", err)
			}
		}
	}
	//var ratingsToUpdate []entities.TraktItem
	//for _, imdbItem := range s.user.ratings.ImdbList {
	//	if imdbItem.Rating != nil {
	//		for _, traktItem := range s.user.ratings.TraktList {
	//			ratedAt := imdbItem.RatingDate.UTC().String()
	//			switch traktItem.Type {
	//			case entities.TraktItemTypeMovie:
	//				if imdbItem.Id == traktItem.Movie.Ids.Imdb && *imdbItem.Rating != traktItem.Rating {
	//					traktItem.Movie.Rating = imdbItem.Rating
	//					traktItem.Movie.RatedAt = &ratedAt
	//					ratingsToUpdate = append(ratingsToUpdate, traktItem)
	//				}
	//			case entities.TraktItemTypeShow:
	//				if imdbItem.Id == traktItem.Show.Ids.Imdb && *imdbItem.Rating != traktItem.Rating {
	//					traktItem.Show.Rating = imdbItem.Rating
	//					traktItem.Show.RatedAt = &ratedAt
	//					ratingsToUpdate = append(ratingsToUpdate, traktItem)
	//				}
	//			case entities.TraktItemTypeEpisode:
	//				if imdbItem.Id == traktItem.Episode.Ids.Imdb && *imdbItem.Rating != traktItem.Rating {
	//					traktItem.Episode.Rating = imdbItem.Rating
	//					traktItem.Episode.RatedAt = &ratedAt
	//					ratingsToUpdate = append(ratingsToUpdate, traktItem)
	//				}
	//			}
	//		}
	//	}
	//}
	//if len(ratingsToUpdate) > 0 {
	//	if err := s.traktClient.RatingsAdd(ratingsToUpdate); err != nil {
	//		return fmt.Errorf("failure updating trakt ratings: %w", err)
	//	}
	//}
	return nil
}

// cleanupLists ignore non-existent imdb lists
func (s *Syncer) cleanupLists() error {
	existingLists := make(map[string]entities.ImdbList)
	for id := range s.user.imdbLists {
		imdbList, err := s.imdbClient.ListGet(id)
		if err != nil {
			var apiError *client.ApiError
			if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
				s.logger.Warn("silencing not found error while cleaning up user provided imdb lists", zap.Error(apiError))
				continue
			}
			return fmt.Errorf("unexpected error while cleaning up user provided imdb lists: %w", err)
		}
		existingLists[id] = *imdbList
	}
	s.user.imdbLists = existingLists
	return nil
}

func validateEnvVars() error {
	requiredEnvVarKeys := []string{
		EnvVarKeyListIds,
		EnvVarKeyCookieAtMain,
		EnvVarKeyCookieUbidMain,
		EnvVarKeyTraktClientId,
		EnvVarKeyTraktClientSecret,
		EnvVarKeyTraktEmail,
		EnvVarKeyTraktPassword,
	}
	var missingEnvVars []string
	for i := range requiredEnvVarKeys {
		if _, ok := os.LookupEnv(requiredEnvVarKeys[i]); !ok {
			missingEnvVars = append(missingEnvVars, requiredEnvVarKeys[i])
		}
	}
	if len(missingEnvVars) > 0 {
		return &MissingEnvironmentVariablesError{
			variables: missingEnvVars,
		}
	}
	return nil
}

func contains(imdbLists map[string]entities.ImdbList, traktListName string) bool {
	for _, imdbList := range imdbLists {
		if imdbList.ListName == traktListName {
			return true
		}
	}
	return false
}
