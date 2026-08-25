package feedfetch

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

// ErrBlockedAddress is returned when a feed resolves to an address the fetcher
// refuses to connect to.
var ErrBlockedAddress = errors.New("blocked address")

// Extra ranges that netip's own predicates do not cover but that must never be
// reachable from a user-supplied feed URL.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),   // RFC 6598 carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // RFC 6890 IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),   // RFC 2544 benchmarking
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("::/128"),          // unspecified
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64, can embed private IPv4
}

// isPublic reports whether an address is on the public internet.
func isPublic(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	// An IPv4-mapped IPv6 address must be judged on its IPv4 value, otherwise
	// ::ffff:169.254.169.254 would slip past the predicates below.
	if addr.Is4In6() {
		addr = addr.Unmap()
	}

	switch {
	case addr.IsLoopback(),
		addr.IsPrivate(),
		addr.IsLinkLocalUnicast(),
		addr.IsLinkLocalMulticast(),
		addr.IsInterfaceLocalMulticast(),
		addr.IsMulticast(),
		addr.IsUnspecified():
		return false
	}

	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// guardAddress rejects a dial target that is not on the public internet.
//
// It is wired into net.Dialer.Control, which runs after DNS resolution with the
// concrete address about to be connected to. Checking there rather than on the
// URL is what makes this resistant to a hostname that resolves to a private
// address, to DNS rebinding, and to redirects: every connection the client
// makes passes through it, whatever produced the URL.
func guardAddress(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: cannot parse dial address %q", ErrBlockedAddress, address)
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: cannot parse dial address %q", ErrBlockedAddress, host)
	}

	if !isPublic(addr) {
		return fmt.Errorf("%w: %s is not a public address", ErrBlockedAddress, addr)
	}
	return nil
}
