package syncer

import (
	"context"
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

func (s *Syncer) setupTraktLists(ctx context.Context, imdbLists imdb.Lists) (trakt.IDMetas, error) {
	traktLists, err := s.traktClient.ListsGetAllMeta(ctx)
	if err != nil {
		return nil, fmt.Errorf("failure fetching trakt lists metadata: %w", err)
	}
	traktListsMetaMap := make(map[string]trakt.IDMeta, len(traktLists))
	for _, traktList := range traktLists {
		traktListsMetaMap[*traktList.Name] = traktList.IDMeta
	}

	traktListsMeta := make(trakt.IDMetas, 0, len(imdbLists))
	for _, imdbList := range imdbLists {
		s.user.imdbLists[imdbList.ListID] = imdbList
		if *s.conf.Mode == appconfig.SyncModeDryRun {
			s.logger.Info("sync would have created trakt list", "name", imdbList.ListName)
			continue
		}
		traktListMeta, ok := traktListsMetaMap[imdbList.ListName]
		if !ok {
			newTraktListMeta, err := s.traktClient.ListCreate(ctx, imdbList.ListName)
			if err != nil {
				return nil, fmt.Errorf("failure creating trakt list %s: %w", imdbList.ListName, err)
			}
			traktListMeta = *newTraktListMeta
		}
		traktListMeta.IMDb = imdbList.ListID
		traktListsMeta = append(traktListsMeta, traktListMeta)
	}

	return traktListsMeta, nil
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
		traktIDMetas, err := s.setupTraktLists(ctx, imdbLists)
		if err != nil {
			return fmt.Errorf("failure setting up trakt lists: %w", err)
		}
		traktLists, err := s.traktClient.ListsGet(ctx, traktIDMetas)
		if err != nil {
			return fmt.Errorf("failure hydrating trakt lists: %w", err)
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
				if *s.conf.Mode == appconfig.SyncModeDryRun {
					s.logger.Info("sync would have added trakt watchlist items", "count", len(diff.Add))
					continue
				}
				if err := s.traktClient.WatchlistItemsAdd(ctx, diff.Add); err != nil {
					return fmt.Errorf("failure adding items to trakt watchlist: %w", err)
				}
			} else {
				s.logger.Info("no trakt watchlist items to add")
			}
			if len(diff.Remove) > 0 {
				if *s.conf.Mode == appconfig.SyncModeDryRun || *s.conf.Mode == appconfig.SyncModeAddOnly {
					s.logger.Info("sync would have removed trakt watchlist items", "count", len(diff.Remove))
					continue
				}
				if err := s.traktClient.WatchlistItemsRemove(ctx, diff.Remove); err != nil {
					return fmt.Errorf("failure removing trakt watchlist items: %w", err)
				}
			} else {
				s.logger.Info("no trakt watchlist items to remove")
			}
			continue
		}
		traktListID := s.user.traktLists[imdbList.ListID].IDMeta.Trakt
		if len(diff.Add) > 0 {
			if *s.conf.Mode == appconfig.SyncModeDryRun {
				s.logger.Info("sync would have added trakt list items", "count", len(diff.Add), "name", imdbList.ListName)
				continue
			}
			if err := s.traktClient.ListItemsAdd(ctx, traktListID, imdbList.ListName, diff.Add); err != nil {
				return fmt.Errorf("failure adding items to trakt list %s: %w", imdbList.ListName, err)
			}
		} else {
			s.logger.Info("no trakt list items to add", "name", imdbList.ListName)
		}
		if len(diff.Remove) > 0 {
			if *s.conf.Mode == appconfig.SyncModeDryRun || *s.conf.Mode == appconfig.SyncModeAddOnly {
				s.logger.Info("sync would have deleted trakt list items", "count", len(diff.Remove), "name", imdbList.ListName)
				continue
			}
			if err := s.traktClient.ListItemsRemove(ctx, traktListID, imdbList.ListName, diff.Remove); err != nil {
				return fmt.Errorf("failure removing trakt list items from %s: %w", imdbList.ListName, err)
			}
		} else {
			s.logger.Info("no trakt list items to remove", "name", imdbList.ListName)
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
		if *s.conf.Mode == appconfig.SyncModeDryRun {
			s.logger.Info("sync would have added trakt ratings", "count", len(diff.Add))
		} else {
			if err := s.traktClient.RatingsAdd(ctx, diff.Add); err != nil {
				return fmt.Errorf("failure adding trakt ratings: %w", err)
			}
		}
	} else {
		s.logger.Info("no trakt ratings to add")
	}
	if len(diff.Remove) > 0 {
		if *s.conf.Mode == appconfig.SyncModeDryRun || *s.conf.Mode == appconfig.SyncModeAddOnly {
			s.logger.Info("sync would have deleted trakt ratings", "count", len(diff.Remove))
		} else {
			if err := s.traktClient.RatingsRemove(ctx, diff.Remove); err != nil {
				return fmt.Errorf("failure removing trakt ratings: %w", err)
			}
		}
	} else {
		s.logger.Info("no trakt ratings to remove")
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
			if *s.conf.Mode == appconfig.SyncModeDryRun {
				s.logger.Info("sync would have added trakt history", "count", len(historyToAdd))
			} else {
				if err := s.traktClient.HistoryAdd(ctx, historyToAdd); err != nil {
					return fmt.Errorf("failure adding trakt history: %w", err)
				}
			}
		}
	} else {
		s.logger.Info("no history to add to trakt")
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
			if *s.conf.Mode == appconfig.SyncModeDryRun || *s.conf.Mode == appconfig.SyncModeAddOnly {
				s.logger.Info("sync would have deleted trakt history", "count", len(historyToRemove))
			} else {
				if err := s.traktClient.HistoryRemove(ctx, historyToRemove); err != nil {
					return fmt.Errorf("failure removing trakt history: %w", err)
				}
			}
		}
	} else {
		s.logger.Info("no trakt history to remove")
	}
	return nil
}
