package syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	appconfig "github.com/cecobask/imdb-trakt-sync/internal/config"
	"github.com/cecobask/imdb-trakt-sync/internal/imdb"
	"github.com/cecobask/imdb-trakt-sync/internal/logger"
	"github.com/cecobask/imdb-trakt-sync/internal/trakt"
)

type Syncer struct {
	logger      *slog.Logger
	imdbClient  imdb.API
	traktClient trakt.API
	user        *user
	conf        appconfig.Sync
	authless    bool
}

type user struct {
	imdbLists    map[string]imdb.List
	imdbRatings  map[string]imdb.Item
	traktLists   map[string]trakt.List
	traktRatings map[string]trakt.Item
}

func NewSyncer(ctx context.Context, conf *appconfig.Config) (*Syncer, error) {
	log := logger.NewLogger(os.Stdout)
	imdbClient, err := imdb.NewAPI(ctx, &conf.IMDb, log)
	if err != nil {
		return nil, fmt.Errorf("failure initialising imdb client: %w", err)
	}
	traktClient, err := trakt.NewAPI(ctx, conf.Trakt, log)
	if err != nil {
		return nil, fmt.Errorf("failure initialising trakt client: %w", err)
	}
	syncer := &Syncer{
		logger:      log,
		imdbClient:  imdbClient,
		traktClient: traktClient,
		user:        &user{},
		conf:        conf.Sync,
		authless:    *conf.IMDb.Auth == appconfig.IMDbAuthMethodNone,
	}
	if *conf.Sync.Ratings {
		syncer.user.imdbRatings = make(map[string]imdb.Item)
		syncer.user.traktRatings = make(map[string]trakt.Item)
	}
	if *conf.Sync.Lists || *conf.Sync.Watchlist {
		syncer.user.imdbLists = make(map[string]imdb.List, len(*conf.IMDb.Lists))
		syncer.user.traktLists = make(map[string]trakt.List, len(*conf.IMDb.Lists))
		for _, lid := range *conf.IMDb.Lists {
			syncer.user.imdbLists[lid] = imdb.List{ListID: lid}
		}
	}
	return syncer, nil
}

func (s *Syncer) Sync(ctx context.Context) error {
	s.logger.Info("sync started")
	if err := s.hydrate(ctx); err != nil {
		s.logger.Error("failure hydrating imdb client", logger.Error(err))
		return err
	}
	if err := s.syncLists(ctx); err != nil {
		s.logger.Error("failure syncing lists", logger.Error(err))
		return err
	}
	if err := s.syncRatings(ctx); err != nil {
		s.logger.Error("failure syncing ratings", logger.Error(err))
		return err
	}
	if err := s.syncHistory(ctx); err != nil {
		s.logger.Error("failure syncing history", logger.Error(err))
		return err
	}
	s.logger.Info("sync completed")
	return nil
}

func (s *Syncer) hydrate(ctx context.Context) error {
	lids := make([]string, 0, len(s.user.imdbLists))
	for lid := range s.user.imdbLists {
		lids = append(lids, lid)
	}
	if *s.conf.Ratings {
		if err := s.imdbClient.RatingsExport(); err != nil {
			return fmt.Errorf("failure exporting imdb ratings: %w", err)
		}
	}
	if *s.conf.Lists {
		if err := s.imdbClient.ListsExport(lids...); err != nil {
			return fmt.Errorf("failure exporting imdb lists: %w", err)
		}
	}
	if *s.conf.Watchlist {
		if err := s.imdbClient.WatchlistExport(); err != nil {
			return fmt.Errorf("failure exporting imdb watchlist: %w", err)
		}
	}
	if *s.conf.Lists {
		imdbLists, err := s.imdbClient.ListsGet(lids...)
		if err != nil {
			return fmt.Errorf("failure fetching imdb lists: %w", err)
		}
		traktIDMetas := make(trakt.IDMetas, 0, len(imdbLists))
		for _, imdbList := range imdbLists {
			s.user.imdbLists[imdbList.ListID] = imdbList
			traktIDMetas = append(traktIDMetas, trakt.IDMeta{
				IMDb:     imdbList.ListID,
				Slug:     inferTraktListSlug(imdbList.ListName),
				ListName: &imdbList.ListName,
			})
		}
		traktLists, delegatedErrors := s.traktClient.ListsGet(ctx, traktIDMetas)
		for _, delegatedErr := range delegatedErrors {
			var lnferr *trakt.ListNotFoundError
			if errors.As(delegatedErr, &lnferr) {
				listName := traktIDMetas.GetListNameFromSlug(lnferr.Slug)
				if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
					msg := fmt.Sprintf("sync mode %s would have created trakt list %s to backfill imdb list %s", syncMode, lnferr.Slug, listName)
					s.logger.Info(msg)
					continue
				}
				if err = s.traktClient.ListAdd(ctx, lnferr.Slug, listName); err != nil {
					return fmt.Errorf("failure creating trakt list: %w", err)
				}
				continue
			}
			return fmt.Errorf("failure hydrating trakt lists: %w", delegatedErr)
		}
		for _, traktList := range traktLists {
			s.user.traktLists[traktList.IDMeta.IMDb] = traktList
		}
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
		traktWatchlist, err := s.traktClient.WatchlistGet(ctx)
		if err != nil {
			return fmt.Errorf("failure fetching trakt watchlist: %w", err)
		}
		s.user.traktLists[imdbWatchlist.ListID] = *traktWatchlist
	}
	if *s.conf.Ratings {
		traktRatings, err := s.traktClient.RatingsGet(ctx)
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

func (s *Syncer) syncLists(ctx context.Context) error {
	if !*s.conf.Watchlist {
		s.logger.Info("skipping watchlist sync")
	}
	if !*s.conf.Lists {
		s.logger.Info("skipping lists sync")
	}
	if !*s.conf.Watchlist && !*s.conf.Lists {
		return nil
	}
	for _, imdbList := range s.user.imdbLists {
		diff := listDiff(imdbList, s.user.traktLists[imdbList.ListID])
		if imdbList.IsWatchlist {
			if len(diff.Add) > 0 {
				if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
					msg := fmt.Sprintf("sync mode %s would have added %d trakt list item(s)", syncMode, len(diff.Add))
					s.logger.Info(msg, slog.Any("watchlist", diff.Add))
					continue
				}
				if err := s.traktClient.WatchlistItemsAdd(ctx, diff.Add); err != nil {
					return fmt.Errorf("failure adding items to trakt watchlist: %w", err)
				}
			} else {
				s.logger.Info("no items to add to trakt watchlist")
			}
			if len(diff.Remove) > 0 {
				if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun || syncMode == appconfig.SyncModeAddOnly {
					msg := fmt.Sprintf("sync mode %s would have deleted %d trakt list item(s)", syncMode, len(diff.Remove))
					s.logger.Info(msg, slog.Any("watchlist", diff.Remove))
					continue
				}
				if err := s.traktClient.WatchlistItemsRemove(ctx, diff.Remove); err != nil {
					return fmt.Errorf("failure removing items from trakt watchlist: %w", err)
				}
			} else {
				s.logger.Info("no items to remove from trakt watchlist")
			}
			continue
		}
		slug := inferTraktListSlug(imdbList.ListName)
		if len(diff.Add) > 0 {
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
				msg := fmt.Sprintf("sync mode %s would have added %d trakt list item(s)", syncMode, len(diff.Add))
				s.logger.Info(msg, slog.Any(slug, diff.Add))
				continue
			}
			if err := s.traktClient.ListItemsAdd(ctx, slug, diff.Add); err != nil {
				return fmt.Errorf("failure adding items to trakt list %s: %w", slug, err)
			}
		} else {
			s.logger.Info("no items to add to trakt list", slog.String("slug", slug), slog.String("name", imdbList.ListName))
		}
		if len(diff.Remove) > 0 {
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun || syncMode == appconfig.SyncModeAddOnly {
				msg := fmt.Sprintf("sync mode %s would have deleted %d trakt list item(s)", syncMode, len(diff.Remove))
				s.logger.Info(msg, slog.Any(slug, diff.Remove))
				continue
			}
			if err := s.traktClient.ListItemsRemove(ctx, slug, diff.Remove); err != nil {
				return fmt.Errorf("failure removing items from trakt list %s: %w", slug, err)
			}
		} else {
			s.logger.Info("no items to remove from trakt list", slog.String("slug", slug), slog.String("name", imdbList.ListName))
		}
	}
	return nil
}

func (s *Syncer) syncRatings(ctx context.Context) error {
	if s.authless {
		s.logger.Info("skipping ratings sync since no imdb auth was provided")
		return nil
	}
	if !*s.conf.Ratings {
		s.logger.Info("skipping ratings sync")
		return nil
	}
	diff := itemsDifference(s.user.imdbRatings, s.user.traktRatings)
	if len(diff.Add) > 0 {
		if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
			msg := fmt.Sprintf("sync mode %s would have added %d trakt rating item(s)", syncMode, len(diff.Add))
			s.logger.Info(msg, slog.Any("ratings", diff.Add))
		} else {
			if err := s.traktClient.RatingsAdd(ctx, diff.Add); err != nil {
				return fmt.Errorf("failure adding trakt ratings: %w", err)
			}
		}
	} else {
		s.logger.Info("no ratings to add to trakt")
	}
	if len(diff.Remove) > 0 {
		if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun || syncMode == appconfig.SyncModeAddOnly {
			msg := fmt.Sprintf("sync mode %s would have deleted %d trakt rating item(s)", syncMode, len(diff.Remove))
			s.logger.Info(msg, slog.Any("ratings", diff.Remove))
		} else {
			if err := s.traktClient.RatingsRemove(ctx, diff.Remove); err != nil {
				return fmt.Errorf("failure removing trakt ratings: %w", err)
			}
		}
	} else {
		s.logger.Info("no ratings to remove from trakt")
	}
	return nil
}

func (s *Syncer) syncHistory(ctx context.Context) error {
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
	diff := itemsDifference(s.user.imdbRatings, s.user.traktRatings)
	if len(diff.Add) > 0 {
		var historyToAdd trakt.Items
		for i := range diff.Add {
			traktItemID, err := diff.Add[i].GetItemID()
			if err != nil {
				return fmt.Errorf("failure fetching trakt item id: %w", err)
			}
			history, err := s.traktClient.HistoryGet(ctx, diff.Add[i].Type, *traktItemID)
			if err != nil {
				return fmt.Errorf("failure fetching trakt history for %s %s: %w", diff.Add[i].Type, *traktItemID, err)
			}
			if len(history) > 0 {
				continue
			}
			historyToAdd = append(historyToAdd, diff.Add[i])
		}
		if len(historyToAdd) > 0 {
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun {
				msg := fmt.Sprintf("sync mode %s would have added %d trakt history item(s)", syncMode, len(historyToAdd))
				s.logger.Info(msg, slog.Any("history", historyToAdd))
			} else {
				if err := s.traktClient.HistoryAdd(ctx, historyToAdd); err != nil {
					return fmt.Errorf("failure adding trakt history: %w", err)
				}
			}
		}
	} else {
		s.logger.Info("no history items to add to trakt")
	}
	if len(diff.Remove) > 0 {
		var historyToRemove trakt.Items
		for i := range diff.Remove {
			traktItemID, err := diff.Remove[i].GetItemID()
			if err != nil {
				return fmt.Errorf("failure fetching trakt item id: %w", err)
			}
			history, err := s.traktClient.HistoryGet(ctx, diff.Remove[i].Type, *traktItemID)
			if err != nil {
				return fmt.Errorf("failure fetching trakt history for %s %s: %w", diff.Remove[i].Type, *traktItemID, err)
			}
			if len(history) == 0 {
				continue
			}
			historyToRemove = append(historyToRemove, diff.Remove[i])
		}
		if len(historyToRemove) > 0 {
			if syncMode := *s.conf.Mode; syncMode == appconfig.SyncModeDryRun || syncMode == appconfig.SyncModeAddOnly {
				msg := fmt.Sprintf("sync mode %s would have deleted %d trakt history item(s)", syncMode, len(historyToRemove))
				s.logger.Info(msg, slog.Any("history", historyToRemove))
			} else {
				if err := s.traktClient.HistoryRemove(ctx, historyToRemove); err != nil {
					return fmt.Errorf("failure removing trakt history: %w", err)
				}
			}
		}
	} else {
		s.logger.Info("no history items to remove from trakt")
	}
	return nil
}
