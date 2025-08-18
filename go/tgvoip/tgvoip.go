package tgvoip

// Endpoint describes a remote audio endpoint.
type Endpoint struct {
	ID   int64
	IPv4 string
	Port int
}

// DSPOptions groups audio processing flags.
type DSPOptions struct {
	EchoCancellation bool
	NoiseSuppression bool
	AutoGain         bool
}

// Controller represents a tgvoip call instance.
type Controller interface {
	Configure(key []byte, endpoints []Endpoint, opts DSPOptions) error
	SetAudioCallbacks(input func([]int16), output func([]int16))
	Close()
}

// NewController creates a new tgvoip controller instance.
func NewController() Controller {
	return newController()
}

// ConnectSIPMedia links SIP media streams with a tgvoip controller.
func ConnectSIPMedia(ctrl Controller, in func([]int16), out func([]int16)) {
	ctrl.SetAudioCallbacks(in, out)
}
