package syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	appconfig "github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/entities"
	"github.com/cecobask/imdb-trakt-sync/pkg/client"
	"github.com/cecobask/imdb-trakt-sync/pkg/logger"
)

type Syncer struct {
	logger      *slog.Logger
	imdbClient  client.IMDbClientInterface
	traktClient client.TraktClientInterface
	user        *user
	conf        appconfig.Sync
	authless    bool
}

type user struct {
	imdbLists    map[string]entities.IMDbList
	imdbRatings  map[string]entities.IMDbItem
	traktLists   map[string]entities.TraktList
	traktRatings map[string]entities.TraktItem
}

func NewSyncer(ctx context.Context, conf *appconfig.Config) (*Syncer, error) {
	log := logger.NewLogger(os.Stdout)
	imdbClient, err := client.NewIMDbClient(ctx, &conf.IMDb, log)
	if err != nil {
		return nil, fmt.Errorf("failure initialising imdb client: %w", err)
	}
	traktClient, err := client.NewTraktClient(conf.Trakt, log)
	if err != nil {
		return nil, fmt.Errorf("failure initialising trakt client: %w", err)
	}
	syncer := &Syncer{
		logger:      log,
		imdbClient:  imdbClient,
		traktClient: traktClient,
		user: &user{
			imdbLists:    make(map[string]entities.IMDbList, len(*conf.IMDb.Lists)),
			imdbRatings:  make(map[string]entities.IMDbItem),
			traktLists:   make(map[string]entities.TraktList, len(*conf.IMDb.Lists)),
			traktRatings: make(map[string]entities.TraktItem),
		},
		conf:     conf.Sync,
		authless: *conf.IMDb.Auth == appconfig.IMDbAuthMethodNone,
	}
	for _, lid := range *conf.IMDb.Lists {
		syncer.user.imdbLists[lid] = entities.IMDbList{ListID: lid}
	}
	return syncer, nil
}

func (s *Syncer) Sync() error {
	s.logger.Info("sync started")
	if err := s.hydrate(); err != nil {
		s.logger.Error("failure hydrating imdb client", logger.Error(err))
		return err
	}
	if err := s.syncLists(); err != nil {
		s.logger.Error("failure syncing lists", logger.Error(err))
		return err
	}
	if err := s.syncRatings(); err != nil {
		s.logger.Error("failure syncing ratings", logger.Error(err))
		return err
	}
	if err := s.syncHistory(); err != nil {
		s.logger.Error("failure syncing history", logger.Error(err))
		return err
	}
	s.logger.Info("sync completed")
	return nil
}

func (s *Syncer) hydrate() error {
	lids := make([]string, len(s.user.imdbLists))
	var i int
	for lid := range s.user.imdbLists {
		lids[i] = lid
		i++
	}
	if *s.conf.Ratings {
		if err := s.imdbClient.RatingsExport(); err != nil {
			return fmt.Errorf("failure exporting imdb ratings: %w", err)
		}
	}
	if err := s.imdbClient.ListsExport(lids...); err != nil {
		return fmt.Errorf("failure exporting imdb lists: %w", err)
	}
	if *s.conf.Watchlist {
		if err := s.imdbClient.WatchlistExport(); err != nil {
			return fmt.Errorf("failure exporting imdb watchlist: %w", err)
		}
	}
	imdbLists, err := s.imdbClient.ListsGet(lids...)
	if err != nil {
		return fmt.Errorf("failure fetching imdb lists: %w", err)
	}
	traktIDMetas := make(entities.TraktIDMetas, 0, len(imdbLists))
	for _, imdbList := range imdbLists {
		s.user.imdbLists[imdbList.ListID] = imdbList
		traktIDMetas = append(traktIDMetas, entities.TraktIDMeta{
			IMDb:     imdbList.ListID,
			Slug:     entities.InferTraktListSlug(imdbList.ListName),
			ListName: &imdbList.ListName,
		})
	}
	traktLists, delegatedErrors := s.traktClient.ListsGet(traktIDMetas)
	for _, delegatedErr := range delegatedErrors {
		var notFoundError *client.TraktListNotFoundError
		if errors.As(delegatedErr, &notFoundError) {
			listName := traktIDMetas.GetListNameFromSlug(notFoundError.Slug)
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
				msg := fmt.Sprintf("sync mode %s would have created trakt list %s to backfill imdb list %s", syncMode, notFoundError.Slug, listName)
				s.logger.Info(msg)
				continue
			}
			if err = s.traktClient.ListAdd(notFoundError.Slug, listName); err != nil {
				return fmt.Errorf("failure creating trakt list: %w", err)
			}
			continue
		}
		return fmt.Errorf("failure hydrating trakt lists: %w", delegatedErr)
	}
	for _, traktList := range traktLists {
		s.user.traktLists[traktList.IDMeta.IMDb] = traktList
	}
	if s.authless {
		return nil
	}
	if *s.conf.Watchlist {
		imdbWatchlist, err := s.imdbClient.WatchlistGet()
		if err != nil {
			return fmt.Errorf("failure fetching imdb watchlist: %w", err)
		}
		s.user.imdbLists[imdbWatchlist.ListID] = *imdbWatchlist
		traktWatchlist, err := s.traktClient.WatchlistGet()
		if err != nil {
			return fmt.Errorf("failure fetching trakt watchlist: %w", err)
		}
		s.user.traktLists[imdbWatchlist.ListID] = *traktWatchlist
	}
	if *s.conf.Ratings {
		traktRatings, err := s.traktClient.RatingsGet()
		if err != nil {
			return fmt.Errorf("failure fetching trakt ratings: %w", err)
		}
		for _, traktRating := range traktRatings {
			id, err := traktRating.GetItemID()
			if err != nil {
				return fmt.Errorf("failure fetching trakt item id: %w", err)
			}
			if id != nil {
				s.user.traktRatings[*id] = traktRating
			}
		}
		imdbRatings, err := s.imdbClient.RatingsGet()
		if err != nil {
			return fmt.Errorf("failure fetching imdb ratings: %w", err)
		}
		for _, imdbRating := range imdbRatings {
			s.user.imdbRatings[imdbRating.ID] = imdbRating
		}
	}
	return nil
}

func (s *Syncer) syncLists() error {
	for _, list := range s.user.imdbLists {
		traktListSlug := entities.InferTraktListSlug(list.ListName)
		diff := entities.ListDifference(list, s.user.traktLists[list.ListID])
		if list.IsWatchlist {
			if len(diff["add"]) > 0 {
				if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
					msg := fmt.Sprintf("sync mode %s would have added %d trakt list item(s)", syncMode, len(diff["add"]))
					s.logger.Info(msg, slog.Any("watchlist", diff["add"]))
					continue
				}
				if err := s.traktClient.WatchlistItemsAdd(diff["add"]); err != nil {
					return fmt.Errorf("failure adding items to trakt watchlist: %w", err)
				}
			}
			if len(diff["remove"]) > 0 {
				if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun || syncMode == appconfig.SyncModeAddOnly {
					msg := fmt.Sprintf("sync mode %s would have deleted %d trakt list item(s)", syncMode, len(diff["remove"]))
					s.logger.Info(msg, slog.Any("watchlist", diff["remove"]))
					continue
				}
				if err := s.traktClient.WatchlistItemsRemove(diff["remove"]); err != nil {
					return fmt.Errorf("failure removing items from trakt watchlist: %w", err)
				}
			}
			continue
		}
		if len(diff["add"]) > 0 {
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
				msg := fmt.Sprintf("sync mode %s would have added %d trakt list item(s)", syncMode, len(diff["add"]))
				s.logger.Info(msg, slog.Any(traktListSlug, diff["add"]))
				continue
			}
			if err := s.traktClient.ListItemsAdd(traktListSlug, diff["add"]); err != nil {
				return fmt.Errorf("failure adding items to trakt list %s: %w", traktListSlug, err)
			}
		}
		if len(diff["remove"]) > 0 {
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun || syncMode == appconfig.SyncModeAddOnly {
				msg := fmt.Sprintf("sync mode %s would have deleted %d trakt list item(s)", syncMode, len(diff["remove"]))
				s.logger.Info(msg, slog.Any(traktListSlug, diff["remove"]))
				continue
			}
			if err := s.traktClient.ListItemsRemove(traktListSlug, diff["remove"]); err != nil {
				return fmt.Errorf("failure removing items from trakt list %s: %w", traktListSlug, err)
			}
		}
	}
	return nil
}

func (s *Syncer) syncRatings() error {
	if s.authless {
		s.logger.Info("skipping ratings sync since no imdb auth was provided")
		return nil
	}
	if !*s.conf.Ratings {
		s.logger.Info("skipping ratings sync")
		return nil
	}
	diff := entities.ItemsDifference(s.user.imdbRatings, s.user.traktRatings)
	if len(diff["add"]) > 0 {
		if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
			msg := fmt.Sprintf("sync mode %s would have added %d trakt rating item(s)", syncMode, len(diff["add"]))
			s.logger.Info(msg, slog.Any("ratings", diff["add"]))
		} else {
			if err := s.traktClient.RatingsAdd(diff["add"]); err != nil {
				return fmt.Errorf("failure adding trakt ratings: %w", err)
			}
		}
	}
	if len(diff["remove"]) > 0 {
		if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun || syncMode == appconfig.SyncModeAddOnly {
			msg := fmt.Sprintf("sync mode %s would have deleted %d trakt rating item(s)", syncMode, len(diff["remove"]))
			s.logger.Info(msg, slog.Any("ratings", diff["remove"]))
		} else {
			if err := s.traktClient.RatingsRemove(diff["remove"]); err != nil {
				return fmt.Errorf("failure removing trakt ratings: %w", err)
			}
		}
	}
	return nil
}

func (s *Syncer) syncHistory() error {
	if s.authless {
		s.logger.Info("skipping history sync since no imdb auth was provided")
		return nil
	}
	if !*s.conf.History {
		s.logger.Info("skipping history sync")
		return nil
	}
	// imdb doesn't offer functionality similar to trakt history, hence why there can't be a direct mapping between them
	// the syncer will assume a user to have watched an item if they've submitted a rating for it
	// if the above is satisfied and the user's history for this item is empty, a new history entry is added!
	diff := entities.ItemsDifference(s.user.imdbRatings, s.user.traktRatings)
	if len(diff["add"]) > 0 {
		var historyToAdd entities.TraktItems
		for i := range diff["add"] {
			traktItemID, err := diff["add"][i].GetItemID()
			if err != nil {
				return fmt.Errorf("failure fetching trakt item id: %w", err)
			}
			history, err := s.traktClient.HistoryGet(diff["add"][i].Type, *traktItemID)
			if err != nil {
				return fmt.Errorf("failure fetching trakt history for %s %s: %w", diff["add"][i].Type, *traktItemID, err)
			}
			if len(history) > 0 {
				continue
			}
			historyToAdd = append(historyToAdd, diff["add"][i])
		}
		if len(historyToAdd) > 0 {
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
				msg := fmt.Sprintf("sync mode %s would have added %d trakt history item(s)", syncMode, len(historyToAdd))
				s.logger.Info(msg, slog.Any("history", historyToAdd))
			} else {
				if err := s.traktClient.HistoryAdd(historyToAdd); err != nil {
					return fmt.Errorf("failure adding trakt history: %w", err)
				}
			}
		}
	}
	if len(diff["remove"]) > 0 {
		var historyToRemove entities.TraktItems
		for i := range diff["remove"] {
			traktItemID, err := diff["remove"][i].GetItemID()
			if err != nil {
				return fmt.Errorf("failure fetching trakt item id: %w", err)
			}
			history, err := s.traktClient.HistoryGet(diff["remove"][i].Type, *traktItemID)
			if err != nil {
				return fmt.Errorf("failure fetching trakt history for %s %s: %w", diff["remove"][i].Type, *traktItemID, err)
			}
			if len(history) == 0 {
				continue
			}
			historyToRemove = append(historyToRemove, diff["remove"][i])
		}
		if len(historyToRemove) > 0 {
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun || syncMode == appconfig.SyncModeAddOnly {
				msg := fmt.Sprintf("sync mode %s would have deleted %d trakt history item(s)", syncMode, len(historyToRemove))
				s.logger.Info(msg, slog.Any("history", historyToRemove))
			} else {
				if err := s.traktClient.HistoryRemove(historyToRemove); err != nil {
					return fmt.Errorf("failure removing trakt history: %w", err)
				}
			}
		}
	}
	return nil
}
