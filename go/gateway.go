package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	gosip "github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/sip"
	client "github.com/zelenin/go-tdlib/client"
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
	blockUntil     time.Time
	extraWait      time.Duration
	peerFlood      time.Duration
}

// NewGateway creates a new Gateway instance.
func NewGateway(sipSrv gosip.Server, tgCl *client.Client, callback string, extraWait, peerFlood time.Duration) *Gateway {
	return &Gateway{
		sipServer:      sipSrv,
		tgClient:       tgCl,
		sipClient:      NewSIPClient(sipSrv),
		events:         make(chan interface{}, 16),
		internalEvents: make(chan internalEvent, 16),
		calls:          make(map[string]*Context),
		contacts:       NewContactCache(),
		callback:       callback,
		extraWait:      extraWait,
		peerFlood:      peerFlood,
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

// dtmfRegex matches valid DTMF digit sequences.
var dtmfRegex = regexp.MustCompile(`^[0-9A-D*#]+$`)

var retryAfterRe = regexp.MustCompile(`(?i)retry after (\d+)`)
var peerFloodRe = regexp.MustCompile(`(?i)PEER_FLOOD`)

func parseFloodError(err error) (time.Duration, bool, bool) {
	var respErr client.ResponseError
	if !errors.As(err, &respErr) {
		return 0, false, false
	}
	if m := retryAfterRe.FindStringSubmatch(respErr.Err.Message); len(m) > 1 {
		if s, e := strconv.Atoi(m[1]); e == nil {
			return time.Duration(s) * time.Second, false, true
		}
	}
	if peerFloodRe.MatchString(respErr.Err.Message) {
		return 0, true, true
	}
	return 0, false, false
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
				coreLog.Infof("received telegram call update: %d", u.Call.Id)
				g.handleTelegramCall(u)
			case *client.UpdateUser:
				g.contacts.Update(u.User)
			case *client.UpdateNewMessage:
				g.handleTelegramMessage(u)
			}
		case ev := <-g.events:
			coreLog.Infof("received gateway event: %#v", ev)
		case ie := <-g.internalEvents:
			g.processInternalEvent(ie)
		case <-ctx.Done():
			g.mu.Lock()
			for _, c := range g.calls {
				g.cleanUp(c)
			}
			g.mu.Unlock()
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
func (g *Gateway) parseUserFromHeaders(req sip.Request) (int64, bool, error) {
	if hdrs := req.GetHeaders("X-TG-ID"); len(hdrs) > 0 {
		if id, err := strconv.ParseInt(hdrs[0].Value(), 10, 64); err == nil {
			return id, true, nil
		} else {
			return 0, false, err
		}
	}
	if hdrs := req.GetHeaders("X-TG-Username"); len(hdrs) > 0 {
		return g.resolveUser("tg#" + hdrs[0].Value())
	}
	if hdrs := req.GetHeaders("X-TG-Phone"); len(hdrs) > 0 {
		return g.resolveUser("+" + hdrs[0].Value())
	}
	return 0, false, nil
}

// resolveUser resolves extension patterns like tg#username, +phone or numeric ID.
func (g *Gateway) resolveUser(ext string) (int64, bool, error) {
	if ext == "" {
		return 0, false, nil
	}
	switch {
	case strings.HasPrefix(ext, "tg#"):
		name := ext[3:]
		if id, ok := g.contacts.Resolve(strings.ToLower(name)); ok {
			return id, true, nil
		}
		return g.contacts.SearchAndAdd(g.tgClient, name)
	case strings.HasPrefix(ext, "+"):
		phone := ext[1:]
		if id, ok := g.contacts.Resolve(phone); ok {
			return id, true, nil
		}
		return g.contacts.SearchAndAdd(g.tgClient, phone)
	default:
		if id, err := strconv.ParseInt(ext, 10, 64); err == nil {
			return id, true, nil
		}
		if id, ok := g.contacts.Resolve(ext); ok {
			return id, true, nil
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
	if err := acceptTelegramCall(g.tgClient, u.Call.Id); err != nil {
		coreLog.Warnf("acceptCall failed: %v", err)
		return
	}
	user, err := g.tgClient.GetUser(&client.GetUserRequest{UserId: u.Call.UserId})
	if err != nil {
		coreLog.Warnf("getUser failed: %v", err)
		return
	}
	headers := buildUserHeaders(u.Call.Id, user)
	if err := g.sipClient.Dial(context.Background(), "tg", g.callback, headers); err != nil {
		coreLog.Warnf("SIP dial failed: %v", err)
	}
}

// handleTelegramMessage processes incoming Telegram text messages for DTMF digits.
func (g *Gateway) handleTelegramMessage(u *client.UpdateNewMessage) {
	msg := u.Message
	content, ok := msg.Content.(*client.MessageText)
	if !ok || content.Text == nil {
		return
	}
	text := strings.ToUpper(strings.TrimSpace(content.Text.Text))
	if text == "" || !dtmfRegex.MatchString(text) {
		return
	}
	if len(text) > 32 {
		text = text[:32]
	}
	sender, ok := msg.SenderId.(*client.MessageSenderUser)
	if !ok {
		return
	}
	var ctx *Context
	g.mu.Lock()
	for _, c := range g.calls {
		if c.UserID == sender.UserId && c.State == StateWaitDTMF {
			ctx = c
			break
		}
	}
	g.mu.Unlock()
	if ctx != nil {
		_ = g.sipClient.DialDtmf(context.Background(), ctx.SIPCallID, text)
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
	if uname := getUsername(u); uname != "" {
		headers["X-TG-Username"] = uname
	}
	if u.PhoneNumber != "" {
		headers["X-TG-Phone"] = u.PhoneNumber
	}
	return headers
}

// handleInvite creates a call context and emits a call state event.
func (g *Gateway) handleInvite(req sip.Request, tx sip.ServerTransaction) {
	cid, _ := req.CallID()
	callID := ""
	if cid != nil {
		callID = cid.String()
	}
	fromHdr, _ := req.From()
	toHdr, _ := req.To()
	coreLog.Infof("received SIP INVITE: %s -> %s", fromHdr, toHdr)

	now := time.Now()
	if !g.blockUntil.IsZero() {
		if now.After(g.blockUntil) {
			g.blockUntil = time.Time{}
		} else {
			wait := int(g.blockUntil.Sub(now).Seconds())
			coreLog.Warnf("dropping call due to temp TG block for %d seconds", wait)
			if tx != nil {
				g.sipServer.RespondOnRequest(req, sip.StatusServiceUnavailable,
					fmt.Sprintf("FLOOD_WAIT %d", wait), "", nil)
			}
			return
		}
	}

	ext := ""
	if toHdr != nil && toHdr.Address != nil {
		if u := toHdr.Address.User(); u != nil {
			ext = u.String()
		}
	}

	userID, ok, err := g.parseUserFromHeaders(req)
	if err != nil {
		if wait, peer, matched := parseFloodError(err); matched {
			if peer {
				wait = g.peerFlood
			}
			wait += g.extraWait
			g.blockUntil = time.Now().Add(wait)
			if tx != nil {
				g.sipServer.RespondOnRequest(req, sip.StatusServiceUnavailable,
					fmt.Sprintf("FLOOD_WAIT %d", int(wait.Seconds())), "", nil)
			}
			return
		}
		coreLog.Warnf("parse headers failed: %v", err)
		if tx != nil {
			g.sipServer.RespondOnRequest(req, sip.StatusInternalServerError, "Internal error", "", nil)
		}
		return
	}
	if !ok {
		userID, ok, err = g.resolveUser(ext)
		if err != nil {
			if wait, peer, matched := parseFloodError(err); matched {
				if peer {
					wait = g.peerFlood
				}
				wait += g.extraWait
				g.blockUntil = time.Now().Add(wait)
				if tx != nil {
					g.sipServer.RespondOnRequest(req, sip.StatusServiceUnavailable,
						fmt.Sprintf("FLOOD_WAIT %d", int(wait.Seconds())), "", nil)
				}
				return
			}
			coreLog.Warnf("resolve user failed: %v", err)
			if tx != nil {
				g.sipServer.RespondOnRequest(req, sip.StatusInternalServerError, "Internal error", "", nil)
			}
			return
		}
	}
	if !ok {
		coreLog.Warnf("unknown extension %s", ext)
		if tx != nil {
			g.sipServer.RespondOnRequest(req, sip.StatusNotFound, "Not Found", "", nil)
		}
		return
	}

	if err := createTelegramCall(g.tgClient, userID); err != nil {
		if wait, peer, matched := parseFloodError(err); matched {
			if peer {
				wait = g.peerFlood
			}
			wait += g.extraWait
			g.blockUntil = time.Now().Add(wait)
			if tx != nil {
				g.sipServer.RespondOnRequest(req, sip.StatusServiceUnavailable,
					fmt.Sprintf("FLOOD_WAIT %d", int(wait.Seconds())), "", nil)
			}
			return
		}
		coreLog.Warnf("createCall failed: %v", err)
	} else {
		g.blockUntil = time.Time{}
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
	cid, _ := req.CallID()
	callID := ""
	if cid != nil {
		callID = cid.String()
	}
	coreLog.Infof("received SIP ACK: %s", callID)
	g.events <- CallStateEvent{CallID: callID, State: "answered"}
	g.internalEvents <- internalEvent{ctxID: callID, typ: evWaitMedia}
}

// handleBye cleans up the call context and emits an ended state.
func (g *Gateway) handleBye(req sip.Request, tx sip.ServerTransaction) {
	cid, _ := req.CallID()
	callID := ""
	if cid != nil {
		callID = cid.String()
	}
	coreLog.Infof("received SIP BYE: %s", callID)
	g.events <- CallStateEvent{CallID: callID, State: "ended"}
	g.internalEvents <- internalEvent{ctxID: callID, typ: evCleanup}
	if tx != nil {
		g.sipServer.RespondOnRequest(req, sip.StatusOK, "OK", "", nil)
	}
}

// handleInfo emits a media event for INFO requests.
func (g *Gateway) handleInfo(req sip.Request, tx sip.ServerTransaction) {
	cid, _ := req.CallID()
	callID := ""
	if cid != nil {
		callID = cid.String()
	}
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
func startGateway(ctx context.Context, cfg *Settings) error {
	coreLog.Info("starting gateway")
	callback := cfg.CallbackURI()
	gw := NewGateway(sipServer, tgClient, callback, cfg.ExtraWaitTime(), cfg.PeerFloodTime())
	return gw.Start(ctx)
}
