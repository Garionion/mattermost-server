// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/mattermost/mattermost-server/v6/app"
	"github.com/mattermost/mattermost-server/v6/audit"
	"github.com/mattermost/mattermost-server/v6/model"
)

func (api *API) InitGroup() {
	// GET /api/v4/groups
	api.BaseRoutes.Groups.Handle("", api.APISessionRequired(requireLicense(getGroups))).Methods("GET")

	// POST /api/v4/groups
	api.BaseRoutes.Groups.Handle("", api.APISessionRequired(requireLicense(createGroup))).Methods("POST")

	// GET /api/v4/groups/:group_id
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}",
		api.APISessionRequired(requireLicense(getGroup))).Methods("GET")

	// PUT /api/v4/groups/:group_id/patch
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/patch",
		api.APISessionRequired(requireLicense(patchGroup))).Methods("PUT")

	// POST /api/v4/groups/:group_id/teams/:team_id/link
	// POST /api/v4/groups/:group_id/channels/:channel_id/link
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/{syncable_type:teams|channels}/{syncable_id:[A-Za-z0-9]+}/link",
		api.APISessionRequired(requireLicense(linkGroupSyncable))).Methods("POST")

	// DELETE /api/v4/groups/:group_id/teams/:team_id/link
	// DELETE /api/v4/groups/:group_id/channels/:channel_id/link
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/{syncable_type:teams|channels}/{syncable_id:[A-Za-z0-9]+}/link",
		api.APISessionRequired(requireLicense(unlinkGroupSyncable))).Methods("DELETE")

	// GET /api/v4/groups/:group_id/teams/:team_id
	// GET /api/v4/groups/:group_id/channels/:channel_id
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/{syncable_type:teams|channels}/{syncable_id:[A-Za-z0-9]+}",
		api.APISessionRequired(requireLicense(getGroupSyncable))).Methods("GET")

	// GET /api/v4/groups/:group_id/teams
	// GET /api/v4/groups/:group_id/channels
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/{syncable_type:teams|channels}",
		api.APISessionRequired(requireLicense(getGroupSyncables))).Methods("GET")

	// PUT /api/v4/groups/:group_id/teams/:team_id/patch
	// PUT /api/v4/groups/:group_id/channels/:channel_id/patch
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/{syncable_type:teams|channels}/{syncable_id:[A-Za-z0-9]+}/patch",
		api.APISessionRequired(requireLicense(patchGroupSyncable))).Methods("PUT")

	// GET /api/v4/groups/:group_id/stats
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/stats",
		api.APISessionRequired(requireLicense(getGroupStats))).Methods("GET")

	// GET /api/v4/groups/:group_id/members
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/members",
		api.APISessionRequired(requireLicense(getGroupMembers))).Methods("GET")

	// GET /api/v4/users/:user_id/groups
	api.BaseRoutes.Users.Handle("/{user_id:[A-Za-z0-9]+}/groups",
		api.APISessionRequired(requireLicense(getGroupsByUserId))).Methods("GET")

	// GET /api/v4/channels/:channel_id/groups
	api.BaseRoutes.Channels.Handle("/{channel_id:[A-Za-z0-9]+}/groups",
		api.APISessionRequired(requireLicense(getGroupsByChannel))).Methods("GET")

	// GET /api/v4/teams/:team_id/groups
	api.BaseRoutes.Teams.Handle("/{team_id:[A-Za-z0-9]+}/groups",
		api.APISessionRequired(requireLicense(getGroupsByTeam))).Methods("GET")

	// GET /api/v4/teams/:team_id/groups_by_channels
	api.BaseRoutes.Teams.Handle("/{team_id:[A-Za-z0-9]+}/groups_by_channels",
		api.APISessionRequired(requireLicense(getGroupsAssociatedToChannelsByTeam))).Methods("GET")

	// DELETE /api/v4/groups/:group_id
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}",
		api.APISessionRequired(requireLicense(deleteGroup))).Methods("DELETE")

	// POST /api/v4/groups/:group_id/members
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/members",
		api.APISessionRequired(requireLicense(addGroupMembers))).Methods("POST")

	// DELETE /api/v4/groups/:group_id/members
	api.BaseRoutes.Groups.Handle("/{group_id:[A-Za-z0-9]+}/members",
		api.APISessionRequired(requireLicense(deleteGroupMembers))).Methods("DELETE")
}

func getGroup(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	group, err := c.App.GetGroup(c.Params.GroupId, &model.GetGroupOpts{
		IncludeMemberCount: c.Params.IncludeMemberCount,
	})
	if err != nil {
		c.Err = err
		return
	}

	if group.Source == model.GroupSourceLdap {
		if !c.App.SessionHasPermissionToGroup(*c.AppContext.Session(), c.Params.GroupId, model.PermissionSysconsoleReadUserManagementGroups) {
			c.SetPermissionError(model.PermissionSysconsoleReadUserManagementGroups)
			return
		}
	}

	if lcErr := licensedAndConfiguredForGroupBySource(c.App, group.Source); lcErr != nil {
		lcErr.Where = "Api4.getGroup"
		c.Err = lcErr
		return
	}

	b, marshalErr := json.Marshal(group)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroup", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func createGroup(c *Context, w http.ResponseWriter, r *http.Request) {
	var group *model.GroupWithUserIds
	if jsonErr := json.NewDecoder(r.Body).Decode(&group); jsonErr != nil {
		c.SetInvalidParam("group")
		return
	}

	if group.Source != model.GroupSourceCustom {
		c.Err = model.NewAppError("createGroup", "app.group.crud_permission", nil, "", http.StatusNotImplemented)
		return
	}

	if lcErr := licensedAndConfiguredForGroupBySource(c.App, group.Source); lcErr != nil {
		lcErr.Where = "Api4.createGroup"
		c.Err = lcErr
		return
	}

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionCreateCustomGroup) {
		c.SetPermissionError(model.PermissionCreateCustomGroup)
		return
	}

	if !group.AllowReference {
		c.Err = model.NewAppError("createGroup", "api.custom_groups.must_be_referenceable", nil, "", http.StatusNotImplemented)
		return
	}

	if group.GetRemoteId() != "" {
		c.Err = model.NewAppError("createGroup", "api.custom_groups.no_remote_id", nil, "", http.StatusNotImplemented)
		return
	}

	auditRec := c.MakeAuditRecord("createGroup", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("group", group)

	newGroup, err := c.App.CreateGroupWithUserIds(group)
	if err != nil {
		c.Err = err
		return
	}

	auditRec.AddMeta("group", newGroup)
	js, jsonErr := json.Marshal(newGroup)
	if jsonErr != nil {
		c.Err = model.NewAppError("createGroup", "api.marshal_error", nil, jsonErr.Error(), http.StatusInternalServerError)
		return
	}
	auditRec.Success()
	w.WriteHeader(http.StatusCreated)
	w.Write(js)
}

func patchGroup(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	group, err := c.App.GetGroup(c.Params.GroupId, nil)
	if err != nil {
		c.Err = err
		return
	}

	if lcErr := licensedAndConfiguredForGroupBySource(c.App, group.Source); lcErr != nil {
		lcErr.Where = "Api4.patchGroup"
		c.Err = lcErr
		return
	}

	var requiredPermission *model.Permission
	if group.Source == model.GroupSourceCustom {
		requiredPermission = model.PermissionEditCustomGroup
	} else {
		requiredPermission = model.PermissionSysconsoleWriteUserManagementGroups
	}
	if !c.App.SessionHasPermissionToGroup(*c.AppContext.Session(), c.Params.GroupId, requiredPermission) {
		c.SetPermissionError(requiredPermission)
		return
	}

	var groupPatch model.GroupPatch
	if jsonErr := json.NewDecoder(r.Body).Decode(&groupPatch); jsonErr != nil {
		c.SetInvalidParam("group")
		return
	}

	if group.Source == model.GroupSourceCustom && groupPatch.AllowReference != nil && !*groupPatch.AllowReference {
		c.Err = model.NewAppError("Api4.patchGroup", "api.custom_groups.must_be_referenceable", nil, "", http.StatusBadRequest)
		return
	}

	auditRec := c.MakeAuditRecord("patchGroup", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("group", group)

	if groupPatch.AllowReference != nil && *groupPatch.AllowReference {
		if groupPatch.Name == nil {
			tmp := strings.ReplaceAll(strings.ToLower(group.DisplayName), " ", "-")
			groupPatch.Name = &tmp
		} else {
			if *groupPatch.Name == model.UserNotifyAll || *groupPatch.Name == model.ChannelMentionsNotifyProp || *groupPatch.Name == model.UserNotifyHere {
				c.Err = model.NewAppError("Api4.patchGroup", "api.ldap_groups.existing_reserved_name_error", nil, "", http.StatusNotImplemented)
				return
			}
			//check if a user already has this group name
			user, _ := c.App.GetUserByUsername(*groupPatch.Name)
			if user != nil {
				c.Err = model.NewAppError("Api4.patchGroup", "api.ldap_groups.existing_user_name_error", nil, "", http.StatusNotImplemented)
				return
			}
			//check if a mentionable group already has this name
			searchOpts := model.GroupSearchOpts{
				FilterAllowReference: true,
			}
			existingGroup, _ := c.App.GetGroupByName(*groupPatch.Name, searchOpts)
			if existingGroup != nil {
				c.Err = model.NewAppError("Api4.patchGroup", "api.ldap_groups.existing_group_name_error", nil, "", http.StatusNotImplemented)
				return
			}
		}
	}

	group.Patch(&groupPatch)

	group, err = c.App.UpdateGroup(group)
	if err != nil {
		c.Err = err
		return
	}
	auditRec.AddMeta("patch", group)

	b, marshalErr := json.Marshal(group)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.patchGroup", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	auditRec.Success()
	w.Write(b)
}

func linkGroupSyncable(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	c.RequireSyncableId()
	if c.Err != nil {
		return
	}
	syncableID := c.Params.SyncableId

	c.RequireSyncableType()
	if c.Err != nil {
		return
	}
	syncableType := c.Params.SyncableType

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		c.Err = model.NewAppError("Api4.createGroupSyncable", "api.io_error", nil, err.Error(), http.StatusBadRequest)
		return
	}

	group, groupErr := c.App.GetGroup(c.Params.GroupId, nil)
	if groupErr != nil {
		c.Err = groupErr
		return
	}

	if group.Source != model.GroupSourceLdap {
		c.Err = model.NewAppError("Api4.linkGroupSyncable", "app.group.crud_permission", nil, "", http.StatusBadRequest)
		return
	}

	auditRec := c.MakeAuditRecord("linkGroupSyncable", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("group_id", c.Params.GroupId)
	auditRec.AddMeta("syncable_id", syncableID)
	auditRec.AddMeta("syncable_type", syncableType)

	var patch *model.GroupSyncablePatch
	err = json.Unmarshal(body, &patch)
	if err != nil || patch == nil {
		c.SetInvalidParam(fmt.Sprintf("Group%s", syncableType.String()))
		return
	}

	if !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.createGroupSyncable", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	appErr := verifyLinkUnlinkPermission(c, syncableType, syncableID)
	if appErr != nil {
		c.Err = appErr
		return
	}

	groupSyncable := &model.GroupSyncable{
		GroupId:    c.Params.GroupId,
		SyncableId: syncableID,
		Type:       syncableType,
	}
	groupSyncable.Patch(patch)
	groupSyncable, appErr = c.App.UpsertGroupSyncable(groupSyncable)
	if appErr != nil {
		c.Err = appErr
		return
	}

	c.App.Srv().Go(func() {
		c.App.SyncRolesAndMembership(c.AppContext, syncableID, syncableType, false)
	})

	w.WriteHeader(http.StatusCreated)

	b, marshalErr := json.Marshal(groupSyncable)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.createGroupSyncable", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}
	auditRec.Success()
	w.Write(b)
}

func getGroupSyncable(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	c.RequireSyncableId()
	if c.Err != nil {
		return
	}
	syncableID := c.Params.SyncableId

	c.RequireSyncableType()
	if c.Err != nil {
		return
	}
	syncableType := c.Params.SyncableType

	if !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.getGroupSyncable", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionManageSystem) {
		c.SetPermissionError(model.PermissionManageSystem)
		return
	}

	groupSyncable, err := c.App.GetGroupSyncable(c.Params.GroupId, syncableID, syncableType)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(groupSyncable)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroupSyncable", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func getGroupSyncables(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	c.RequireSyncableType()
	if c.Err != nil {
		return
	}
	syncableType := c.Params.SyncableType

	if !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.getGroupSyncables", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionSysconsoleReadUserManagementGroups) {
		c.SetPermissionError(model.PermissionSysconsoleReadUserManagementGroups)
		return
	}

	groupSyncables, err := c.App.GetGroupSyncables(c.Params.GroupId, syncableType)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(groupSyncables)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroupSyncables", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func patchGroupSyncable(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	c.RequireSyncableId()
	if c.Err != nil {
		return
	}
	syncableID := c.Params.SyncableId

	c.RequireSyncableType()
	if c.Err != nil {
		return
	}
	syncableType := c.Params.SyncableType

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		c.Err = model.NewAppError("Api4.patchGroupSyncable", "api.io_error", nil, err.Error(), http.StatusBadRequest)
		return
	}

	auditRec := c.MakeAuditRecord("patchGroupSyncable", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("group_id", c.Params.GroupId)
	auditRec.AddMeta("old_syncable_id", syncableID)
	auditRec.AddMeta("old_syncable_type", syncableType)

	var patch *model.GroupSyncablePatch
	err = json.Unmarshal(body, &patch)
	if err != nil || patch == nil {
		c.SetInvalidParam(fmt.Sprintf("Group[%s]Patch", syncableType.String()))
		return
	}

	if !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.patchGroupSyncable", "api.ldap_groups.license_error", nil, "",
			http.StatusNotImplemented)
		return
	}

	appErr := verifyLinkUnlinkPermission(c, syncableType, syncableID)
	if appErr != nil {
		c.Err = appErr
		return
	}

	groupSyncable, appErr := c.App.GetGroupSyncable(c.Params.GroupId, syncableID, syncableType)
	if appErr != nil {
		c.Err = appErr
		return
	}

	groupSyncable.Patch(patch)

	groupSyncable, appErr = c.App.UpdateGroupSyncable(groupSyncable)
	if appErr != nil {
		c.Err = appErr
		return
	}

	auditRec.AddMeta("new_syncable_id", groupSyncable.SyncableId)
	auditRec.AddMeta("new_syncable_type", groupSyncable.Type)

	c.App.Srv().Go(func() {
		c.App.SyncRolesAndMembership(c.AppContext, syncableID, syncableType, false)
	})

	b, marshalErr := json.Marshal(groupSyncable)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.patchGroupSyncable", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}
	auditRec.Success()
	w.Write(b)
}

func unlinkGroupSyncable(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	c.RequireSyncableId()
	if c.Err != nil {
		return
	}
	syncableID := c.Params.SyncableId

	c.RequireSyncableType()
	if c.Err != nil {
		return
	}
	syncableType := c.Params.SyncableType

	auditRec := c.MakeAuditRecord("unlinkGroupSyncable", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("group_id", c.Params.GroupId)
	auditRec.AddMeta("syncable_id", syncableID)
	auditRec.AddMeta("syncable_type", syncableType)

	if !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.unlinkGroupSyncable", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	err := verifyLinkUnlinkPermission(c, syncableType, syncableID)
	if err != nil {
		c.Err = err
		return
	}

	_, err = c.App.DeleteGroupSyncable(c.Params.GroupId, syncableID, syncableType)
	if err != nil {
		c.Err = err
		return
	}

	c.App.Srv().Go(func() {
		c.App.SyncRolesAndMembership(c.AppContext, syncableID, syncableType, false)
	})

	auditRec.Success()

	ReturnStatusOK(w)
}

func verifyLinkUnlinkPermission(c *Context, syncableType model.GroupSyncableType, syncableID string) *model.AppError {
	switch syncableType {
	case model.GroupSyncableTypeTeam:
		if !c.App.SessionHasPermissionToTeam(*c.AppContext.Session(), syncableID, model.PermissionManageTeam) {
			return c.App.MakePermissionError(c.AppContext.Session(), []*model.Permission{model.PermissionManageTeam})
		}
	case model.GroupSyncableTypeChannel:
		channel, err := c.App.GetChannel(syncableID)
		if err != nil {
			return err
		}

		var permission *model.Permission
		if channel.Type == model.ChannelTypePrivate {
			permission = model.PermissionManagePrivateChannelMembers
		} else {
			permission = model.PermissionManagePublicChannelMembers
		}

		if !c.App.SessionHasPermissionToChannel(*c.AppContext.Session(), syncableID, permission) {
			return c.App.MakePermissionError(c.AppContext.Session(), []*model.Permission{permission})
		}
	}

	return nil
}

func getGroupMembers(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	group, err := c.App.GetGroup(c.Params.GroupId, nil)
	if err != nil {
		c.Err = err
		return
	}

	if lcErr := licensedAndConfiguredForGroupBySource(c.App, group.Source); lcErr != nil {
		lcErr.Where = "Api4.getGroupMembers"
		c.Err = lcErr
		return
	}

	if group.Source == model.GroupSourceLdap && !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionSysconsoleReadUserManagementGroups) {
		c.SetPermissionError(model.PermissionSysconsoleReadUserManagementGroups)
		return
	}

	members, count, err := c.App.GetGroupMemberUsersPage(c.Params.GroupId, c.Params.Page, c.Params.PerPage)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(struct {
		Members []*model.User `json:"members"`
		Count   int           `json:"total_member_count"`
	}{
		Members: members,
		Count:   count,
	})
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroupMembers", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func getGroupStats(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	if !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.getGroupStats", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionSysconsoleReadUserManagementGroups) {
		c.SetPermissionError(model.PermissionSysconsoleReadUserManagementGroups)
		return
	}

	groupID := c.Params.GroupId
	count, err := c.App.GetGroupMemberCount(groupID)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(model.GroupStats{
		GroupID:          groupID,
		TotalMemberCount: count,
	})
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroupStats", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func getGroupsByUserId(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireUserId()
	if c.Err != nil {
		return
	}

	if c.AppContext.Session().UserId != c.Params.UserId && !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionManageSystem) {
		c.SetPermissionError(model.PermissionManageSystem)
		return
	}

	if !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.getGroupsByUserId", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	groups, err := c.App.GetGroupsByUserId(c.Params.UserId)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(groups)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroupsByUserId", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func getGroupsByChannel(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireChannelId()
	if c.Err != nil {
		return
	}

	if c.App.Srv().License() == nil || !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.getGroupsByChannel", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	channel, err := c.App.GetChannel(c.Params.ChannelId)
	if err != nil {
		c.Err = err
		return
	}
	var permission *model.Permission
	if channel.Type == model.ChannelTypePrivate {
		permission = model.PermissionReadPrivateChannelGroups
	} else {
		permission = model.PermissionReadPublicChannelGroups
	}
	if !c.App.SessionHasPermissionToChannel(*c.AppContext.Session(), c.Params.ChannelId, permission) {
		c.SetPermissionError(permission)
		return
	}

	opts := model.GroupSearchOpts{
		Q:                    c.Params.Q,
		IncludeMemberCount:   c.Params.IncludeMemberCount,
		FilterAllowReference: c.Params.FilterAllowReference,
	}
	if c.Params.Paginate == nil || *c.Params.Paginate {
		opts.PageOpts = &model.PageOpts{Page: c.Params.Page, PerPage: c.Params.PerPage}
	}

	groups, totalCount, err := c.App.GetGroupsByChannel(c.Params.ChannelId, opts)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(struct {
		Groups []*model.GroupWithSchemeAdmin `json:"groups"`
		Count  int                           `json:"total_group_count"`
	}{
		Groups: groups,
		Count:  totalCount,
	})

	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroupsByChannel", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func getGroupsByTeam(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireTeamId()
	if c.Err != nil {
		return
	}
	if c.App.Srv().License() == nil || !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.getGroupsByTeam", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	opts := model.GroupSearchOpts{
		Q:                    c.Params.Q,
		IncludeMemberCount:   c.Params.IncludeMemberCount,
		FilterAllowReference: c.Params.FilterAllowReference,
	}
	if c.Params.Paginate == nil || *c.Params.Paginate {
		opts.PageOpts = &model.PageOpts{Page: c.Params.Page, PerPage: c.Params.PerPage}
	}

	groups, totalCount, err := c.App.GetGroupsByTeam(c.Params.TeamId, opts)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(struct {
		Groups []*model.GroupWithSchemeAdmin `json:"groups"`
		Count  int                           `json:"total_group_count"`
	}{
		Groups: groups,
		Count:  totalCount,
	})

	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroupsByTeam", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func getGroupsAssociatedToChannelsByTeam(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireTeamId()
	if c.Err != nil {
		return
	}

	if !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.getGroupsAssociatedToChannelsByTeam", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	opts := model.GroupSearchOpts{
		Q:                    c.Params.Q,
		IncludeMemberCount:   c.Params.IncludeMemberCount,
		FilterAllowReference: c.Params.FilterAllowReference,
	}
	if c.Params.Paginate == nil || *c.Params.Paginate {
		opts.PageOpts = &model.PageOpts{Page: c.Params.Page, PerPage: c.Params.PerPage}
	}

	groupsAssociatedByChannelID, err := c.App.GetGroupsAssociatedToChannelsByTeam(c.Params.TeamId, opts)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(struct {
		GroupsAssociatedToChannels map[string][]*model.GroupWithSchemeAdmin `json:"groups"`
	}{
		GroupsAssociatedToChannels: groupsAssociatedByChannelID,
	})

	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroupsAssociatedToChannelsByTeam", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func getGroups(c *Context, w http.ResponseWriter, r *http.Request) {
	var teamID, channelID string

	if id := c.Params.NotAssociatedToTeam; model.IsValidId(id) {
		teamID = id
	}

	if id := c.Params.NotAssociatedToChannel; model.IsValidId(id) {
		channelID = id
	}

	opts := model.GroupSearchOpts{
		Q:                         c.Params.Q,
		IncludeMemberCount:        c.Params.IncludeMemberCount,
		FilterAllowReference:      c.Params.FilterAllowReference,
		FilterParentTeamPermitted: c.Params.FilterParentTeamPermitted,
		Source:                    c.Params.GroupSource,
		FilterHasMember:           c.Params.FilterHasMember,
	}

	if !c.App.Config().FeatureFlags.CustomGroups && opts.Source == model.GroupSourceCustom {
		c.Err = model.NewAppError("getGroups", "api.custom_groups.feature_disabled", nil, "", http.StatusNotImplemented)
		return
	}

	if lcErr := licensedAndConfiguredForGroupBySource(c.App, opts.Source); lcErr != nil {
		lcErr.Where = "Api4.getGroups"
		c.Err = lcErr
		return
	}

	if teamID != "" {
		_, err := c.App.GetTeam(teamID)
		if err != nil {
			c.Err = err
			return
		}

		opts.NotAssociatedToTeam = teamID
	}

	if channelID != "" {
		channel, err := c.App.GetChannel(channelID)
		if err != nil {
			c.Err = err
			return
		}
		var permission *model.Permission
		if channel.Type == model.ChannelTypePrivate {
			permission = model.PermissionManagePrivateChannelMembers
		} else {
			permission = model.PermissionManagePublicChannelMembers
		}
		if !c.App.SessionHasPermissionToChannel(*c.AppContext.Session(), channelID, permission) {
			c.SetPermissionError(permission)
			return
		}
		opts.NotAssociatedToChannel = channelID
	}

	sinceString := r.URL.Query().Get("since")
	if sinceString != "" {
		since, parseError := strconv.ParseInt(sinceString, 10, 64)
		if parseError != nil {
			c.SetInvalidParam("since")
			return
		}
		opts.Since = since
	}

	groups, err := c.App.GetGroups(c.Params.Page, c.Params.PerPage, opts)
	if err != nil {
		c.Err = err
		return
	}

	var b []byte
	var marshalErr error
	if c.Params.IncludeTotalCount {
		totalCount, countErr := c.App.Srv().Store.Group().GroupCount()
		if countErr != nil {
			c.Err = model.NewAppError("Api4.getGroups", "api.custom_groups.count_err", nil, countErr.Error(), http.StatusInternalServerError)
			return
		}
		gwc := &model.GroupsWithCount{
			Groups:     groups,
			TotalCount: totalCount,
		}
		b, marshalErr = json.Marshal(gwc)
	} else {
		b, marshalErr = json.Marshal(groups)
	}

	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getGroups", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func deleteGroup(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	group, err := c.App.GetGroup(c.Params.GroupId, nil)
	if err != nil {
		c.Err = err
		return
	}

	if group.Source != model.GroupSourceCustom {
		c.Err = model.NewAppError("Api4.deleteGroup", "app.group.crud_permission", nil, "", http.StatusNotImplemented)
		return
	}

	if lcErr := licensedAndConfiguredForGroupBySource(c.App, model.GroupSourceCustom); lcErr != nil {
		lcErr.Where = "Api4.deleteGroup"
		c.Err = lcErr
		return
	}

	if !c.App.SessionHasPermissionToGroup(*c.AppContext.Session(), c.Params.GroupId, model.PermissionDeleteCustomGroup) {
		c.SetPermissionError(model.PermissionDeleteCustomGroup)
		return
	}

	auditRec := c.MakeAuditRecord("deleteGroup", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("group_id", c.Params.GroupId)

	_, err = c.App.DeleteGroup(c.Params.GroupId)
	if err != nil {
		c.Err = err
		return
	}

	auditRec.Success()

	ReturnStatusOK(w)
}

func addGroupMembers(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	group, err := c.App.GetGroup(c.Params.GroupId, nil)
	if err != nil {
		c.Err = err
		return
	}

	if group.Source != model.GroupSourceCustom {
		c.Err = model.NewAppError("Api4.deleteGroup", "app.group.crud_permission", nil, "", http.StatusNotImplemented)
		return
	}

	if lcErr := licensedAndConfiguredForGroupBySource(c.App, model.GroupSourceCustom); lcErr != nil {
		lcErr.Where = "Api4.deleteGroup"
		c.Err = lcErr
		return
	}

	if !c.App.SessionHasPermissionToGroup(*c.AppContext.Session(), c.Params.GroupId, model.PermissionManageCustomGroupMembers) {
		c.SetPermissionError(model.PermissionManageCustomGroupMembers)
		return
	}

	var newMembers *model.GroupModifyMembers
	if jsonErr := json.NewDecoder(r.Body).Decode(&newMembers); jsonErr != nil {
		c.SetInvalidParam("addGroupMembers")
		return
	}

	auditRec := c.MakeAuditRecord("addGroupMembers", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("addGroupMembers", newMembers)

	members, err := c.App.UpsertGroupMembers(c.Params.GroupId, newMembers.UserIds)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(members)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.addGroupMembers", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}
	auditRec.Success()
	w.Write(b)
}

func deleteGroupMembers(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireGroupId()
	if c.Err != nil {
		return
	}

	group, err := c.App.GetGroup(c.Params.GroupId, nil)
	if err != nil {
		c.Err = err
		return
	}

	if group.Source != model.GroupSourceCustom {
		c.Err = model.NewAppError("Api4.deleteGroup", "app.group.crud_permission", nil, "", http.StatusNotImplemented)
		return
	}

	if lcErr := licensedAndConfiguredForGroupBySource(c.App, model.GroupSourceCustom); lcErr != nil {
		lcErr.Where = "Api4.deleteGroup"
		c.Err = lcErr
		return
	}

	if !c.App.SessionHasPermissionToGroup(*c.AppContext.Session(), c.Params.GroupId, model.PermissionManageCustomGroupMembers) {
		c.SetPermissionError(model.PermissionManageCustomGroupMembers)
		return
	}

	var deleteBody *model.GroupModifyMembers
	if jsonErr := json.NewDecoder(r.Body).Decode(&deleteBody); jsonErr != nil {
		c.SetInvalidParam("deleteGroupMembers")
		return
	}

	auditRec := c.MakeAuditRecord("deleteGroupMembers", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("deleteGroupMembers", deleteBody)

	members, err := c.App.DeleteGroupMembers(c.Params.GroupId, deleteBody.UserIds)
	if err != nil {
		c.Err = err
		return
	}

	b, marshalErr := json.Marshal(members)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.addGroupMembers", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}
	auditRec.Success()
	w.Write(b)
}

// licensedAndConfiguredForGroupBySource returns an app error if not properly license or configured for the given group type. The returned app error
// will have a blank 'Where' field, which should be subsequently set by the caller, for example:
//
//    err := licensedAndConfiguredForGroupBySource(c.App, group.Source)
//    err.Where = "Api4.getGroup"
//
// Temporarily, this function also checks for the CustomGroups feature flag.
func licensedAndConfiguredForGroupBySource(app app.AppIface, source model.GroupSource) *model.AppError {
	lic := app.Srv().License()

	if lic == nil {
		return model.NewAppError("", "api.license_error", nil, "", http.StatusNotImplemented)
	}

	if source == model.GroupSourceLdap && !*lic.Features.LDAPGroups {
		return model.NewAppError("", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
	}

	if source == model.GroupSourceCustom && lic.SkuShortName != model.LicenseShortSkuProfessional && lic.SkuShortName != model.LicenseShortSkuEnterprise {
		return model.NewAppError("", "api.custom_groups.license_error", nil, "", http.StatusNotImplemented)
	}

	if source == model.GroupSourceCustom && (!app.Config().FeatureFlags.CustomGroups || !*app.Config().ServiceSettings.EnableCustomGroups) {
		return model.NewAppError("", "api.custom_groups.feature_disabled", nil, "", http.StatusNotImplemented)
	}

	return nil
}
