package signal

import (
	"context"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/web"
)

func TestDiscoverContactsRequiresREST(t *testing.T) {
	c := &Client{acct: testAccount()}
	_, err := c.DiscoverContacts(context.Background(), []string{"+15551234567"})
	if err == nil {
		t.Fatal("expected error without web client")
	}
}

func TestDiscoverContactsEmptyInput(t *testing.T) {
	c := &Client{
		acct: testAccount(),
		webc: web.New("http://example.invalid", "test"),
	}
	result, err := c.DiscoverContacts(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contacts) != 0 {
		t.Fatalf("got %+v", result.Contacts)
	}
}

func TestEnsureCDSILifecycle(t *testing.T) {
	c := &Client{webc: web.New("", "signal-go-test")}
	tokio, cm, err := c.ensureCDSI()
	if err != nil {
		t.Fatal(err)
	}
	if tokio == nil || cm == nil {
		t.Fatal("expected non-nil runtime")
	}
	tokio2, cm2, err := c.ensureCDSI()
	if err != nil || tokio2 != tokio || cm2 != cm {
		t.Fatal("expected cached runtime")
	}
	c.closeCDSI()
}
