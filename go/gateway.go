package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	gosip "github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/sip"
	client "github.com/zelenin/go-tdlib/client"
)

// Gateway connects SIP server and Telegram client.
type Gateway struct {
	sipServer gosip.Server
	tgClient  *client.Client
}

// NewGateway creates a new Gateway instance.
func NewGateway(sipSrv gosip.Server, tgCl *client.Client) *Gateway {
	return &Gateway{sipServer: sipSrv, tgClient: tgCl}
}

// Start runs the gateway until ctx is canceled.
func (g *Gateway) Start(ctx context.Context) error {
	if err := g.sipServer.OnRequest(sip.INVITE, g.handleInvite); err != nil {
		return err
	}

	listener := g.tgClient.GetListener()
	go func() {
		for update := range listener.Updates {
			switch u := update.(type) {
			case *client.UpdateCall:
				coreLog.Infof("received telegram call update: %d", u.Call.ID)
			}
		}
	}()

	<-ctx.Done()
	return nil
}

// handleInvite logs incoming SIP INVITE requests and responds with 501.
func (g *Gateway) handleInvite(req sip.Request, tx sip.ServerTransaction) {
	coreLog.Infof("received SIP INVITE: %s -> %s", req.From(), req.To())
	if tx != nil {
		g.sipServer.RespondOnRequest(req, sip.StatusNotImplemented, "Not implemented", "", nil)
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
