//go:build tgvoip

package tgvoip

/*
#cgo CXXFLAGS: -std=c++17 -I../../libtgvoip
#cgo LDFLAGS: -L../../libtgvoip -ltgvoip -lstdc++ -lm -lopus
#include <stdlib.h>
#include "VoIPController.h"
#include "NetworkSocket.h"
#include "logging.h"
#include <vector>

using tgvoip::VoIPController;
using tgvoip::Endpoint;

struct tgvoip_endpoint {
    long long id;
    char* ip;
    int port;
};

struct tgvoip_dsp {
    int aec;
    int ns;
    int agc;
};

static VoIPController* tgvoip_new() {
    return new VoIPController();
}

static void tgvoip_free(VoIPController* c) {
    delete c;
}

static void tgvoip_configure(VoIPController* c, char* key, int keyLen, struct tgvoip_endpoint* eps, int epCount, struct tgvoip_dsp dsp) {
    if(key && keyLen>0){
        c->SetEncryptionKey(key, true);
    }
    std::vector<Endpoint> vec;
    for(int i=0;i<epCount;i++){
        tgvoip::IPv4Address v4(std::string(eps[i].ip));
        tgvoip::IPv6Address v6;
        Endpoint e(eps[i].id, eps[i].port, v4, v6, Endpoint::UDP_P2P_INET, NULL);
        vec.push_back(e);
    }
    c->SetRemoteEndpoints(vec, true, 92);
    c->SetEchoCancellationStrength(dsp.aec);
}

extern void goInputCallback(int16_t* data, size_t length, void* user);
extern void goOutputCallback(int16_t* data, size_t length, void* user);

static void tgvoip_set_callbacks(VoIPController* c, void* user) {
    c->SetAudioDataCallbacks(
        [user](int16_t* data, size_t len){ goInputCallback(data, len, user); },
        [user](int16_t* data, size_t len){ goOutputCallback(data, len, user); }
    );
}

extern void goTgvoipLog(char level, const char* msg);
static void tgvoip_init_logger() { tgvoip_set_log_callback(goTgvoipLog); }
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"
)

type controller struct {
	ptr    *C.VoIPController
	handle cgo.Handle
}

func newController() Controller {
	return &controller{ptr: C.tgvoip_new()}
}

func (c *controller) Configure(key []byte, endpoints []Endpoint, opts DSPOptions) error {
	var keyPtr *C.char
	if len(key) > 0 {
		keyPtr = (*C.char)(unsafe.Pointer(&key[0]))
	}
	eps := make([]C.struct_tgvoip_endpoint, len(endpoints))
	for i, e := range endpoints {
		eps[i].id = C.longlong(e.ID)
		eps[i].ip = C.CString(e.IPv4)
		eps[i].port = C.int(e.Port)
		defer C.free(unsafe.Pointer(eps[i].ip))
	}
	dsp := C.struct_tgvoip_dsp{aec: toCInt(opts.EchoCancellation), ns: toCInt(opts.NoiseSuppression), agc: toCInt(opts.AutoGain)}
	var epPtr *C.struct_tgvoip_endpoint
	if len(eps) > 0 {
		epPtr = (*C.struct_tgvoip_endpoint)(unsafe.Pointer(&eps[0]))
	}
	C.tgvoip_configure(c.ptr, keyPtr, C.int(len(key)), epPtr, C.int(len(eps)), dsp)
	return nil
}

func toCInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

type audioCallbacks struct {
	in  func([]int16)
	out func([]int16)
}

//export goInputCallback
func goInputCallback(data *C.int16_t, length C.size_t, user unsafe.Pointer) {
	h := cgo.Handle(user)
	cb := h.Value().(audioCallbacks)
	slice := unsafe.Slice((*int16)(unsafe.Pointer(data)), int(length))
	if cb.in != nil {
		cb.in(slice)
	}
}

//export goOutputCallback
func goOutputCallback(data *C.int16_t, length C.size_t, user unsafe.Pointer) {
	h := cgo.Handle(user)
	cb := h.Value().(audioCallbacks)
	slice := unsafe.Slice((*int16)(unsafe.Pointer(data)), int(length))
	if cb.out != nil {
		cb.out(slice)
	}
}

func (c *controller) SetAudioCallbacks(input func([]int16), output func([]int16)) {
	cb := audioCallbacks{in: input, out: output}
	c.handle = cgo.NewHandle(cb)
	C.tgvoip_set_callbacks(c.ptr, unsafe.Pointer(c.handle))
}

func (c *controller) Close() {
	C.tgvoip_free(c.ptr)
	if c.handle != 0 {
		c.handle.Delete()
	}
}

//export goTgvoipLog
func goTgvoipLog(level C.char, msg *C.char) {
	if logCallback != nil {
		logCallback(byte(level), C.GoString(msg))
	}
}

func init() {
	C.tgvoip_init_logger()
}
