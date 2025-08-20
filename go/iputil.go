package main

import (
	"fmt"
	"net"
)

// detectHostIP returns the first non-loopback IPv4 address of the host.
func detectHostIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			// Ensure we never return an address from the 127.0.0.0/8
			// loopback range.
			if ip4.IsLoopback() || ip4[0] == 127 {
				continue
			}
			return ip4.String(), nil
		}
	}
	return "", fmt.Errorf("no non-loopback IPv4 address found")
}
