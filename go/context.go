package main

// Controller represents an external media controller.
type Controller interface {
	Stop()
}

// CallState represents states of a bridged call.
type CallState int

const (
	StateIncoming CallState = iota
	StateOutgoing
	StateWaitMedia
	StateWaitDTMF
	StateCleanup
)

// Context holds state for a single bridged call.
type Context struct {
	ID         string
	SIPCallID  string
	TGCallID   int64
	UserID     int64
	Controller Controller
	State      CallState
}

// internalEventType enumerates internal gateway events.
type internalEventType int

const (
	evIncoming internalEventType = iota
	evOutgoing
	evWaitMedia
	evWaitDTMF
	evCleanup
)

type internalEvent struct {
	ctxID string
	typ   internalEventType
}
