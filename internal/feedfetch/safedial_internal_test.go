package feedfetch

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPublic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want bool
		why  string
	}{
		// The reason this guard exists: cloud instance metadata.
		{addr: "169.254.169.254", want: false, why: "link-local, cloud metadata endpoint"},
		{addr: "::ffff:169.254.169.254", want: false, why: "the same address as IPv4-mapped IPv6"},
		{addr: "fe80::1", want: false, why: "IPv6 link-local"},

		{addr: "127.0.0.1", want: false, why: "loopback"},
		{addr: "::1", want: false, why: "IPv6 loopback"},
		{addr: "0.0.0.0", want: false, why: "unspecified"},
		{addr: "::", want: false, why: "IPv6 unspecified"},

		{addr: "10.0.0.5", want: false, why: "RFC 1918"},
		{addr: "172.16.0.1", want: false, why: "RFC 1918"},
		{addr: "172.31.255.255", want: false, why: "RFC 1918 upper bound"},
		{addr: "192.168.1.1", want: false, why: "RFC 1918"},
		{addr: "fd00::1", want: false, why: "IPv6 unique local"},
		{addr: "100.64.0.1", want: false, why: "carrier-grade NAT"},
		{addr: "198.18.0.1", want: false, why: "benchmarking range"},
		{addr: "224.0.0.1", want: false, why: "multicast"},

		// Public addresses must still be reachable, or the scraper is useless.
		{addr: "93.184.216.34", want: true, why: "public IPv4"},
		{addr: "8.8.8.8", want: true, why: "public IPv4"},
		{addr: "2606:2800:220:1:248:1893:25c8:1946", want: true, why: "public IPv6"},
		{addr: "172.32.0.1", want: true, why: "just outside RFC 1918"},
		{addr: "172.15.255.255", want: true, why: "just below RFC 1918"},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()

			addr, err := netip.ParseAddr(tt.addr)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, isPublic(addr), tt.why)
		})
	}
}

func TestIsPublicRejectsInvalidAddress(t *testing.T) {
	t.Parallel()

	assert.False(t, isPublic(netip.Addr{}), "the zero address must never be dialled")
}

func TestGuardAddressRejectsUnparseableTarget(t *testing.T) {
	t.Parallel()

	assert.ErrorIs(t, guardAddress("tcp", "not-an-address", nil), ErrBlockedAddress)
	assert.ErrorIs(t, guardAddress("tcp", "example.com:80", nil), ErrBlockedAddress)
}

func TestGuardAddressAllowsPublicTarget(t *testing.T) {
	t.Parallel()

	assert.NoError(t, guardAddress("tcp", "93.184.216.34:443", nil))
}
