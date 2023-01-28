package syncer

import (
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
	EnvVarKeyCookieAtMain      = "IMDB_COOKIE_AT_MAIN"
	EnvVarKeyCookieUbidMain    = "IMDB_COOKIE_UBID_MAIN"
	EnvVarKeyListIds           = "IMDB_LIST_IDS"
	EnvVarKeySyncMode          = "SYNC_MODE"
	EnvVarKeyTraktClientId     = "TRAKT_CLIENT_ID"
	EnvVarKeyTraktClientSecret = "TRAKT_CLIENT_SECRET"
	EnvVarKeyTraktEmail        = "TRAKT_EMAIL"
	EnvVarKeyTraktPassword     = "TRAKT_PASSWORD"
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
			SyncMode:     os.Getenv(EnvVarKeySyncMode),
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
	s.logger.Info("successfully ran the syncer")
}

func (s *Syncer) hydrate() (err error) {
	var imdbLists []entities.ImdbList
	if len(s.user.imdbLists) != 0 {
		listIds := make([]string, 0, len(s.user.imdbLists))
		for id := range s.user.imdbLists {
			listIds = append(listIds, id)
		}
		imdbLists, err = s.imdbClient.ListsGet(listIds)
		if err != nil {
			return fmt.Errorf("failure hydrating imdb lists: %w", err)
		}
	} else {
		imdbLists, err = s.imdbClient.ListsGetAll()
		if err != nil {
			return fmt.Errorf("failure fetching all imdb lists: %w", err)
		}
	}
	traktIds := make([]entities.TraktIds, 0, len(imdbLists))
	for i := range imdbLists {
		imdbList := imdbLists[i]
		s.user.imdbLists[imdbList.ListId] = imdbList
		traktIds = append(traktIds, entities.TraktIds{
			Imdb: imdbList.ListId,
			Slug: imdbList.TraktListSlug,
		})
	}
	traktLists, err := s.traktClient.ListsGet(traktIds)
	if err != nil {
		return fmt.Errorf("failure hydrating trakt lists: %w", err)
	}
	for i := range traktLists {
		traktList := traktLists[i]
		s.user.traktLists[traktList.Ids.Imdb] = traktList
	}
	imdbWatchlist, err := s.imdbClient.WatchlistGet()
	if err != nil {
		return fmt.Errorf("failure fetching imdb watchlist: %w", err)
	}
	s.user.imdbLists[imdbWatchlist.ListId] = *imdbWatchlist
	traktWatchlist, err := s.traktClient.WatchlistGet()
	if err != nil {
		return fmt.Errorf("failure fetching trakt watchlist: %w", err)
	}
	s.user.traktLists[imdbWatchlist.ListId] = *traktWatchlist
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
		if id != nil {
			s.user.traktRatings[*id] = traktRating
		}
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
	traktLists, err := s.traktClient.ListsMetadataGet()
	if err != nil {
		return fmt.Errorf("failure fetching trakt lists: %w", err)
	}
	for i := range traktLists {
		if traktListIsStray(s.user.imdbLists, *traktLists[i].Name) {
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
		var historyToAdd entities.TraktItems
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
		var historyToRemove entities.TraktItems
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
	return nil
}

func validateEnvVars() error {
	requiredEnvVarKeys := []string{
		EnvVarKeyCookieAtMain,
		EnvVarKeyCookieUbidMain,
		EnvVarKeyListIds,
		EnvVarKeySyncMode,
		EnvVarKeyTraktClientId,
		EnvVarKeyTraktClientSecret,
		EnvVarKeyTraktEmail,
		EnvVarKeyTraktPassword,
	}
	var missingEnvVars []string
	for i := range requiredEnvVarKeys {
		if value, ok := os.LookupEnv(requiredEnvVarKeys[i]); !ok || value == "" {
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

func traktListIsStray(imdbLists map[string]entities.ImdbList, traktListName string) bool {
	for _, imdbList := range imdbLists {
		if imdbList.ListName == traktListName {
			return false
		}
	}
	return true
}
