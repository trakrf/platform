package emailcheck

import (
	"context"
	"errors"
	"net"
	"testing"
)

type fakeResolver struct {
	mxs      []*net.MX
	mxErr    error
	hosts    []string
	hostErr  error
	mxCalls  int
	hstCalls int
}

func (f *fakeResolver) LookupMX(_ context.Context, _ string) ([]*net.MX, error) {
	f.mxCalls++
	return f.mxs, f.mxErr
}

func (f *fakeResolver) LookupHost(_ context.Context, _ string) ([]string, error) {
	f.hstCalls++
	return f.hosts, f.hostErr
}

func notFound() error  { return &net.DNSError{Err: "no such host", IsNotFound: true} }
func timedOut() error  { return &net.DNSError{Err: "i/o timeout", IsTimeout: true} }
func temporary() error { return &net.DNSError{Err: "server misbehaving", IsTemporary: true} }

func TestDomainDeliverable(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		resolver *fakeResolver
		wantErr  error
	}{
		{
			name:     "domain with MX records is deliverable",
			addr:     "someone@trakrf.id",
			resolver: &fakeResolver{mxs: []*net.MX{{Host: "mx1.trakrf.id."}}},
		},
		{
			name:     "nonexistent domain is rejected",
			addr:     "someone@exmaple-nope-tra958.com",
			resolver: &fakeResolver{mxErr: notFound(), hostErr: notFound()},
			wantErr:  ErrDomainUndeliverable,
		},
		{
			// RFC 7505 — the domain explicitly declines mail.
			name:     "null MX is rejected",
			addr:     "someone@no-mail-tra958.com",
			resolver: &fakeResolver{mxs: []*net.MX{{Host: "."}}},
			wantErr:  ErrDomainUndeliverable,
		},
		{
			// Plenty of small domains have no MX and take mail at their A record.
			name:     "no MX but an A record is deliverable",
			addr:     "someone@implicit-tra958.com",
			resolver: &fakeResolver{hosts: []string{"203.0.113.7"}},
		},
		{
			name:     "no MX and no host is rejected",
			addr:     "someone@nothing-tra958.com",
			resolver: &fakeResolver{},
			wantErr:  ErrDomainUndeliverable,
		},
		{
			// A flaky resolver must never block someone fixing their address.
			name:     "DNS timeout fails open",
			addr:     "someone@trakrf.id",
			resolver: &fakeResolver{mxErr: timedOut()},
		},
		{
			name:     "temporary DNS failure fails open",
			addr:     "someone@trakrf.id",
			resolver: &fakeResolver{mxErr: temporary()},
		},
		{
			name:     "host lookup timeout after missing MX fails open",
			addr:     "someone@trakrf.id",
			resolver: &fakeResolver{mxErr: notFound(), hostErr: timedOut()},
		},
		{
			name:     "address with no @ is rejected",
			addr:     "not-an-address",
			resolver: &fakeResolver{},
			wantErr:  ErrDomainUndeliverable,
		},
		{
			name:     "address with empty domain is rejected",
			addr:     "trailing@",
			resolver: &fakeResolver{},
			wantErr:  ErrDomainUndeliverable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DomainDeliverable(context.Background(), tt.addr, tt.resolver)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("DomainDeliverable() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DomainDeliverable() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// The real *net.Resolver must satisfy the seam, or the handler cannot use it.
func TestNetResolverSatisfiesResolver(t *testing.T) {
	var _ Resolver = net.DefaultResolver
}

// Reserved fixture domains must never be DNS-checked. example.com publishes a
// null MX (RFC 7505), so a live check rejects it — which would break every
// test and e2e account in this repo. The resolver must not even be consulted.
func TestDomainDeliverable_ExemptsReservedTestDomains(t *testing.T) {
	for _, addr := range []string{
		"fixture@example.com",
		"fixture@example.net",
		"fixture@example.org",
		"fixture@sub.example.com",
		"fixture@anything.test",
		"fixture@nope.invalid",
	} {
		r := &fakeResolver{mxErr: notFound(), hostErr: notFound()}
		if err := DomainDeliverable(context.Background(), addr, r); err != nil {
			t.Errorf("DomainDeliverable(%q) = %v, want nil", addr, err)
		}
		if r.mxCalls != 0 || r.hstCalls != 0 {
			t.Errorf("DomainDeliverable(%q) consulted DNS (mx=%d host=%d), want none",
				addr, r.mxCalls, r.hstCalls)
		}
	}
}

// A real domain is still checked — the exemption must not swallow everything.
func TestDomainDeliverable_RealDomainStillChecked(t *testing.T) {
	r := &fakeResolver{mxErr: notFound(), hostErr: notFound()}
	if err := DomainDeliverable(context.Background(), "someone@gmial-typo.com", r); !errors.Is(err, ErrDomainUndeliverable) {
		t.Fatalf("DomainDeliverable() = %v, want ErrDomainUndeliverable", err)
	}
	if r.mxCalls == 0 {
		t.Error("expected a real domain to be looked up")
	}
}
