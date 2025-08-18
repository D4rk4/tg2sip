//go:build !tgvoip

package tgvoip

// controller is a stub implementation used when tgvoip build tag is disabled.
type controller struct{}

func newController() Controller { return &controller{} }

func (c *controller) Configure(key []byte, endpoints []Endpoint, opts DSPOptions) error { return nil }

func (c *controller) SetAudioCallbacks(input func([]int16), output func([]int16)) {}

func (c *controller) Close() {}
