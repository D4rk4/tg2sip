package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	gosip "github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/sip"
	client "github.com/zelenin/go-tdlib/client"
	ini "gopkg.in/ini.v1"
)

// Gateway connects SIP server and Telegram client.
type Gateway struct {
	sipServer      gosip.Server
	tgClient       *client.Client
	sipClient      *SIPClient
	events         chan interface{}
	internalEvents chan internalEvent
	calls          map[string]*Context
	contacts       *ContactCache
	callback       string
	mu             sync.Mutex
}

// NewGateway creates a new Gateway instance.
func NewGateway(sipSrv gosip.Server, tgCl *client.Client, callback string) *Gateway {
	return &Gateway{
		sipServer:      sipSrv,
		tgClient:       tgCl,
		sipClient:      NewSIPClient(sipSrv),
		events:         make(chan interface{}, 16),
		internalEvents: make(chan internalEvent, 16),
		calls:          make(map[string]*Context),
		contacts:       NewContactCache(),
		callback:       callback,
	}
}

// CallStateEvent represents a change in call state.
type CallStateEvent struct {
	CallID string
	State  string
}

// MediaEvent represents a media-related SIP event.
type MediaEvent struct {
	CallID string
	Body   string
}

// Start runs the gateway until ctx is canceled.
func (g *Gateway) Start(ctx context.Context) error {
	if err := g.sipServer.OnRequest(sip.INVITE, g.handleInvite); err != nil {
		return err
	}
	if err := g.sipServer.OnRequest(sip.ACK, g.handleAck); err != nil {
		return err
	}
	if err := g.sipServer.OnRequest(sip.BYE, g.handleBye); err != nil {
		return err
	}
	if err := g.sipServer.OnRequest(sip.INFO, g.handleInfo); err != nil {
		return err
	}

	if err := g.contacts.Refresh(g.tgClient); err != nil {
		coreLog.Warnf("initial contacts load failed: %v", err)
	}
	go g.refreshContactsLoop(ctx)

	listener := g.tgClient.GetListener()
	defer listener.Close()

	for {
		select {
		case update := <-listener.Updates:
			switch u := update.(type) {
			case *client.UpdateCall:
				coreLog.Infof("received telegram call update: %d", u.Call.ID)
				g.handleTelegramCall(u)
			case *client.UpdateUser:
				g.contacts.Update(u.User)
			}
		case ev := <-g.events:
			coreLog.Infof("received gateway event: %#v", ev)
		case ie := <-g.internalEvents:
			g.processInternalEvent(ie)
		case <-ctx.Done():
			return nil
		}
	}
}

// refreshContactsLoop periodically reloads the contact cache.
func (g *Gateway) refreshContactsLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := g.contacts.Refresh(g.tgClient); err != nil {
				coreLog.Warnf("contact refresh failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// parseUserFromHeaders checks custom SIP headers for Telegram user info.
func (g *Gateway) parseUserFromHeaders(req sip.Request) (int64, bool) {
	if hdrs := req.GetHeaders("X-TG-ID"); len(hdrs) > 0 {
		if id, err := strconv.ParseInt(hdrs[0].Value(), 10, 64); err == nil {
			return id, true
		}
	}
	if hdrs := req.GetHeaders("X-TG-Username"); len(hdrs) > 0 {
		name := hdrs[0].Value()
		if id, ok := g.contacts.Resolve(strings.ToLower(name)); ok {
			return id, true
		}
		if id, ok := g.contacts.SearchAndAdd(g.tgClient, name); ok {
			return id, true
		}
	}
	if hdrs := req.GetHeaders("X-TG-Phone"); len(hdrs) > 0 {
		phone := hdrs[0].Value()
		if id, ok := g.contacts.Resolve(phone); ok {
			return id, true
		}
		if id, ok := g.contacts.SearchAndAdd(g.tgClient, phone); ok {
			return id, true
		}
	}
	return 0, false
}

// resolveUser resolves extension patterns like tg#username, +phone or numeric ID.
func (g *Gateway) resolveUser(ext string) (int64, bool) {
	if ext == "" {
		return 0, false
	}
	switch {
	case strings.HasPrefix(ext, "tg#"):
		name := ext[3:]
		if id, ok := g.contacts.Resolve(strings.ToLower(name)); ok {
			return id, true
		}
		return g.contacts.SearchAndAdd(g.tgClient, name)
	case strings.HasPrefix(ext, "+"):
		phone := ext[1:]
		if id, ok := g.contacts.Resolve(phone); ok {
			return id, true
		}
		return g.contacts.SearchAndAdd(g.tgClient, phone)
	default:
		if id, err := strconv.ParseInt(ext, 10, 64); err == nil {
			return id, true
		}
		if id, ok := g.contacts.Resolve(ext); ok {
			return id, true
		}
		return g.contacts.SearchAndAdd(g.tgClient, ext)
	}
}

// handleTelegramCall processes incoming Telegram call updates and dials SIP.
func (g *Gateway) handleTelegramCall(u *client.UpdateCall) {
	if u.Call.IsOutgoing {
		return
	}
	if _, ok := u.Call.State.(*client.CallStatePending); !ok {
		return
	}
	if err := acceptTelegramCall(g.tgClient, u.Call.ID); err != nil {
		coreLog.Warnf("acceptCall failed: %v", err)
		return
	}
	user, err := g.tgClient.GetUser(&client.GetUserRequest{UserId: u.Call.UserId})
	if err != nil {
		coreLog.Warnf("getUser failed: %v", err)
		return
	}
	headers := buildUserHeaders(u.Call.ID, user)
	if err := g.sipClient.Dial(context.Background(), "tg", g.callback, headers); err != nil {
		coreLog.Warnf("SIP dial failed: %v", err)
	}
}

// processInternalEvent advances call state machine and handles terminal transitions.
func (g *Gateway) processInternalEvent(ev internalEvent) {
	g.mu.Lock()
	ctx := g.calls[ev.ctxID]
	g.mu.Unlock()
	if ctx == nil {
		return
	}
	switch ev.typ {
	case evIncoming:
		ctx.State = StateIncoming
	case evOutgoing:
		ctx.State = StateOutgoing
	case evWaitMedia:
		ctx.State = StateWaitMedia
	case evWaitDTMF:
		ctx.State = StateWaitDTMF
	case evCleanup:
		ctx.State = StateCleanup
		g.cleanUp(ctx)
		g.mu.Lock()
		delete(g.calls, ev.ctxID)
		g.mu.Unlock()
	}
}

// cleanUp stops controllers and hangs up on both sides.
func (g *Gateway) cleanUp(ctx *Context) {
	if ctx.Controller != nil {
		ctx.Controller.Stop()
	}
	if ctx.SIPCallID != "" {
		_ = g.sipClient.Hangup(context.Background(), ctx.SIPCallID)
	}
	if ctx.TGCallID != 0 {
		if err := discardTelegramCall(g.tgClient, ctx.TGCallID); err != nil {
			coreLog.Warnf("discard telegram call failed: %v", err)
		}
	}
}

// buildUserHeaders builds SIP headers with Telegram user info.
func buildUserHeaders(callID int64, u *client.User) map[string]string {
	headers := map[string]string{
		"X-GW-Context": fmt.Sprintf("%d", callID),
		"X-TG-ID":      fmt.Sprintf("%d", u.Id),
	}
	if u.FirstName != "" {
		headers["X-TG-FirstName"] = u.FirstName
	}
	if u.LastName != "" {
		headers["X-TG-LastName"] = u.LastName
	}
	if u.Username != "" {
		headers["X-TG-Username"] = u.Username
	}
	if u.PhoneNumber != "" {
		headers["X-TG-Phone"] = u.PhoneNumber
	}
	return headers
}

// handleInvite creates a call context and emits a call state event.
func (g *Gateway) handleInvite(req sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().String()
	coreLog.Infof("received SIP INVITE: %s -> %s", req.From(), req.To())

	to, _ := req.To()
	ext := ""
	if to != nil && to.Address != nil {
		if u := to.Address.User(); u != nil {
			ext = u.String()
		}
	}

	userID, ok := g.parseUserFromHeaders(req)
	if !ok {
		userID, ok = g.resolveUser(ext)
	}
	if !ok {
		coreLog.Warnf("unknown extension %s", ext)
		if tx != nil {
			g.sipServer.RespondOnRequest(req, sip.StatusNotFound, "Not Found", "", nil)
		}
		return
	}

	if err := createTelegramCall(g.tgClient, userID); err != nil {
		coreLog.Warnf("createCall failed: %v", err)
	}

	ctx := &Context{ID: callID, SIPCallID: callID, UserID: userID, State: StateIncoming}

	g.mu.Lock()
	g.calls[callID] = ctx
	g.mu.Unlock()

	g.events <- CallStateEvent{CallID: callID, State: "incoming"}
	g.internalEvents <- internalEvent{ctxID: callID, typ: evIncoming}

	if tx != nil {
		g.sipServer.RespondOnRequest(req, sip.StatusTrying, "Trying", "", nil)
	}
}

// handleAck emits an answered state for an existing call.
func (g *Gateway) handleAck(req sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().String()
	coreLog.Infof("received SIP ACK: %s", callID)
	g.events <- CallStateEvent{CallID: callID, State: "answered"}
	g.internalEvents <- internalEvent{ctxID: callID, typ: evWaitMedia}
}

// handleBye cleans up the call context and emits an ended state.
func (g *Gateway) handleBye(req sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().String()
	coreLog.Infof("received SIP BYE: %s", callID)
	g.events <- CallStateEvent{CallID: callID, State: "ended"}
	g.internalEvents <- internalEvent{ctxID: callID, typ: evCleanup}
	if tx != nil {
		g.sipServer.RespondOnRequest(req, sip.StatusOK, "OK", "", nil)
	}
}

// handleInfo emits a media event for INFO requests.
func (g *Gateway) handleInfo(req sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().String()
	body := ""
	if b := req.Body(); b != nil {
		body = b.String()
	}
	coreLog.Infof("received SIP INFO: %s", callID)
	g.events <- MediaEvent{CallID: callID, Body: body}
	g.internalEvents <- internalEvent{ctxID: callID, typ: evWaitDTMF}
	if tx != nil {
		g.sipServer.RespondOnRequest(req, sip.StatusOK, "OK", "", nil)
	}
}

// startGateway initializes and starts the gateway component.
func startGateway(cfg *ini.File) error {
	coreLog.Info("starting gateway")
	callback := cfg.Section("sip").Key("callback_uri").String()
	gw := NewGateway(sipServer, tgClient, callback)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return gw.Start(ctx)
}
