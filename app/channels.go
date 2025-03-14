// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mattermost/mattermost-server/v6/app/imaging"
	"github.com/mattermost/mattermost-server/v6/app/request"
	"github.com/mattermost/mattermost-server/v6/config"
	"github.com/mattermost/mattermost-server/v6/einterfaces"
	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin"
	"github.com/mattermost/mattermost-server/v6/services/imageproxy"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
	"github.com/pkg/errors"
)

// configSvc is a consumer interface to work
// with any config related task with the server.
type configSvc interface {
	Config() *model.Config
	AddConfigListener(listener func(*model.Config, *model.Config)) string
	RemoveConfigListener(id string)
	UpdateConfig(f func(*model.Config))
	SaveConfig(newCfg *model.Config, sendConfigChangeClusterMessage bool) (*model.Config, *model.Config, *model.AppError)
}

// licenseSvc is added to act as a starting point for future integrated products.
// It has the same signature and functionality with the license related APIs of the plugin-api.
type licenseSvc interface { // nolint: unused,deadcode
	GetLicense() *model.License
	RequestTrialLicense(requesterID string, users int, termsAccepted bool, receiveEmailsAccepted bool) *model.AppError
}

// namer is an interface which enforces that
// all services can return their names.
type namer interface {
	Name() ServiceKey
}

// Channels contains all channels related state.
type Channels struct {
	srv    *Server
	cfgSvc configSvc

	postActionCookieSecret []byte

	pluginCommandsLock     sync.RWMutex
	pluginCommands         []*PluginCommand
	pluginsLock            sync.RWMutex
	pluginsEnvironment     *plugin.Environment
	pluginConfigListenerID string

	imageProxy *imageproxy.ImageProxy

	asymmetricSigningKey atomic.Value
	clientConfig         atomic.Value
	clientConfigHash     atomic.Value
	limitedClientConfig  atomic.Value

	// cached counts that are used during notice condition validation
	cachedPostCount   int64
	cachedUserCount   int64
	cachedDBMSVersion string
	// previously fetched notices
	cachedNotices model.ProductNotices

	AccountMigration einterfaces.AccountMigrationInterface
	Compliance       einterfaces.ComplianceInterface
	DataRetention    einterfaces.DataRetentionInterface
	MessageExport    einterfaces.MessageExportInterface

	// These are used to prevent concurrent upload requests
	// for a given upload session which could cause inconsistencies
	// and data corruption.
	uploadLockMapMut sync.Mutex
	uploadLockMap    map[string]bool

	imgDecoder *imaging.Decoder
	imgEncoder *imaging.Encoder

	dndTaskMut sync.Mutex
	dndTask    *model.ScheduledTask
}

func init() {
	RegisterProduct("channels", func(s *Server, services map[ServiceKey]interface{}) (Product, error) {
		return NewChannels(s, services)
	})
}

func NewChannels(s *Server, services map[ServiceKey]interface{}) (*Channels, error) {

	ch := &Channels{
		srv:           s,
		imageProxy:    imageproxy.MakeImageProxy(s, s.httpService, s.Log),
		uploadLockMap: map[string]bool{},
	}

	// To get another service:
	// 1. Prepare the service interface
	// 2. Add the field to *Channels
	// 3. Add the service key to the slice.
	// 4. Add a new case in the switch statement.
	requiredServices := []ServiceKey{ConfigKey}
	for _, svcKey := range requiredServices {
		svc, ok := services[svcKey]
		if !ok {
			return nil, fmt.Errorf("Service %s not passed", svcKey)
		}
		switch svcKey {
		// Keep adding more services here
		case ConfigKey:
			cfgSvc, ok := svc.(configSvc)
			if !ok {
				return nil, errors.New("Config service did not satisfy ConfigSvc interface")
			}
			_, ok = svc.(namer)
			if !ok {
				return nil, errors.New("Config service does not contain Name method")
			}
			ch.cfgSvc = cfgSvc
		}

	}
	// We are passing a partially filled Channels struct so that the enterprise
	// methods can have access to app methods.
	// Otherwise, passing server would mean it has to call s.Channels(),
	// which would be nil at this point.
	if complianceInterface != nil {
		ch.Compliance = complianceInterface(New(ServerConnector(ch)))
	}
	if messageExportInterface != nil {
		ch.MessageExport = messageExportInterface(New(ServerConnector(ch)))
	}
	if dataRetentionInterface != nil {
		ch.DataRetention = dataRetentionInterface(New(ServerConnector(ch)))
	}
	if accountMigrationInterface != nil {
		ch.AccountMigration = accountMigrationInterface(New(ServerConnector(ch)))
	}

	var imgErr error
	ch.imgDecoder, imgErr = imaging.NewDecoder(imaging.DecoderOptions{
		ConcurrencyLevel: runtime.NumCPU(),
	})
	if imgErr != nil {
		return nil, errors.Wrap(imgErr, "failed to create image decoder")
	}
	ch.imgEncoder, imgErr = imaging.NewEncoder(imaging.EncoderOptions{
		ConcurrencyLevel: runtime.NumCPU(),
	})
	if imgErr != nil {
		return nil, errors.Wrap(imgErr, "failed to create image encoder")
	}

	// Setup routes.
	pluginsRoute := ch.srv.Router.PathPrefix("/plugins/{plugin_id:[A-Za-z0-9\\_\\-\\.]+}").Subrouter()
	pluginsRoute.HandleFunc("", ch.ServePluginRequest)
	pluginsRoute.HandleFunc("/public/{public_file:.*}", ch.ServePluginPublicRequest)
	pluginsRoute.HandleFunc("/{anything:.*}", ch.ServePluginRequest)

	return ch, nil
}

func (ch *Channels) Start() error {
	// Start plugins
	ctx := request.EmptyContext()
	ch.initPlugins(ctx, *ch.cfgSvc.Config().PluginSettings.Directory, *ch.cfgSvc.Config().PluginSettings.ClientDirectory)

	ch.AddConfigListener(func(prevCfg, cfg *model.Config) {
		// We compute the difference between configs
		// to ensure we don't re-init plugins unnecessarily.
		diffs, err := config.Diff(prevCfg, cfg)
		if err != nil {
			mlog.Warn("Error in comparing configs", mlog.Err(err))
			return
		}

		hasDiff := false
		// TODO: This could be a method on ConfigDiffs itself
		for _, diff := range diffs {
			if strings.HasPrefix(diff.Path, "PluginSettings.") {
				hasDiff = true
				break
			}
		}

		// Do only if some plugin related settings has changed.
		if hasDiff {
			if *cfg.PluginSettings.Enable {
				ch.initPlugins(ctx, *cfg.PluginSettings.Directory, *ch.cfgSvc.Config().PluginSettings.ClientDirectory)
			} else {
				ch.ShutDownPlugins()
			}
		}

	})

	if err := ch.ensureAsymmetricSigningKey(); err != nil {
		return errors.Wrapf(err, "unable to ensure asymmetric signing key")
	}

	if err := ch.ensurePostActionCookieSecret(); err != nil {
		return errors.Wrapf(err, "unable to ensure PostAction cookie secret")
	}
	return nil
}

func (ch *Channels) Stop() error {
	ch.ShutDownPlugins()

	ch.dndTaskMut.Lock()
	if ch.dndTask != nil {
		ch.dndTask.Cancel()
	}
	ch.dndTaskMut.Unlock()

	return nil
}

func (ch *Channels) AddConfigListener(listener func(*model.Config, *model.Config)) string {
	return ch.srv.AddConfigListener(listener)
}

func (ch *Channels) RemoveConfigListener(id string) {
	ch.srv.RemoveConfigListener(id)
}
