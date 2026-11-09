package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/serverstatus"
	"github.com/UruhaLushia/tomori/locales"
)

type serverState struct {
	data         page.ServersData
	request      uint64
	cancel       context.CancelFunc
	ctx          context.Context
	lane         chan struct{}
	pending      int
	editIndex    int
	editRevision [32]byte
	operation    uint64
}

func (a *app) releaseServers() {
	s := &a.servers
	if s.data.Key == "" {
		return
	}
	a.closeServerEdit()
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	s.data, s.ctx, s.cancel, s.pending = page.ServersData{}, nil, nil, 0
}

func (a *app) inspectServers(instance minecraft.Instance, refresh bool) {
	s := &a.servers
	root := instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate)
	if !refresh && s.data.Key == instance.Key() && s.data.Root == root {
		return
	}
	if s.data.Edit.Open {
		a.closeServerEdit()
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	token := s.request
	ctx, cancel := context.WithCancel(a.ctx)
	s.ctx, s.cancel, s.pending = ctx, cancel, 0
	s.data = page.ServersData{Key: instance.Key(), Root: root, Loading: true, Protocol: -1, Status: make(map[int]page.ServerResult)}
	service := a.newService()
	a.workers.Go(func() {
		defer service.Close()
		list, err := service.Servers(ctx, instance)
		protocol := -1
		if err == nil {
			protocol = service.ServerProtocol(ctx, instance)
		}
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.servers
			if token != s.request || a.ctx.Err() != nil {
				return
			}
			s.data.Loading = false
			if err != nil {
				s.data.Error = err.Error()
				return
			}
			s.data.List, s.data.Protocol, s.data.Ready = list, protocol, true
		})
	})
}

func (a *app) refreshServers() {
	if a.servers.data.Edit.Open {
		return
	}
	if instance, ok := a.pages.instances.Selected(a.installedList); ok && a.pages.instances.ServersOpen() {
		if a.resources.data.Embedded {
			a.closeResources()
		}
		a.inspectServers(instance, true)
	}
}

func (a *app) queryServer(index int) {
	s := &a.servers
	if !s.data.Ready || index < 0 || index >= len(s.data.List.Entries) || s.pending >= 16 {
		return
	}
	if result := s.data.Status[index]; result.Done || result.Loading {
		return
	}
	if s.lane == nil {
		s.lane = make(chan struct{}, 4)
	}
	s.data.Status[index] = page.ServerResult{Loading: true}
	s.pending++
	ctx, token, lane, address, protocol := s.ctx, s.request, s.lane, s.data.List.Entries[index].Address, s.data.Protocol
	a.workers.Go(func() {
		select {
		case lane <- struct{}{}:
		case <-ctx.Done():
			return
		}
		value, err := (serverstatus.Client{}).Query(ctx, address, protocol)
		<-lane
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.servers
			if token != s.request || a.ctx.Err() != nil {
				return
			}
			s.pending--
			var bitmap *ui.Bitmap
			if err == nil && len(value.Icon) > 0 {
				if icon, decodeErr := minecraft.DecodeInstanceIcon(value.Icon); decodeErr == nil {
					bitmap = ui.NewBitmap(icon)
				}
			}
			value.Icon = nil
			memberIcons := make(map[string]*ui.Bitmap, len(value.Members))
			for i, member := range value.Members {
				if i >= 32 {
					break
				}
				id := member.UUID
				if id == "" {
					id = member.Name
				}
				memberIcons[id] = ui.NewBitmap(account.DefaultAvatar(account.Account{Name: member.Name, UUID: id}))
			}
			result := page.ServerResult{Done: true, Value: value, Icon: bitmap, MemberIcons: memberIcons}
			if err != nil {
				result.Error = err.Error()
			}
			s.data.Status[index] = result
		})
	})
}

func (a *app) loadServerMemberAvatars(index int, token uint64, members []serverstatus.Member) {
	ctx := a.servers.ctx
	cacheDir, proxy, budget := a.cfg.CacheDirectory(), a.cfg.Download.Proxy, a.downloadBudget
	for i, member := range members {
		if i >= 32 {
			break
		}
		member := member
		uuid := strings.ReplaceAll(member.UUID, "-", "")
		if len(uuid) != 32 {
			continue
		}
		a.workers.Go(func() {
			client := account.NewClient(proxy, budget)
			defer client.Close()
			portrait, err := client.Avatar(ctx, cacheDir, account.Account{ID: "server:" + uuid, Kind: account.Microsoft, Name: member.Name, UUID: uuid})
			if err != nil || portrait == nil {
				return
			}
			bitmap := ui.NewBitmap(portrait)
			a.win.Update(func() {
				s := &a.servers
				if ctx.Err() != nil || a.ctx.Err() != nil || token != s.request {
					return
				}
				result := s.data.Status[index]
				if result.MemberIcons == nil {
					result.MemberIcons = make(map[string]*ui.Bitmap)
				}
				result.MemberIcons[member.UUID] = bitmap
				s.data.Status[index] = result
			})
		})
	}
}

func (a *app) serverInstance() (minecraft.Instance, bool) {
	s := &a.servers
	instance, ok := a.instanceByKey(s.data.Key)
	return instance, ok && instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate) == s.data.Root
}

func (a *app) openServerEdit(index int, remove bool) {
	s := &a.servers
	if !s.data.Ready || a.pathBusy || a.tasksFull() || a.instanceInUse(s.data.Key) || index < -1 || index >= len(s.data.List.Entries) || index == -1 && remove {
		return
	}
	if _, ok := a.serverInstance(); !ok {
		return
	}
	d := page.ServerEditData{Open: true, New: index == -1}
	if index >= 0 {
		entry := s.data.List.Entries[index]
		d.Name, d.Address, d.Textures = entry.Name, entry.Address, entry.Textures
	}
	d.Remove = remove
	s.editIndex, s.editRevision = index, s.data.List.Revision
	s.data.Edit = d
	a.pathBusy = true
}

func (a *app) closeServerEdit() {
	s := &a.servers
	if s.data.Edit.Busy {
		a.cancelOperation(s.operation)
	}
	s.data.Edit, s.operation = page.ServerEditData{}, 0
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) saveServer() {
	s := &a.servers
	if !s.data.Edit.Open || s.data.Edit.Busy || a.tasksFull() || a.instanceInUse(s.data.Key) {
		return
	}
	instance, ok := a.serverInstance()
	if !ok {
		s.data.Edit.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	d, index, revision, token := s.data.Edit, s.editIndex, s.editRevision, s.request
	if !d.Remove {
		if _, err := serverstatus.ParseAddress(d.Address); err != nil {
			s.data.Edit.Error = err.Error()
			return
		}
	}
	title := locales.SaveServer()
	if d.Remove {
		title = locales.DeleteServer()
	}
	ctx, service, operation := a.beginOperation(title, instance.Key(), s.data.Root)
	if operation == 0 {
		return
	}
	s.data.Edit.Busy, s.data.Edit.Error, s.operation = true, "", operation
	next := instancecontent.Server{Name: d.Name, Address: d.Address, Textures: d.Textures}
	a.workers.Go(func() {
		defer service.Close()
		err := service.ChangeServer(ctx, instance, revision, index, next, d.Remove)
		a.win.Update(func() {
			s := &a.servers
			if a.ctx.Err() != nil || token != s.request || s.operation != operation || !s.data.Edit.Open {
				return
			}
			s.data.Edit.Busy = false
			if err != nil {
				s.data.Edit.Error = err.Error()
				return
			}
			a.closeServerEdit()
			a.inspectServers(instance, true)
		})
		a.finishOperation(operation, err)
	})
}

func (a *app) serverEditView(c *ui.Context) {
	page.ServerEdit(c, &a.servers.data.Edit, a.saveServer, a.closeServerEdit)
}

func (a *app) serverActions() page.ServersActions {
	return page.ServersActions{Refresh: a.refreshServers, Query: a.queryServer, Members: a.showServerMembers, Edit: a.openServerEdit}
}

func (a *app) showServerMembers(index int) {
	if index < 0 || index >= len(a.servers.data.List.Entries) {
		return
	}
	result := a.servers.data.Status[index]
	if !result.Done || result.Error != "" {
		return
	}
	a.pages.instances.ShowServerMembers(index)
	a.loadServerMemberAvatars(index, a.servers.request, result.Value.Members)
}

func (a *app) serversBusy() bool {
	return a.pathBusy || a.tasksFull() || a.instanceInUse(a.servers.data.Key)
}
