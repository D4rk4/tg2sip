package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	gosip "github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/sip"
	client "github.com/zelenin/go-tdlib/client"
)

// Gateway connects SIP server and Telegram client.
type Gateway struct {
	sipServer gosip.Server
	tgClient  *client.Client
	sipClient *SIPClient
	events    chan interface{}
	calls     map[string]*CallContext
	contacts  *ContactCache
	mu        sync.Mutex
}

// NewGateway creates a new Gateway instance.
func NewGateway(sipSrv gosip.Server, tgCl *client.Client) *Gateway {
	return &Gateway{
		sipServer: sipSrv,
		tgClient:  tgCl,
		sipClient: NewSIPClient(sipSrv),
		events:    make(chan interface{}, 16),
		calls:     make(map[string]*CallContext),
		contacts:  NewContactCache(),
	}
}

// CallContext stores basic SIP call information.
type CallContext struct {
	ID   string
	From string
	To   string
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
			case *client.UpdateUser:
				g.contacts.Update(u.User)
			}
		case ev := <-g.events:
			coreLog.Infof("received gateway event: %#v", ev)
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

	userID, ok := g.contacts.Resolve(ext)
	if !ok {
		if id, found := g.contacts.SearchAndAdd(g.tgClient, ext); found {
			userID = id
			ok = true
		}
	}
	if !ok {
		coreLog.Warnf("unknown extension %s", ext)
		if tx != nil {
			g.sipServer.RespondOnRequest(req, sip.StatusNotFound, "Not Found", "", nil)
		}
		return
	}

	protocol := &client.CallProtocol{UdpP2p: true, UdpReflector: true, MinLayer: 65, MaxLayer: 92}
	if _, err := g.tgClient.CreateCall(&client.CreateCallRequest{UserId: userID, Protocol: protocol}); err != nil {
		coreLog.Warnf("createCall failed: %v", err)
	}

	ctx := &CallContext{
		ID:   callID,
		From: req.From().String(),
		To:   req.To().String(),
	}

	g.mu.Lock()
	g.calls[callID] = ctx
	g.mu.Unlock()

	g.events <- CallStateEvent{CallID: callID, State: "incoming"}

	if tx != nil {
		g.sipServer.RespondOnRequest(req, sip.StatusTrying, "Trying", "", nil)
	}
}

// handleAck emits an answered state for an existing call.
func (g *Gateway) handleAck(req sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().String()
	coreLog.Infof("received SIP ACK: %s", callID)
	g.events <- CallStateEvent{CallID: callID, State: "answered"}
}

// handleBye cleans up the call context and emits an ended state.
func (g *Gateway) handleBye(req sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().String()
	coreLog.Infof("received SIP BYE: %s", callID)
	g.events <- CallStateEvent{CallID: callID, State: "ended"}
	g.mu.Lock()
	delete(g.calls, callID)
	g.mu.Unlock()
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
	if tx != nil {
		g.sipServer.RespondOnRequest(req, sip.StatusOK, "OK", "", nil)
	}
}

// startGateway initializes and starts the gateway component.
func startGateway() error {
	coreLog.Info("starting gateway")
	gw := NewGateway(sipServer, tgClient)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return gw.Start(ctx)
}
