// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"encoding/json"
	"mime/multipart"
	"net/http"

	"github.com/mattermost/mattermost-server/v6/audit"
	"github.com/mattermost/mattermost-server/v6/model"
)

type mixedUnlinkedGroup struct {
	Id           *string `json:"mattermost_group_id"`
	DisplayName  string  `json:"name"`
	RemoteId     string  `json:"primary_key"`
	HasSyncables *bool   `json:"has_syncables"`
}

func (api *API) InitLdap() {
	api.BaseRoutes.LDAP.Handle("/sync", api.APISessionRequired(syncLdap)).Methods("POST")
	api.BaseRoutes.LDAP.Handle("/test", api.APISessionRequired(testLdap)).Methods("POST")
	api.BaseRoutes.LDAP.Handle("/migrateid", api.APISessionRequired(migrateIdLdap)).Methods("POST")

	// GET /api/v4/ldap/groups?page=0&per_page=1000
	api.BaseRoutes.LDAP.Handle("/groups", api.APISessionRequired(getLdapGroups)).Methods("GET")

	// POST /api/v4/ldap/groups/:remote_id/link
	api.BaseRoutes.LDAP.Handle(`/groups/{remote_id}/link`, api.APISessionRequired(linkLdapGroup)).Methods("POST")

	// DELETE /api/v4/ldap/groups/:remote_id/link
	api.BaseRoutes.LDAP.Handle(`/groups/{remote_id}/link`, api.APISessionRequired(unlinkLdapGroup)).Methods("DELETE")

	api.BaseRoutes.LDAP.Handle("/certificate/public", api.APISessionRequired(addLdapPublicCertificate)).Methods("POST")
	api.BaseRoutes.LDAP.Handle("/certificate/private", api.APISessionRequired(addLdapPrivateCertificate)).Methods("POST")

	api.BaseRoutes.LDAP.Handle("/certificate/public", api.APISessionRequired(removeLdapPublicCertificate)).Methods("DELETE")
	api.BaseRoutes.LDAP.Handle("/certificate/private", api.APISessionRequired(removeLdapPrivateCertificate)).Methods("DELETE")

}

func syncLdap(c *Context, w http.ResponseWriter, r *http.Request) {
	if c.App.Srv().License() == nil || !*c.App.Srv().License().Features.LDAP {
		c.Err = model.NewAppError("Api4.syncLdap", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	type LdapSyncOptions struct {
		IncludeRemovedMembers bool `json:"include_removed_members"`
	}
	var opts LdapSyncOptions
	json.NewDecoder(r.Body).Decode(&opts)

	auditRec := c.MakeAuditRecord("syncLdap", audit.Fail)
	defer c.LogAuditRec(auditRec)

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionCreateLdapSyncJob) {
		c.SetPermissionError(model.PermissionCreateLdapSyncJob)
		return
	}

	c.App.SyncLdap(opts.IncludeRemovedMembers)

	auditRec.Success()
	ReturnStatusOK(w)
}

func testLdap(c *Context, w http.ResponseWriter, r *http.Request) {
	if c.App.Srv().License() == nil || !*c.App.Srv().License().Features.LDAP {
		c.Err = model.NewAppError("Api4.testLdap", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionTestLdap) {
		c.SetPermissionError(model.PermissionTestLdap)
		return
	}

	if err := c.App.TestLdap(); err != nil {
		c.Err = err
		return
	}

	ReturnStatusOK(w)
}

func getLdapGroups(c *Context, w http.ResponseWriter, r *http.Request) {
	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionSysconsoleReadUserManagementGroups) {
		c.SetPermissionError(model.PermissionSysconsoleReadUserManagementGroups)
		return
	}

	if c.App.Srv().License() == nil || !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.getLdapGroups", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	opts := model.LdapGroupSearchOpts{
		Q: c.Params.Q,
	}
	if c.Params.IsLinked != nil {
		opts.IsLinked = c.Params.IsLinked
	}
	if c.Params.IsConfigured != nil {
		opts.IsConfigured = c.Params.IsConfigured
	}

	groups, total, err := c.App.GetAllLdapGroupsPage(c.Params.Page, c.Params.PerPage, opts)
	if err != nil {
		c.Err = err
		return
	}

	mugs := []*mixedUnlinkedGroup{}
	for _, group := range groups {
		mug := &mixedUnlinkedGroup{
			DisplayName: group.DisplayName,
			RemoteId:    group.GetRemoteId(),
		}
		if len(group.Id) == 26 {
			mug.Id = &group.Id
			mug.HasSyncables = &group.HasSyncables
		}
		mugs = append(mugs, mug)
	}

	b, marshalErr := json.Marshal(struct {
		Count  int                   `json:"count"`
		Groups []*mixedUnlinkedGroup `json:"groups"`
	}{Count: total, Groups: mugs})
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.getLdapGroups", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func linkLdapGroup(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireRemoteId()
	if c.Err != nil {
		return
	}

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionSysconsoleWriteUserManagementGroups) {
		c.SetPermissionError(model.PermissionSysconsoleWriteUserManagementGroups)
		return
	}

	auditRec := c.MakeAuditRecord("linkLdapGroup", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("remote_id", c.Params.RemoteId)

	if c.App.Srv().License() == nil || !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.linkLdapGroup", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	ldapGroup, err := c.App.GetLdapGroup(c.Params.RemoteId)
	if err != nil {
		c.Err = err
		return
	}
	auditRec.AddMeta("ldap_group", ldapGroup)

	if ldapGroup == nil {
		c.Err = model.NewAppError("Api4.linkLdapGroup", "api.ldap_group.not_found", nil, "", http.StatusNotFound)
		return
	}

	group, err := c.App.GetGroupByRemoteID(ldapGroup.GetRemoteId(), model.GroupSourceLdap)
	if err != nil && err.Id != "app.group.no_rows" {
		c.Err = err
		return
	}
	if group != nil {
		auditRec.AddMeta("group", group)
	}

	var status int
	var newOrUpdatedGroup *model.Group

	// Truncate display name if necessary
	var displayName string
	if len(ldapGroup.DisplayName) > model.GroupDisplayNameMaxLength {
		displayName = ldapGroup.DisplayName[:model.GroupDisplayNameMaxLength]
	} else {
		displayName = ldapGroup.DisplayName
	}

	// Group has been previously linked
	if group != nil {
		if group.DeleteAt == 0 {
			newOrUpdatedGroup = group
		} else {
			group.DeleteAt = 0
			group.DisplayName = displayName
			group.RemoteId = ldapGroup.RemoteId
			newOrUpdatedGroup, err = c.App.UpdateGroup(group)
			if err != nil {
				c.Err = err
				return
			}
		}
		status = http.StatusOK
	} else {
		// Group has never been linked
		//
		// For group mentions implementation, the Name column will no longer be set by default.
		// Instead it will be set and saved in the web app when Group Mentions is enabled.
		newGroup := &model.Group{
			DisplayName: displayName,
			RemoteId:    ldapGroup.RemoteId,
			Source:      model.GroupSourceLdap,
		}
		newOrUpdatedGroup, err = c.App.CreateGroup(newGroup)
		if err != nil {
			c.Err = err
			return
		}
		status = http.StatusCreated
	}

	b, marshalErr := json.Marshal(newOrUpdatedGroup)
	if marshalErr != nil {
		c.Err = model.NewAppError("Api4.linkLdapGroup", "api.marshal_error", nil, marshalErr.Error(), http.StatusInternalServerError)
		return
	}

	auditRec.Success()

	w.WriteHeader(status)
	w.Write(b)
}

func unlinkLdapGroup(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireRemoteId()
	if c.Err != nil {
		return
	}

	auditRec := c.MakeAuditRecord("unlinkLdapGroup", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("remote_id", c.Params.RemoteId)

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionSysconsoleWriteUserManagementGroups) {
		c.SetPermissionError(model.PermissionSysconsoleWriteUserManagementGroups)
		return
	}

	if c.App.Srv().License() == nil || !*c.App.Srv().License().Features.LDAPGroups {
		c.Err = model.NewAppError("Api4.unlinkLdapGroup", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	group, err := c.App.GetGroupByRemoteID(c.Params.RemoteId, model.GroupSourceLdap)
	if err != nil {
		c.Err = err
		return
	}
	auditRec.AddMeta("group", group)

	if group.DeleteAt == 0 {
		_, err = c.App.DeleteGroup(group.Id)
		if err != nil {
			c.Err = err
			return
		}
	}

	auditRec.Success()
	ReturnStatusOK(w)
}

func migrateIdLdap(c *Context, w http.ResponseWriter, r *http.Request) {
	props := model.StringInterfaceFromJSON(r.Body)
	toAttribute, ok := props["toAttribute"].(string)
	if !ok || toAttribute == "" {
		c.SetInvalidParam("toAttribute")
		return
	}

	auditRec := c.MakeAuditRecord("idMigrateLdap", audit.Fail)
	defer c.LogAuditRec(auditRec)

	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionManageSystem) {
		c.SetPermissionError(model.PermissionManageSystem)
		return
	}

	if c.App.Srv().License() == nil || !*c.App.Srv().License().Features.LDAP {
		c.Err = model.NewAppError("Api4.idMigrateLdap", "api.ldap_groups.license_error", nil, "", http.StatusNotImplemented)
		return
	}

	if err := c.App.MigrateIdLDAP(toAttribute); err != nil {
		c.Err = err
		return
	}

	auditRec.Success()
	ReturnStatusOK(w)
}

func parseLdapCertificateRequest(r *http.Request, maxFileSize int64) (*multipart.FileHeader, *model.AppError) {
	err := r.ParseMultipartForm(maxFileSize)
	if err != nil {
		return nil, model.NewAppError("addLdapCertificate", "api.admin.add_certificate.parseform.app_error", nil, err.Error(), http.StatusBadRequest)
	}

	m := r.MultipartForm

	fileArray, ok := m.File["certificate"]
	if !ok {
		return nil, model.NewAppError("addLdapCertificate", "api.admin.add_certificate.no_file.app_error", nil, "", http.StatusBadRequest)
	}

	if len(fileArray) <= 0 {
		return nil, model.NewAppError("addLdapCertificate", "api.admin.add_certificate.array.app_error", nil, "", http.StatusBadRequest)
	}

	return fileArray[0], nil
}

func addLdapPublicCertificate(c *Context, w http.ResponseWriter, r *http.Request) {
	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionAddLdapPublicCert) {
		c.SetPermissionError(model.PermissionAddLdapPublicCert)
		return
	}

	fileData, err := parseLdapCertificateRequest(r, *c.App.Config().FileSettings.MaxFileSize)
	if err != nil {
		c.Err = err
		return
	}

	auditRec := c.MakeAuditRecord("addLdapPublicCertificate", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("filename", fileData.Filename)

	if err := c.App.AddLdapPublicCertificate(fileData); err != nil {
		c.Err = err
		return
	}
	auditRec.Success()
	ReturnStatusOK(w)
}

func addLdapPrivateCertificate(c *Context, w http.ResponseWriter, r *http.Request) {
	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionAddLdapPrivateCert) {
		c.SetPermissionError(model.PermissionAddLdapPrivateCert)
		return
	}

	fileData, err := parseLdapCertificateRequest(r, *c.App.Config().FileSettings.MaxFileSize)
	if err != nil {
		c.Err = err
		return
	}

	auditRec := c.MakeAuditRecord("addLdapPrivateCertificate", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("filename", fileData.Filename)

	if err := c.App.AddLdapPrivateCertificate(fileData); err != nil {
		c.Err = err
		return
	}
	auditRec.Success()
	ReturnStatusOK(w)
}

func removeLdapPublicCertificate(c *Context, w http.ResponseWriter, r *http.Request) {
	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionRemoveLdapPublicCert) {
		c.SetPermissionError(model.PermissionRemoveLdapPublicCert)
		return
	}

	auditRec := c.MakeAuditRecord("removeLdapPublicCertificate", audit.Fail)
	defer c.LogAuditRec(auditRec)

	if err := c.App.RemoveLdapPublicCertificate(); err != nil {
		c.Err = err
		return
	}

	auditRec.Success()
	ReturnStatusOK(w)
}

func removeLdapPrivateCertificate(c *Context, w http.ResponseWriter, r *http.Request) {
	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionRemoveLdapPrivateCert) {
		c.SetPermissionError(model.PermissionRemoveLdapPrivateCert)
		return
	}

	auditRec := c.MakeAuditRecord("removeLdapPrivateCertificate", audit.Fail)
	defer c.LogAuditRec(auditRec)

	if err := c.App.RemoveLdapPrivateCertificate(); err != nil {
		c.Err = err
		return
	}

	auditRec.Success()
	ReturnStatusOK(w)
}
