// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"context"
	"database/sql"
	"strconv"
	"sync"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/store"
	"github.com/mattermost/mattermost-server/v6/utils"
)

type SqlThreadStore struct {
	*SqlStore
}

func (s *SqlThreadStore) ClearCaches() {
}

func newSqlThreadStore(sqlStore *SqlStore) store.ThreadStore {
	return &SqlThreadStore{
		SqlStore: sqlStore,
	}
}

func (s *SqlThreadStore) Get(id string) (*model.Thread, error) {
	var thread model.Thread
	query, args, err := s.getQueryBuilder().
		Select("*").
		From("Threads").
		Where(sq.Eq{"PostId": id}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "thread_tosql")
	}
	err = s.GetMasterX().Get(&thread, query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		return nil, errors.Wrapf(err, "failed to get thread with id=%s", id)
	}
	return &thread, nil
}

func (s *SqlThreadStore) GetThreadsForUser(userId, teamId string, opts model.GetUserThreadsOpts) (*model.Threads, error) {
	type JoinedThread struct {
		PostId         string
		ReplyCount     int64
		LastReplyAt    int64
		LastViewedAt   int64
		UnreadReplies  int64
		UnreadMentions int64
		Participants   model.StringArray
		model.Post
	}

	fetchConditions := sq.And{
		sq.Eq{"ThreadMemberships.UserId": userId},
		sq.Eq{"ThreadMemberships.Following": true},
		sq.Or{sq.Eq{"Channels.TeamId": teamId}, sq.Eq{"Channels.TeamId": ""}},
	}
	if !opts.Deleted {
		fetchConditions = sq.And{
			fetchConditions,
			sq.Eq{"COALESCE(Posts.DeleteAt, 0)": 0},
		}
	}

	pageSize := uint64(30)
	if opts.PageSize != 0 {
		pageSize = opts.PageSize
	}

	totalUnreadThreadsChan := make(chan store.StoreResult, 1)
	totalCountChan := make(chan store.StoreResult, 1)
	totalUnreadMentionsChan := make(chan store.StoreResult, 1)
	var threadsChan chan store.StoreResult
	if !opts.TotalsOnly {
		threadsChan = make(chan store.StoreResult, 1)
	}

	go func() {
		repliesQuery, repliesQueryArgs, _ := s.getQueryBuilder().
			Select("COUNT(DISTINCT(Posts.RootId))").
			From("Posts").
			LeftJoin("ThreadMemberships ON Posts.RootId = ThreadMemberships.PostId").
			LeftJoin("Channels ON Posts.ChannelId = Channels.Id").
			Where(fetchConditions).
			Where("Posts.CreateAt > ThreadMemberships.LastViewed").ToSql()

		var totalUnreadThreads int64
		err := s.GetMasterX().Get(&totalUnreadThreads, repliesQuery, repliesQueryArgs...)
		totalUnreadThreadsChan <- store.StoreResult{Data: totalUnreadThreads, NErr: errors.Wrapf(err, "failed to get count unread on threads for user id=%s", userId)}
		close(totalUnreadThreadsChan)
	}()
	go func() {
		newFetchConditions := fetchConditions

		if opts.Unread {
			newFetchConditions = sq.And{newFetchConditions, sq.Expr("ThreadMemberships.LastViewed < Threads.LastReplyAt")}
		}

		threadsQuery, threadsQueryArgs, _ := s.getQueryBuilder().
			Select("COUNT(ThreadMemberships.PostId)").
			LeftJoin("Threads ON Threads.PostId = ThreadMemberships.PostId").
			LeftJoin("Channels ON Threads.ChannelId = Channels.Id").
			LeftJoin("Posts ON Posts.Id = ThreadMemberships.PostId").
			From("ThreadMemberships").
			Where(newFetchConditions).ToSql()

		var totalCount int64
		err := s.GetMasterX().Get(&totalCount, threadsQuery, threadsQueryArgs...)
		totalCountChan <- store.StoreResult{Data: totalCount, NErr: err}
		close(totalCountChan)
	}()
	go func() {
		mentionsQuery, mentionsQueryArgs, _ := s.getQueryBuilder().
			Select("COALESCE(SUM(ThreadMemberships.UnreadMentions),0)").
			From("ThreadMemberships").
			LeftJoin("Threads ON Threads.PostId = ThreadMemberships.PostId").
			LeftJoin("Posts ON Posts.Id = ThreadMemberships.PostId").
			LeftJoin("Channels ON Threads.ChannelId = Channels.Id").
			Where(fetchConditions).ToSql()

		var totalUnreadMentions int64
		err := s.GetMasterX().Get(&totalUnreadMentions, mentionsQuery, mentionsQueryArgs...)
		totalUnreadMentionsChan <- store.StoreResult{Data: totalUnreadMentions, NErr: err}
		close(totalUnreadMentionsChan)
	}()

	if !opts.TotalsOnly {
		go func() {
			newFetchConditions := fetchConditions
			if opts.Since > 0 {
				newFetchConditions = sq.And{newFetchConditions, sq.GtOrEq{"ThreadMemberships.LastUpdated": opts.Since}}
			}
			order := "DESC"
			if opts.Before != "" {
				newFetchConditions = sq.And{
					newFetchConditions,
					sq.Expr(`LastReplyAt < (SELECT LastReplyAt FROM Threads WHERE PostId = ?)`, opts.Before),
				}
			}
			if opts.After != "" {
				order = "ASC"
				newFetchConditions = sq.And{
					newFetchConditions,
					sq.Expr(`LastReplyAt > (SELECT LastReplyAt FROM Threads WHERE PostId = ?)`, opts.After),
				}
			}
			if opts.Unread {
				newFetchConditions = sq.And{newFetchConditions, sq.Expr("ThreadMemberships.LastViewed < Threads.LastReplyAt")}
			}

			unreadRepliesFetchConditions := sq.And{
				sq.Expr("Posts.RootId = ThreadMemberships.PostId"),
				sq.Expr("Posts.CreateAt > ThreadMemberships.LastViewed"),
			}
			if !opts.Deleted {
				unreadRepliesFetchConditions = sq.And{
					unreadRepliesFetchConditions,
					sq.Expr("Posts.DeleteAt = 0"),
				}
			}

			unreadRepliesQuery, _ := sq.
				Select("COUNT(Posts.Id)").
				From("Posts").
				Where(unreadRepliesFetchConditions).
				MustSql()

			threads := []*JoinedThread{}
			query, args, _ := s.getQueryBuilder().
				Select(`Threads.*,
				` + postSliceCoalesceQuery() + `,
				ThreadMemberships.LastViewed as LastViewedAt,
				ThreadMemberships.UnreadMentions as UnreadMentions`).
				From("Threads").
				Column(sq.Alias(sq.Expr(unreadRepliesQuery), "UnreadReplies")).
				LeftJoin("Posts ON Posts.Id = Threads.PostId").
				LeftJoin("Channels ON Posts.ChannelId = Channels.Id").
				LeftJoin("ThreadMemberships ON ThreadMemberships.PostId = Threads.PostId").
				Where(newFetchConditions).
				OrderBy("Threads.LastReplyAt " + order).
				Limit(pageSize).ToSql()

			err := s.GetReplicaX().Select(&threads, query, args...)
			threadsChan <- store.StoreResult{Data: threads, NErr: err}
			close(threadsChan)
		}()
	}

	totalUnreadMentionsResult := <-totalUnreadMentionsChan
	if totalUnreadMentionsResult.NErr != nil {
		return nil, totalUnreadMentionsResult.NErr
	}
	totalUnreadMentions := totalUnreadMentionsResult.Data.(int64)

	totalCountResult := <-totalCountChan
	if totalCountResult.NErr != nil {
		return nil, totalCountResult.NErr
	}
	totalCount := totalCountResult.Data.(int64)

	totalUnreadThreadsResult := <-totalUnreadThreadsChan
	if totalUnreadThreadsResult.NErr != nil {
		return nil, totalUnreadThreadsResult.NErr
	}
	totalUnreadThreads := totalUnreadThreadsResult.Data.(int64)

	// userIds is the de-duped list of participant ids from all threads.
	userIds := []string{}
	// userIdMap is the map of participant ids from all threads.
	// Used to generate userIds
	userIdMap := map[string]bool{}

	result := &model.Threads{
		Total:               totalCount,
		Threads:             []*model.ThreadResponse{},
		TotalUnreadMentions: totalUnreadMentions,
		TotalUnreadThreads:  totalUnreadThreads,
	}

	if !opts.TotalsOnly {
		threadsResult := <-threadsChan
		if threadsResult.NErr != nil {
			return nil, threadsResult.NErr
		}
		threads := threadsResult.Data.([]*JoinedThread)
		for _, thread := range threads {
			for _, participantId := range thread.Participants {
				if _, ok := userIdMap[participantId]; !ok {
					userIdMap[participantId] = true
					userIds = append(userIds, participantId)
				}
			}
		}
		// usersMap is the global profile map of all participants from all threads.
		usersMap := make(map[string]*model.User, len(userIds))
		if opts.Extended {
			users, err := s.User().GetProfileByIds(context.Background(), userIds, &store.UserGetByIdsOpts{}, true)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get threads for user id=%s", userId)
			}
			for _, user := range users {
				usersMap[user.Id] = user
			}
		} else {
			for _, userId := range userIds {
				usersMap[userId] = &model.User{Id: userId}
			}
		}

		result.Threads = make([]*model.ThreadResponse, 0, len(threads))
		for _, thread := range threads {
			participants := make([]*model.User, 0, len(thread.Participants))
			// We get the user profiles for only a single thread filtered from the
			// global users map.
			for _, participantId := range thread.Participants {
				participant, ok := usersMap[participantId]
				if !ok {
					return nil, errors.New("cannot find thread participant with id=" + participantId)
				}
				participants = append(participants, participant)
			}
			result.Threads = append(result.Threads, &model.ThreadResponse{
				PostId:         thread.PostId,
				ReplyCount:     thread.ReplyCount,
				LastReplyAt:    thread.LastReplyAt,
				LastViewedAt:   thread.LastViewedAt,
				UnreadReplies:  thread.UnreadReplies,
				UnreadMentions: thread.UnreadMentions,
				Participants:   participants,
				Post:           thread.Post.ToNilIfInvalid(),
			})
		}
	}

	return result, nil
}

// GetTeamsUnreadForUser returns the total unread threads and unread mentions
// for a user from all teams.
func (s *SqlThreadStore) GetTeamsUnreadForUser(userID string, teamIDs []string) (map[string]*model.TeamUnread, error) {
	fetchConditions := sq.And{
		sq.Eq{"ThreadMemberships.UserId": userID},
		sq.Eq{"ThreadMemberships.Following": true},
		sq.Eq{"Channels.TeamId": teamIDs},
		sq.Eq{"COALESCE(Posts.DeleteAt, 0)": 0},
	}

	var wg sync.WaitGroup
	var err1, err2 error

	unreadThreads := []struct {
		Count  int64
		TeamId string
	}{}
	unreadMentions := []struct {
		Count  int64
		TeamId string
	}{}

	// Running these concurrently hasn't shown any major downside
	// than running them serially. So using a bit of perf boost.
	// In any case, they will be replaced by computed columns later.
	wg.Add(1)
	go func() {
		defer wg.Done()
		repliesQuery, repliesQueryArgs, err := s.getQueryBuilder().
			Select("COUNT(DISTINCT(Posts.RootId)) AS Count, TeamId").
			From("Posts").
			LeftJoin("ThreadMemberships ON Posts.RootId = ThreadMemberships.PostId").
			LeftJoin("Channels ON Posts.ChannelId = Channels.Id").
			Where(fetchConditions).
			Where("Posts.CreateAt > ThreadMemberships.LastViewed").
			GroupBy("Channels.TeamId").
			ToSql()
		if err != nil {
			err1 = errors.Wrap(err, "GetTotalUnreadThreads_Tosql")
			return
		}

		err = s.GetReplicaX().Select(&unreadThreads, repliesQuery, repliesQueryArgs...)
		if err != nil {
			err1 = errors.Wrap(err, "failed to get total unread threads")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		mentionsQuery, mentionsQueryArgs, err := s.getQueryBuilder().
			Select("COALESCE(SUM(ThreadMemberships.UnreadMentions),0) AS Count, TeamId").
			From("ThreadMemberships").
			LeftJoin("Threads ON Threads.PostId = ThreadMemberships.PostId").
			LeftJoin("Posts ON Posts.Id = ThreadMemberships.PostId").
			LeftJoin("Channels ON Threads.ChannelId = Channels.Id").
			Where(fetchConditions).
			GroupBy("Channels.TeamId").
			ToSql()
		if err != nil {
			err2 = errors.Wrap(err, "GetTotalUnreadMentions_Tosql")
		}

		err = s.GetReplicaX().Select(&unreadMentions, mentionsQuery, mentionsQueryArgs...)
		if err != nil {
			err2 = errors.Wrap(err, "failed to get total unread mentions")
		}
	}()

	// Wait for them to be over
	wg.Wait()

	if err1 != nil {
		return nil, err1
	}
	if err2 != nil {
		return nil, err2
	}

	res := make(map[string]*model.TeamUnread)
	// A bit of linear complexity here to create and return the map.
	// This makes it easy to consume the output in the app layer.
	for _, item := range unreadThreads {
		res[item.TeamId] = &model.TeamUnread{
			ThreadCount: item.Count,
		}
	}
	for _, item := range unreadMentions {
		if _, ok := res[item.TeamId]; ok {
			res[item.TeamId].ThreadMentionCount = item.Count
		} else {
			res[item.TeamId] = &model.TeamUnread{
				ThreadMentionCount: item.Count,
			}
		}
	}

	return res, nil
}

func (s *SqlThreadStore) GetThreadFollowers(threadID string, fetchOnlyActive bool) ([]string, error) {
	users := []string{}

	fetchConditions := sq.And{
		sq.Eq{"PostId": threadID},
	}

	if fetchOnlyActive {
		fetchConditions = sq.And{
			sq.Eq{"Following": true},
			fetchConditions,
		}
	}

	query, args, _ := s.getQueryBuilder().
		Select("ThreadMemberships.UserId").
		From("ThreadMemberships").
		Where(fetchConditions).
		ToSql()
	err := s.GetReplicaX().Select(&users, query, args...)

	if err != nil {
		return nil, err
	}
	return users, nil
}

func (s *SqlThreadStore) GetThreadForUser(teamId string, threadMembership *model.ThreadMembership, extended bool) (*model.ThreadResponse, error) {
	if !threadMembership.Following {
		return nil, nil // in case the thread is not followed anymore - return nil error to be interpreted as 404
	}

	type JoinedThread struct {
		PostId         string
		Following      bool
		ReplyCount     int64
		LastReplyAt    int64
		LastViewedAt   int64
		UnreadReplies  int64
		UnreadMentions int64
		Participants   model.StringArray
		model.Post
	}

	unreadRepliesQuery, unreadRepliesArgs := sq.
		Select("COUNT(Posts.Id)").
		From("Posts").
		Where(sq.And{
			sq.Eq{"Posts.RootId": threadMembership.PostId},
			sq.Gt{"Posts.CreateAt": threadMembership.LastViewed},
			sq.Eq{"Posts.DeleteAt": 0},
		}).MustSql()

	fetchConditions := sq.And{
		sq.Or{sq.Eq{"Channels.TeamId": teamId}, sq.Eq{"Channels.TeamId": ""}},
		sq.Eq{"Threads.PostId": threadMembership.PostId},
	}

	var thread JoinedThread
	query, threadArgs, _ := s.getQueryBuilder().
		Select("Threads.*, Posts.*").
		From("Threads").
		Column(sq.Alias(sq.Expr(unreadRepliesQuery), "UnreadReplies")).
		LeftJoin("Posts ON Posts.Id = Threads.PostId").
		LeftJoin("Channels ON Posts.ChannelId = Channels.Id").
		Where(fetchConditions).ToSql()

	args := append(unreadRepliesArgs, threadArgs...)

	err := s.GetReplicaX().Get(&thread, query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, store.NewErrNotFound("Thread", threadMembership.PostId)
		}
		return nil, err
	}

	thread.LastViewedAt = threadMembership.LastViewed
	thread.UnreadMentions = threadMembership.UnreadMentions

	users := []*model.User{}
	if extended {
		var err error
		users, err = s.User().GetProfileByIds(context.Background(), thread.Participants, &store.UserGetByIdsOpts{}, true)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get thread for user id=%s", threadMembership.UserId)
		}
	} else {
		for _, userId := range thread.Participants {
			users = append(users, &model.User{Id: userId})
		}
	}

	participants := []*model.User{}
	for _, participantId := range thread.Participants {
		var participant *model.User
		for _, u := range users {
			if u.Id == participantId {
				participant = u
				break
			}
		}
		if participant != nil {
			participants = append(participants, participant)
		}
	}

	result := &model.ThreadResponse{
		PostId:         thread.PostId,
		ReplyCount:     thread.ReplyCount,
		LastReplyAt:    thread.LastReplyAt,
		LastViewedAt:   thread.LastViewedAt,
		UnreadReplies:  thread.UnreadReplies,
		UnreadMentions: thread.UnreadMentions,
		Participants:   participants,
		Post:           thread.Post.ToNilIfInvalid(),
	}

	return result, nil
}

// MarkAllAsReadInChannels marks all threads for the given user in the given channels as read from
// the current time.
func (s *SqlThreadStore) MarkAllAsReadInChannels(userID string, channelIDs []string) error {
	threadIDs := []string{}

	query, args, _ := s.getQueryBuilder().
		Select("ThreadMemberships.PostId").
		Join("Threads ON Threads.PostId = ThreadMemberships.PostId").
		Join("Channels ON Threads.ChannelId = Channels.Id").
		From("ThreadMemberships").
		Where(sq.Eq{"Threads.ChannelId": channelIDs}).
		Where(sq.Eq{"ThreadMemberships.UserId": userID}).
		ToSql()

	err := s.GetReplicaX().Select(&threadIDs, query, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to get thread membership with userid=%s", userID)
	}

	timestamp := model.GetMillis()
	query, args, _ = s.getQueryBuilder().
		Update("ThreadMemberships").
		Where(sq.Eq{"PostId": threadIDs}).
		Where(sq.Eq{"UserId": userID}).
		Set("LastViewed", timestamp).
		Set("UnreadMentions", 0).
		Set("LastUpdated", model.GetMillis()).
		ToSql()
	if _, err := s.GetMasterX().Exec(query, args...); err != nil {
		return errors.Wrapf(err, "failed to update thread read state for user id=%s", userID)
	}
	return nil

}

// MarkAllAsRead marks all threads for the given user in the given team as read from the current
// time.
func (s *SqlThreadStore) MarkAllAsRead(userId, teamId string) error {
	memberships, err := s.GetMembershipsForUser(userId, teamId)
	if err != nil {
		return err
	}
	membershipIds := []string{}
	for _, m := range memberships {
		membershipIds = append(membershipIds, m.PostId)
	}
	timestamp := model.GetMillis()
	query, args, _ := s.getQueryBuilder().
		Update("ThreadMemberships").
		Where(sq.Eq{"PostId": membershipIds}).
		Where(sq.Eq{"UserId": userId}).
		Set("LastViewed", timestamp).
		Set("UnreadMentions", 0).
		Set("LastUpdated", model.GetMillis()).
		ToSql()
	if _, err := s.GetMasterX().Exec(query, args...); err != nil {
		return errors.Wrapf(err, "failed to update thread read state for user id=%s", userId)
	}
	return nil
}

// MarkAsRead marks the given thread for the given user as unread from the given timestamp.
func (s *SqlThreadStore) MarkAsRead(userId, threadId string, timestamp int64) error {
	query, args, _ := s.getQueryBuilder().
		Update("ThreadMemberships").
		Where(sq.Eq{"UserId": userId}).
		Where(sq.Eq{"PostId": threadId}).
		Set("LastViewed", timestamp).
		Set("LastUpdated", model.GetMillis()).
		ToSql()
	if _, err := s.GetMasterX().Exec(query, args...); err != nil {
		return errors.Wrapf(err, "failed to update thread read state for user id=%s thread_id=%v", userId, threadId)
	}
	return nil
}

func (s *SqlThreadStore) saveMembership(ex sqlxExecutor, membership *model.ThreadMembership) (*model.ThreadMembership, error) {
	query, args, err := s.getQueryBuilder().
		Insert("ThreadMemberships").
		Columns("PostId", "UserId", "Following", "LastViewed", "LastUpdated", "UnreadMentions").
		Values(membership.PostId, membership.UserId, membership.Following, membership.LastViewed, membership.LastUpdated, membership.UnreadMentions).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "threadmembership_tosql")
	}
	if _, err := ex.Exec(query, args...); err != nil {
		return nil, errors.Wrapf(err, "failed to save thread membership with postid=%s userid=%s", membership.PostId, membership.UserId)
	}

	return membership, nil
}

func (s *SqlThreadStore) UpdateMembership(membership *model.ThreadMembership) (*model.ThreadMembership, error) {
	return s.updateMembership(s.GetMasterX(), membership)
}

func (s *SqlThreadStore) updateMembership(ex sqlxExecutor, membership *model.ThreadMembership) (*model.ThreadMembership, error) {
	query, args, err := s.getQueryBuilder().
		Update("ThreadMemberships").
		Set("Following", membership.Following).
		Set("LastViewed", membership.LastViewed).
		Set("LastUpdated", membership.LastUpdated).
		Set("UnreadMentions", membership.UnreadMentions).
		Where(sq.And{
			sq.Eq{"PostId": membership.PostId},
			sq.Eq{"UserId": membership.UserId},
		}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "threadmembership_tosql")
	}
	if _, err := ex.Exec(query, args...); err != nil {
		return nil, errors.Wrapf(err, "failed to update thread membership with postid=%s userid=%s", membership.PostId, membership.UserId)
	}

	return membership, nil
}

func (s *SqlThreadStore) GetMembershipsForUser(userId, teamId string) ([]*model.ThreadMembership, error) {
	memberships := []*model.ThreadMembership{}

	query, args, _ := s.getQueryBuilder().
		Select("ThreadMemberships.*").
		Join("Threads ON Threads.PostId = ThreadMemberships.PostId").
		Join("Channels ON Threads.ChannelId = Channels.Id").
		From("ThreadMemberships").
		Where(sq.Or{sq.Eq{"Channels.TeamId": teamId}, sq.Eq{"Channels.TeamId": ""}}).
		Where(sq.Eq{"ThreadMemberships.UserId": userId}).
		ToSql()

	err := s.GetReplicaX().Select(&memberships, query, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get thread membership with userid=%s", userId)
	}
	return memberships, nil
}

func (s *SqlThreadStore) GetMembershipForUser(userId, postId string) (*model.ThreadMembership, error) {
	return s.getMembershipForUser(s.GetReplicaX(), userId, postId)
}

func (s *SqlThreadStore) getMembershipForUser(ex sqlxExecutor, userId, postId string) (*model.ThreadMembership, error) {
	var membership model.ThreadMembership
	query, args, err := s.getQueryBuilder().
		Select("*").
		From("ThreadMemberships").
		Where(sq.And{
			sq.Eq{"PostId": postId},
			sq.Eq{"UserId": userId},
		}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "threadmembership_tosql")
	}
	err = ex.Get(&membership, query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, store.NewErrNotFound("Thread", postId)
		}
		return nil, errors.Wrapf(err, "failed to get thread membership with userid=%s postid=%s", userId, postId)
	}
	return &membership, nil
}

func (s *SqlThreadStore) DeleteMembershipForUser(userId string, postId string) error {
	query, args, err := s.getQueryBuilder().
		Delete("ThreadMemberships").
		Where(sq.And{
			sq.Eq{"PostId": postId},
			sq.Eq{"UserId": userId},
		}).
		ToSql()
	if err != nil {
		return errors.Wrap(err, "threadmembership_tosql")
	}
	if _, err := s.GetMasterX().Exec(query, args...); err != nil {
		return errors.Wrap(err, "failed to delete thread membership")
	}

	return nil
}

// MaintainMembership creates or updates a thread membership for the given user
// and post. This method is used to update the state of a membership in response
// to some events like:
// - post creation (mentions handling)
// - channel marked unread
// - user explicitly following a thread
func (s *SqlThreadStore) MaintainMembership(userId, postId string, opts store.ThreadMembershipOpts) (*model.ThreadMembership, error) {
	trx, err := s.GetMasterX().Beginx()
	if err != nil {
		return nil, errors.Wrap(err, "begin_transaction")
	}
	defer finalizeTransactionX(trx)

	membership, err := s.getMembershipForUser(trx, userId, postId)
	now := utils.MillisFromTime(time.Now())
	// if membership exists, update it if:
	// a. user started/stopped following a thread
	// b. mention count changed
	// c. user viewed a thread
	if err == nil {
		followingNeedsUpdate := (opts.UpdateFollowing && (membership.Following != opts.Following))
		if followingNeedsUpdate || opts.IncrementMentions || opts.UpdateViewedTimestamp {
			if followingNeedsUpdate {
				membership.Following = opts.Following
			}
			if opts.UpdateViewedTimestamp {
				membership.LastViewed = now
				membership.UnreadMentions = 0
			} else if opts.IncrementMentions {
				membership.UnreadMentions += 1
			}
			membership.LastUpdated = now
			if _, err = s.updateMembership(trx, membership); err != nil {
				return nil, err
			}
		}

		if err = trx.Commit(); err != nil {
			return nil, errors.Wrap(err, "commit_transaction")
		}

		return membership, err
	}

	var nfErr *store.ErrNotFound
	if !errors.As(err, &nfErr) {
		return nil, errors.Wrap(err, "failed to get thread membership")
	}

	membership = &model.ThreadMembership{
		PostId:      postId,
		UserId:      userId,
		Following:   opts.Following,
		LastUpdated: now,
	}
	if opts.IncrementMentions {
		membership.UnreadMentions = 1
	}
	if opts.UpdateViewedTimestamp {
		membership.LastViewed = now
	}
	membership, err = s.saveMembership(trx, membership)
	if err != nil {
		return nil, err
	}

	if opts.UpdateParticipants {
		if s.DriverName() == model.DatabaseDriverPostgres {
			if _, err2 := trx.ExecRaw(`UPDATE Threads
                        SET participants = participants || $1::jsonb
                        WHERE postid=$2
                        AND NOT participants ? $3`, jsonArray([]string{userId}), postId, userId); err2 != nil {
				return nil, err2
			}
		} else {
			// CONCAT('$[', JSON_LENGTH(Participants), ']') just generates $[n]
			// which is the positional syntax required for appending.
			if _, err2 := trx.Exec(`UPDATE Threads
				SET Participants = JSON_ARRAY_INSERT(Participants, CONCAT('$[', JSON_LENGTH(Participants), ']'), ?)
				WHERE PostId=?
				AND NOT JSON_CONTAINS(Participants, ?)`, userId, postId, strconv.Quote(userId)); err2 != nil {
				return nil, err2
			}
		}
	}

	if err = trx.Commit(); err != nil {
		return nil, errors.Wrap(err, "commit_transaction")
	}

	return membership, err
}

func (s *SqlThreadStore) CollectThreadsWithNewerReplies(userId string, channelIds []string, timestamp int64) ([]string, error) {
	changedThreads := []string{}
	query, args, _ := s.getQueryBuilder().
		Select("Threads.PostId").
		From("Threads").
		LeftJoin("ChannelMembers ON ChannelMembers.ChannelId=Threads.ChannelId").
		Where(sq.And{
			sq.Eq{"Threads.ChannelId": channelIds},
			sq.Eq{"ChannelMembers.UserId": userId},
			sq.Or{
				sq.Expr("Threads.LastReplyAt > ChannelMembers.LastViewedAt"),
				sq.Gt{"Threads.LastReplyAt": timestamp},
			},
		}).
		ToSql()
	if err := s.GetReplicaX().Select(&changedThreads, query, args...); err != nil {
		return nil, errors.Wrap(err, "failed to fetch threads")
	}
	return changedThreads, nil
}

// UpdateLastViewedByThreadIds marks the given threads as read up to the given timestamp. If there
// are no newer posts, it effectively marks the thread as read. If there are newer posts, say
// because the user explicitly marked a past post as unread, the thread will be considered unread
// past the given timestamp.
func (s *SqlThreadStore) UpdateLastViewedByThreadIds(userId string, threadIds []string, timestamp int64) error {
	if len(threadIds) == 0 {
		return nil
	}

	qb := s.getQueryBuilder().
		Update("ThreadMemberships").
		Where(sq.Eq{"UserId": userId, "PostId": threadIds}).
		Set("LastViewed", timestamp).
		Set("LastUpdated", model.GetMillis())
	updateQuery, updateArgs, _ := qb.ToSql()

	if _, err := s.GetMasterX().Exec(updateQuery, updateArgs...); err != nil {
		return errors.Wrap(err, "failed to update thread membership")
	}

	return nil
}

func (s *SqlThreadStore) GetPosts(threadId string, since int64) ([]*model.Post, error) {
	query, args, _ := s.getQueryBuilder().
		Select("*").
		From("Posts").
		Where(sq.Eq{"RootId": threadId}).
		Where(sq.Eq{"DeleteAt": 0}).
		Where(sq.GtOrEq{"UpdateAt": since}).ToSql()
	result := []*model.Post{}
	if err := s.GetReplicaX().Select(&result, query, args...); err != nil {
		return nil, errors.Wrap(err, "failed to fetch thread posts")
	}
	return result, nil
}

// PermanentDeleteBatchForRetentionPolicies deletes a batch of records which are affected by
// the global or a granular retention policy.
// See `genericPermanentDeleteBatchForRetentionPolicies` for details.
func (s *SqlThreadStore) PermanentDeleteBatchForRetentionPolicies(now, globalPolicyEndTime, limit int64, cursor model.RetentionPolicyCursor) (int64, model.RetentionPolicyCursor, error) {
	builder := s.getQueryBuilder().
		Select("Threads.PostId").
		From("Threads")
	return genericPermanentDeleteBatchForRetentionPolicies(RetentionPolicyBatchDeletionInfo{
		BaseBuilder:         builder,
		Table:               "Threads",
		TimeColumn:          "LastReplyAt",
		PrimaryKeys:         []string{"PostId"},
		ChannelIDTable:      "Threads",
		NowMillis:           now,
		GlobalPolicyEndTime: globalPolicyEndTime,
		Limit:               limit,
	}, s.SqlStore, cursor)
}

// PermanentDeleteBatchThreadMembershipsForRetentionPolicies deletes a batch of records
// which are affected by the global or a granular retention policy.
// See `genericPermanentDeleteBatchForRetentionPolicies` for details.
func (s *SqlThreadStore) PermanentDeleteBatchThreadMembershipsForRetentionPolicies(now, globalPolicyEndTime, limit int64, cursor model.RetentionPolicyCursor) (int64, model.RetentionPolicyCursor, error) {
	builder := s.getQueryBuilder().
		Select("ThreadMemberships.PostId").
		From("ThreadMemberships").
		InnerJoin("Threads ON ThreadMemberships.PostId = Threads.PostId")
	return genericPermanentDeleteBatchForRetentionPolicies(RetentionPolicyBatchDeletionInfo{
		BaseBuilder:         builder,
		Table:               "ThreadMemberships",
		TimeColumn:          "LastUpdated",
		PrimaryKeys:         []string{"PostId"},
		ChannelIDTable:      "Threads",
		NowMillis:           now,
		GlobalPolicyEndTime: globalPolicyEndTime,
		Limit:               limit,
	}, s.SqlStore, cursor)
}

// DeleteOrphanedRows removes orphaned rows from Threads and ThreadMemberships
func (s *SqlThreadStore) DeleteOrphanedRows(limit int) (deleted int64, err error) {
	// We need the extra level of nesting to deal with MySQL's locking
	const threadsQuery = `
	DELETE FROM Threads WHERE PostId IN (
		SELECT * FROM (
			SELECT Threads.PostId FROM Threads
			LEFT JOIN Channels ON Threads.ChannelId = Channels.Id
			WHERE Channels.Id IS NULL
			LIMIT ?
		) AS A
	)`
	// We only delete a thread membership if the entire thread no longer exists,
	// not if the root post has been deleted
	const threadMembershipsQuery = `
	DELETE FROM ThreadMemberships WHERE PostId IN (
		SELECT * FROM (
			SELECT ThreadMemberships.PostId FROM ThreadMemberships
			LEFT JOIN Threads ON ThreadMemberships.PostId = Threads.PostId
			WHERE Threads.PostId IS NULL
			LIMIT ?
		) AS A
	)`
	result, err := s.GetMasterX().Exec(threadsQuery, limit)
	if err != nil {
		return
	}
	rpcDeleted, err := result.RowsAffected()
	if err != nil {
		return
	}
	result, err = s.GetMasterX().Exec(threadMembershipsQuery, limit)
	if err != nil {
		return
	}
	rptDeleted, err := result.RowsAffected()
	if err != nil {
		return
	}
	deleted = rpcDeleted + rptDeleted
	return
}

// return number of unread replies for a single thread
func (s *SqlThreadStore) GetThreadUnreadReplyCount(threadMembership *model.ThreadMembership) (unreadReplies int64, err error) {
	query, args := s.getQueryBuilder().
		Select("COUNT(Posts.Id)").
		From("Posts").
		Where(sq.And{
			sq.Eq{"Posts.RootId": threadMembership.PostId},
			sq.Gt{"Posts.CreateAt": threadMembership.LastViewed},
			sq.Eq{"Posts.DeleteAt": 0},
		}).MustSql()

	err = s.GetReplicaX().Get(&unreadReplies, query, args...)

	if err != nil {
		return
	}

	return
}
