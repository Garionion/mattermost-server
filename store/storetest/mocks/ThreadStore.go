// Code generated by mockery v1.0.0. DO NOT EDIT.

// Regenerate this file using `make store-mocks`.

package mocks

import (
	model "github.com/mattermost/mattermost-server/v6/model"
	store "github.com/mattermost/mattermost-server/v6/store"
	mock "github.com/stretchr/testify/mock"
)

// ThreadStore is an autogenerated mock type for the ThreadStore type
type ThreadStore struct {
	mock.Mock
}

// CollectThreadsWithNewerReplies provides a mock function with given fields: userId, channelIds, timestamp
func (_m *ThreadStore) CollectThreadsWithNewerReplies(userId string, channelIds []string, timestamp int64) ([]string, error) {
	ret := _m.Called(userId, channelIds, timestamp)

	var r0 []string
	if rf, ok := ret.Get(0).(func(string, []string, int64) []string); ok {
		r0 = rf(userId, channelIds, timestamp)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, []string, int64) error); ok {
		r1 = rf(userId, channelIds, timestamp)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteMembershipForUser provides a mock function with given fields: userId, postID
func (_m *ThreadStore) DeleteMembershipForUser(userId string, postID string) error {
	ret := _m.Called(userId, postID)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(userId, postID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeleteOrphanedRows provides a mock function with given fields: limit
func (_m *ThreadStore) DeleteOrphanedRows(limit int) (int64, error) {
	ret := _m.Called(limit)

	var r0 int64
	if rf, ok := ret.Get(0).(func(int) int64); ok {
		r0 = rf(limit)
	} else {
		r0 = ret.Get(0).(int64)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(int) error); ok {
		r1 = rf(limit)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Get provides a mock function with given fields: id
func (_m *ThreadStore) Get(id string) (*model.Thread, error) {
	ret := _m.Called(id)

	var r0 *model.Thread
	if rf, ok := ret.Get(0).(func(string) *model.Thread); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Thread)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetMembershipForUser provides a mock function with given fields: userId, postID
func (_m *ThreadStore) GetMembershipForUser(userId string, postID string) (*model.ThreadMembership, error) {
	ret := _m.Called(userId, postID)

	var r0 *model.ThreadMembership
	if rf, ok := ret.Get(0).(func(string, string) *model.ThreadMembership); ok {
		r0 = rf(userId, postID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.ThreadMembership)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(userId, postID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetMembershipsForUser provides a mock function with given fields: userId, teamID
func (_m *ThreadStore) GetMembershipsForUser(userId string, teamID string) ([]*model.ThreadMembership, error) {
	ret := _m.Called(userId, teamID)

	var r0 []*model.ThreadMembership
	if rf, ok := ret.Get(0).(func(string, string) []*model.ThreadMembership); ok {
		r0 = rf(userId, teamID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.ThreadMembership)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(userId, teamID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetPosts provides a mock function with given fields: threadID, since
func (_m *ThreadStore) GetPosts(threadID string, since int64) ([]*model.Post, error) {
	ret := _m.Called(threadID, since)

	var r0 []*model.Post
	if rf, ok := ret.Get(0).(func(string, int64) []*model.Post); ok {
		r0 = rf(threadID, since)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.Post)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, int64) error); ok {
		r1 = rf(threadID, since)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetTeamsUnreadForUser provides a mock function with given fields: userID, teamIDs
func (_m *ThreadStore) GetTeamsUnreadForUser(userID string, teamIDs []string) (map[string]*model.TeamUnread, error) {
	ret := _m.Called(userID, teamIDs)

	var r0 map[string]*model.TeamUnread
	if rf, ok := ret.Get(0).(func(string, []string) map[string]*model.TeamUnread); ok {
		r0 = rf(userID, teamIDs)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]*model.TeamUnread)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, []string) error); ok {
		r1 = rf(userID, teamIDs)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetThreadFollowers provides a mock function with given fields: threadID, fetchOnlyActive
func (_m *ThreadStore) GetThreadFollowers(threadID string, fetchOnlyActive bool) ([]string, error) {
	ret := _m.Called(threadID, fetchOnlyActive)

	var r0 []string
	if rf, ok := ret.Get(0).(func(string, bool) []string); ok {
		r0 = rf(threadID, fetchOnlyActive)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, bool) error); ok {
		r1 = rf(threadID, fetchOnlyActive)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetThreadForUser provides a mock function with given fields: teamID, threadMembership, extended
func (_m *ThreadStore) GetThreadForUser(teamID string, threadMembership *model.ThreadMembership, extended bool) (*model.ThreadResponse, error) {
	ret := _m.Called(teamID, threadMembership, extended)

	var r0 *model.ThreadResponse
	if rf, ok := ret.Get(0).(func(string, *model.ThreadMembership, bool) *model.ThreadResponse); ok {
		r0 = rf(teamID, threadMembership, extended)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.ThreadResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, *model.ThreadMembership, bool) error); ok {
		r1 = rf(teamID, threadMembership, extended)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetThreadUnreadReplyCount provides a mock function with given fields: threadMembership
func (_m *ThreadStore) GetThreadUnreadReplyCount(threadMembership *model.ThreadMembership) (int64, error) {
	ret := _m.Called(threadMembership)

	var r0 int64
	if rf, ok := ret.Get(0).(func(*model.ThreadMembership) int64); ok {
		r0 = rf(threadMembership)
	} else {
		r0 = ret.Get(0).(int64)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(*model.ThreadMembership) error); ok {
		r1 = rf(threadMembership)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetThreadsForUser provides a mock function with given fields: userId, teamID, opts
func (_m *ThreadStore) GetThreadsForUser(userId string, teamID string, opts model.GetUserThreadsOpts) (*model.Threads, error) {
	ret := _m.Called(userId, teamID, opts)

	var r0 *model.Threads
	if rf, ok := ret.Get(0).(func(string, string, model.GetUserThreadsOpts) *model.Threads); ok {
		r0 = rf(userId, teamID, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Threads)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, model.GetUserThreadsOpts) error); ok {
		r1 = rf(userId, teamID, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MaintainMembership provides a mock function with given fields: userID, postID, opts
func (_m *ThreadStore) MaintainMembership(userID string, postID string, opts store.ThreadMembershipOpts) (*model.ThreadMembership, error) {
	ret := _m.Called(userID, postID, opts)

	var r0 *model.ThreadMembership
	if rf, ok := ret.Get(0).(func(string, string, store.ThreadMembershipOpts) *model.ThreadMembership); ok {
		r0 = rf(userID, postID, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.ThreadMembership)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, store.ThreadMembershipOpts) error); ok {
		r1 = rf(userID, postID, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MarkAllAsRead provides a mock function with given fields: userID, teamID
func (_m *ThreadStore) MarkAllAsRead(userID string, teamID string) error {
	ret := _m.Called(userID, teamID)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(userID, teamID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MarkAllAsReadInChannels provides a mock function with given fields: userID, channelIDs
func (_m *ThreadStore) MarkAllAsReadInChannels(userID string, channelIDs []string) error {
	ret := _m.Called(userID, channelIDs)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, []string) error); ok {
		r0 = rf(userID, channelIDs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MarkAsRead provides a mock function with given fields: userID, threadID, timestamp
func (_m *ThreadStore) MarkAsRead(userID string, threadID string, timestamp int64) error {
	ret := _m.Called(userID, threadID, timestamp)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, int64) error); ok {
		r0 = rf(userID, threadID, timestamp)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// PermanentDeleteBatchForRetentionPolicies provides a mock function with given fields: now, globalPolicyEndTime, limit, cursor
func (_m *ThreadStore) PermanentDeleteBatchForRetentionPolicies(now int64, globalPolicyEndTime int64, limit int64, cursor model.RetentionPolicyCursor) (int64, model.RetentionPolicyCursor, error) {
	ret := _m.Called(now, globalPolicyEndTime, limit, cursor)

	var r0 int64
	if rf, ok := ret.Get(0).(func(int64, int64, int64, model.RetentionPolicyCursor) int64); ok {
		r0 = rf(now, globalPolicyEndTime, limit, cursor)
	} else {
		r0 = ret.Get(0).(int64)
	}

	var r1 model.RetentionPolicyCursor
	if rf, ok := ret.Get(1).(func(int64, int64, int64, model.RetentionPolicyCursor) model.RetentionPolicyCursor); ok {
		r1 = rf(now, globalPolicyEndTime, limit, cursor)
	} else {
		r1 = ret.Get(1).(model.RetentionPolicyCursor)
	}

	var r2 error
	if rf, ok := ret.Get(2).(func(int64, int64, int64, model.RetentionPolicyCursor) error); ok {
		r2 = rf(now, globalPolicyEndTime, limit, cursor)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// PermanentDeleteBatchThreadMembershipsForRetentionPolicies provides a mock function with given fields: now, globalPolicyEndTime, limit, cursor
func (_m *ThreadStore) PermanentDeleteBatchThreadMembershipsForRetentionPolicies(now int64, globalPolicyEndTime int64, limit int64, cursor model.RetentionPolicyCursor) (int64, model.RetentionPolicyCursor, error) {
	ret := _m.Called(now, globalPolicyEndTime, limit, cursor)

	var r0 int64
	if rf, ok := ret.Get(0).(func(int64, int64, int64, model.RetentionPolicyCursor) int64); ok {
		r0 = rf(now, globalPolicyEndTime, limit, cursor)
	} else {
		r0 = ret.Get(0).(int64)
	}

	var r1 model.RetentionPolicyCursor
	if rf, ok := ret.Get(1).(func(int64, int64, int64, model.RetentionPolicyCursor) model.RetentionPolicyCursor); ok {
		r1 = rf(now, globalPolicyEndTime, limit, cursor)
	} else {
		r1 = ret.Get(1).(model.RetentionPolicyCursor)
	}

	var r2 error
	if rf, ok := ret.Get(2).(func(int64, int64, int64, model.RetentionPolicyCursor) error); ok {
		r2 = rf(now, globalPolicyEndTime, limit, cursor)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// UpdateLastViewedByThreadIds provides a mock function with given fields: userId, threadIds, timestamp
func (_m *ThreadStore) UpdateLastViewedByThreadIds(userId string, threadIds []string, timestamp int64) error {
	ret := _m.Called(userId, threadIds, timestamp)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, []string, int64) error); ok {
		r0 = rf(userId, threadIds, timestamp)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdateMembership provides a mock function with given fields: membership
func (_m *ThreadStore) UpdateMembership(membership *model.ThreadMembership) (*model.ThreadMembership, error) {
	ret := _m.Called(membership)

	var r0 *model.ThreadMembership
	if rf, ok := ret.Get(0).(func(*model.ThreadMembership) *model.ThreadMembership); ok {
		r0 = rf(membership)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.ThreadMembership)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(*model.ThreadMembership) error); ok {
		r1 = rf(membership)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
