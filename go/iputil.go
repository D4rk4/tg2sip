package main

import (
	"fmt"
	"net"
)

// detectHostIP returns the first global unicast IPv4 address of the host.
//
// The previous implementation relied on net.InterfaceAddrs which could return
// addresses from down or loopback interfaces and even the unspecified
// "0.0.0.0" address.  That resulted in selecting an unusable source address
// such as 127.0.0.1 when sending SIP messages over UDP, causing failures like
// "sendto: invalid argument".  We now iterate over the network interfaces,
// selecting the first address that is up, not loopback and globally routable.
func detectHostIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			if !ip4.IsGlobalUnicast() {
				continue
			}
			return ip4.String(), nil
		}
	}
	return "", fmt.Errorf("no non-loopback IPv4 address found")
}
