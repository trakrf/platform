// Package emailcheck answers one narrow question: can this address's domain
// receive mail at all?
//
// It is a first-level typo guard (TRA-958), not verification. It catches the
// biggest class of mistyped addresses — a domain that does not exist, or
// publishes no mail route — synchronously at submit time, which is the same
// class a hard bounce would catch hours later and without any webhook
// infrastructure. It cannot tell you the address belongs to the person typing
// it; only a click-through confirmation does that.
package emailcheck

import (
	"context"
	"errors"
	"net"
	"strings"
)

// ErrDomainUndeliverable means the domain definitively cannot receive mail:
// it does not resolve, or it publishes a null MX (RFC 7505).
var ErrDomainUndeliverable = errors.New("email domain cannot receive mail")

// reservedTestDomains are RFC 2606 / RFC 6761 names reserved for documentation
// and testing. Every fixture and e2e account in this repo uses one.
var reservedTestDomains = map[string]struct{}{
	"example.com": {},
	"example.net": {},
	"example.org": {},
}

var reservedTestSuffixes = []string{".test", ".invalid", ".example"}

// IsReservedTestDomain reports whether addr belongs to a reserved
// documentation/testing domain.
//
// These addresses are treated as fixtures throughout: real sends are stubbed
// rather than dispatched, and deliverability is not checked. That pairing is
// deliberate — example.com publishes a null MX (RFC 7505), so a live check
// would reject the very addresses the test suite is built on.
func IsReservedTestDomain(addr string) bool {
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(addr[at+1:]))
	if _, ok := reservedTestDomains[domain]; ok {
		return true
	}
	for _, s := range reservedTestSuffixes {
		if strings.HasSuffix(domain, s) {
			return true
		}
	}
	for d := range reservedTestDomains {
		if strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

// Resolver is the DNS seam. *net.Resolver satisfies it.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// DomainDeliverable reports whether addr's domain can receive mail.
//
// Fail-closed on a definitive negative (NXDOMAIN, null MX, no mail route).
// Fail-open on anything transient — a timeout, a SERVFAIL, a resolver that is
// briefly unreachable — because a flaky nameserver must never block someone
// from fixing their own address. A guard that locks people out when DNS
// hiccups is worse than the typo it prevents.
func DomainDeliverable(ctx context.Context, addr string, r Resolver) error {
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return ErrDomainUndeliverable
	}
	domain := strings.ToLower(strings.TrimSpace(addr[at+1:]))
	if domain == "" {
		return ErrDomainUndeliverable
	}

	// Fixtures are exempt. example.com publishes a null MX, so checking it for
	// real would reject every test and e2e account in the repo — the same
	// addresses whose sends are already stubbed rather than dispatched.
	if IsReservedTestDomain(addr) {
		return nil
	}

	mxs, err := r.LookupMX(ctx, domain)
	if err != nil {
		if isDefinitiveMiss(err) {
			// No such domain — but only trust that after confirming there is
			// no A/AAAA record either, since some resolvers report a missing
			// MX RRset the same way as a missing domain.
			return hostFallback(ctx, domain, r)
		}
		// Transient: give the caller the benefit of the doubt.
		return nil
	}

	// RFC 7505: a single "." target is an explicit "this domain accepts no mail".
	if len(mxs) == 1 && strings.TrimSuffix(mxs[0].Host, ".") == "" {
		return ErrDomainUndeliverable
	}
	if len(mxs) > 0 {
		return nil
	}

	// No MX records and no error: implicit MX, where the A record is the mail
	// host. Legitimate for plenty of small domains.
	return hostFallback(ctx, domain, r)
}

func hostFallback(ctx context.Context, domain string, r Resolver) error {
	hosts, err := r.LookupHost(ctx, domain)
	if err != nil {
		if isDefinitiveMiss(err) {
			return ErrDomainUndeliverable
		}
		return nil
	}
	if len(hosts) == 0 {
		return ErrDomainUndeliverable
	}
	return nil
}

// isDefinitiveMiss distinguishes "this name does not exist" from "I could not
// reach a nameserver right now".
func isDefinitiveMiss(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout || dnsErr.IsTemporary {
			return false
		}
		return dnsErr.IsNotFound
	}
	return false
}
