package signal

import (
	"testing"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
)

func TestHasLocalSenderKey(t *testing.T) {
	const distID = "72b4a323-25b0-434a-a664-9a39d907a88f"
	ss := memstore.NewSignalStores()
	c := &Client{
		acct:   &account.Account{ACI: "00000000-0000-4000-8000-000000000001", DeviceID: 1},
		stores: ss,
	}

	hasKey, err := c.hasLocalSenderKey(distID)
	if err != nil {
		t.Fatalf("hasLocalSenderKey missing: %v", err)
	}
	if hasKey {
		t.Fatal("expected no sender key before bootstrap")
	}

	addr := store.Address{ServiceID: c.acct.ACI, DeviceID: c.acct.DeviceID}
	if err := ss.StoreSenderKey(addr, distID, []byte("record")); err != nil {
		t.Fatal(err)
	}

	hasKey, err = c.hasLocalSenderKey(distID)
	if err != nil {
		t.Fatalf("hasLocalSenderKey present: %v", err)
	}
	if !hasKey {
		t.Fatal("expected sender key after store")
	}
}

func TestGroupHasOtherMembers(t *testing.T) {
	self := "00000000-0000-4000-8000-000000000001"
	c := &Client{acct: &account.Account{ACI: self}}

	solo := &Group{Members: []GroupMember{{ACI: self}}}
	if c.groupHasOtherMembers(solo) {
		t.Fatal("solo group should have no other members")
	}

	withPeer := &Group{Members: []GroupMember{{ACI: self}, {ACI: "00000000-0000-4000-8000-000000000002"}}}
	if !c.groupHasOtherMembers(withPeer) {
		t.Fatal("expected other member")
	}
}
