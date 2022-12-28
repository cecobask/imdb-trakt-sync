package syncer

import (
	"errors"
	"fmt"
	"github.com/cecobask/imdb-trakt-sync/pkg/client"
	"github.com/cecobask/imdb-trakt-sync/pkg/entities"
	"github.com/cecobask/imdb-trakt-sync/pkg/logger"
	_ "github.com/joho/godotenv/autoload"
	"go.uber.org/zap"
	"os"
	"strings"
)

const (
	EnvVarKeyCookieAtMain   = "IMDB_COOKIE_AT_MAIN"
	EnvVarKeyCookieUbidMain = "IMDB_COOKIE_UBID_MAIN"
	EnvVarKeyListIds        = "IMDB_LIST_IDS"
	EnvVarKeyUserId         = "IMDB_USER_ID"
	EnvVarKeyClientId       = "TRAKT_CLIENT_ID"
	EnvVarKeyClientSecret   = "TRAKT_CLIENT_SECRET"
	EnvVarKeyPassword       = "TRAKT_PASSWORD"
	EnvVarKeyUsername       = "TRAKT_USERNAME"
)

type Syncer struct {
	logger      *zap.Logger
	imdbClient  client.ImdbClientInterface
	traktClient client.TraktClientInterface
	user        *user
}

type user struct {
	lists   []entities.DataPair
	ratings entities.DataPair
}

func NewSyncer() *Syncer {
	syncer := &Syncer{
		logger: logger.NewLogger(),
		user:   &user{},
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
			ClientId:     os.Getenv(EnvVarKeyClientId),
			ClientSecret: os.Getenv(EnvVarKeyClientSecret),
			Username:     os.Getenv(EnvVarKeyUsername),
			Password:     os.Getenv(EnvVarKeyPassword),
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
			syncer.user.lists = append(syncer.user.lists, entities.DataPair{
				ImdbListId: strings.ReplaceAll(imdbListIds[i], " ", ""),
			})
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
	if len(s.user.lists) != 0 {
		s.cleanupLists()
	} else {
		dps, err := s.imdbClient.ListsScrape()
		if err != nil {
			return fmt.Errorf("failure scraping imdb lists: %w", err)
		}
		s.user.lists = dps
	}
	watchlistId, imdbList, err := s.imdbClient.WatchlistGet()
	if err != nil {
		return fmt.Errorf("failure fetching imdb watchlist: %w", err)
	}
	s.user.lists = append(s.user.lists, entities.DataPair{
		ImdbList:     imdbList,
		ImdbListId:   *watchlistId,
		ImdbListName: "watchlist",
		IsWatchlist:  true,
	})
	for i := range s.user.lists {
		currentList := &s.user.lists[i]
		if currentList.IsWatchlist {
			traktWatchlist, err := s.traktClient.WatchlistItemsGet()
			if err != nil {
				return fmt.Errorf("failure fetching trakt watchlist: %w", err)
			}
			currentList.TraktList = traktWatchlist
			continue
		}
		traktList, err := s.traktClient.ListItemsGet(currentList.TraktListId)
		if err != nil {
			if errors.As(err, new(*client.ResourceNotFoundError)) {
				if err = s.traktClient.ListAdd(currentList.TraktListId, currentList.ImdbListName); err != nil {
					return fmt.Errorf("failure creating trakt list %s", currentList.TraktListId)
				}
				currentList.TraktList = traktList
				continue
			}
			return fmt.Errorf("failure fetching contents of trakt list %s", currentList.TraktListId)
		}
		currentList.TraktList = traktList
	}
	imdbRatings, err := s.imdbClient.RatingsGet()
	if err != nil {
		return fmt.Errorf("failure fetching imdb ratings: %w", err)
	}
	traktRatings, err := s.traktClient.RatingsGet()
	if err != nil {
		return fmt.Errorf("failure fetching trakt ratings: %w", err)
	}
	s.user.ratings = entities.DataPair{
		ImdbList:  imdbRatings,
		TraktList: traktRatings,
	}
	return nil
}

func (s *Syncer) syncLists() error {
	for _, list := range s.user.lists {
		diff := list.Difference()
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
			if err := s.traktClient.ListItemsAdd(list.TraktListId, diff["add"]); err != nil {
				return fmt.Errorf("failure adding items to trakt list %s: %w", list.TraktListId, err)
			}
		}
		if len(diff["remove"]) > 0 {
			if err := s.traktClient.ListItemsRemove(list.TraktListId, diff["remove"]); err != nil {
				return fmt.Errorf("failure removing items from trakt list %s: %w", list.TraktListId, err)
			}
		}
	}
	// Remove lists that only exist in Trakt
	traktLists, err := s.traktClient.ListsGet()
	if err != nil {
		return fmt.Errorf("failure fetching trakt lists: %w", err)
	}
	for _, tl := range traktLists {
		if !contains(s.user.lists, tl.Name) {
			if err = s.traktClient.ListRemove(tl.Ids.Slug); err != nil {
				return fmt.Errorf("failure removing trakt list %s: %w", tl.Name, err)
			}
		}
	}
	return nil
}

func (s *Syncer) syncRatings() error {
	diff := s.user.ratings.Difference()
	if len(diff["add"]) > 0 {

		if err := s.traktClient.RatingsAdd(diff["add"]); err != nil {
			return fmt.Errorf("failure adding trakt ratings: %w", err)
		}
		for _, ti := range diff["add"] {
			traktItemType, traktItemId, err := client.GetTraktItemTypeAndId(ti)
			if err != nil {
				return fmt.Errorf("failure fetching trakt item type: %w", err)
			}
			history, err := s.traktClient.HistoryGet(traktItemType, traktItemId)
			if err != nil {
				return fmt.Errorf("failure fetching trakt history for %s %s: %w", traktItemType, traktItemId, err)
			}
			if len(history) > 0 {
				continue
			}
			if err = s.traktClient.HistoryAdd([]entities.TraktItem{ti}); err != nil {
				return fmt.Errorf("failure adding trakt history for %s %s: %w", traktItemType, traktItemId, err)
			}
		}
	}
	if len(diff["remove"]) > 0 {
		if err := s.traktClient.RatingsRemove(diff["remove"]); err != nil {
			return fmt.Errorf("failure removing trakt ratings: %w", err)
		}
		for _, ti := range diff["remove"] {
			traktItemType, traktItemId, err := client.GetTraktItemTypeAndId(ti)
			if err != nil {
				return fmt.Errorf("failure fetching trakt item type: %w", err)
			}
			history, err := s.traktClient.HistoryGet(traktItemType, traktItemId)
			if err != nil {
				return fmt.Errorf("failure fetching trakt history for %s %s: %w", traktItemType, traktItemId, err)
			}
			if len(history) == 0 {
				continue
			}
			if err = s.traktClient.HistoryRemove([]entities.TraktItem{ti}); err != nil {
				return fmt.Errorf("failure removing trakt history for %s %s: %w", traktItemType, traktItemId, err)
			}
		}
	}
	var ratingsToUpdate []entities.TraktItem
	for _, imdbItem := range s.user.ratings.ImdbList {
		if imdbItem.Rating != nil {
			for _, traktItem := range s.user.ratings.TraktList {
				switch traktItem.Type {
				case entities.TraktItemTypeMovie:
					if imdbItem.Id == traktItem.Movie.Ids.Imdb && *imdbItem.Rating != traktItem.Rating {
						traktItem.Movie.Rating = *imdbItem.Rating
						traktItem.Movie.RatedAt = imdbItem.RatingDate.UTC().String()
						ratingsToUpdate = append(ratingsToUpdate, traktItem)
					}
				case entities.TraktItemTypeShow:
					if imdbItem.Id == traktItem.Show.Ids.Imdb && *imdbItem.Rating != traktItem.Rating {
						traktItem.Show.Rating = *imdbItem.Rating
						traktItem.Show.RatedAt = imdbItem.RatingDate.UTC().String()
						ratingsToUpdate = append(ratingsToUpdate, traktItem)
					}
				case entities.TraktItemTypeEpisode:
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
		if err := s.traktClient.RatingsAdd(ratingsToUpdate); err != nil {
			return fmt.Errorf("failure updating trakt ratings: %w", err)
		}
	}
	return nil
}

// cleanupLists ignore duplicate and non-existent imdb lists
func (s *Syncer) cleanupLists() {
	uniqueListNames := make(map[string]bool)
	lists := make([]entities.DataPair, len(s.user.lists))
	n := 0
	for _, list := range s.user.lists {
		if _, found := uniqueListNames[list.ImdbListId]; found {
			continue
		}
		uniqueListNames[list.ImdbListId] = true
		imdbListName, imdbList, err := s.imdbClient.ListItemsGet(list.ImdbListId)
		if errors.As(err, new(*client.ResourceNotFoundError)) {
			continue
		}
		lists[n] = entities.DataPair{
			ImdbList:     imdbList,
			ImdbListId:   list.ImdbListId,
			ImdbListName: *imdbListName,
			TraktListId:  client.FormatTraktListName(*imdbListName),
		}
		n++
	}
	s.user.lists = lists[:n]
}

func validateEnvVars() error {
	requiredEnvVarKeys := []string{
		EnvVarKeyListIds,
		EnvVarKeyCookieAtMain,
		EnvVarKeyCookieUbidMain,
		EnvVarKeyClientId,
		EnvVarKeyClientSecret,
		EnvVarKeyUsername,
		EnvVarKeyPassword,
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

func contains(dps []entities.DataPair, traktListName string) bool {
	for _, dp := range dps {
		if dp.ImdbListName == traktListName {
			return true
		}
	}
	return false
}
