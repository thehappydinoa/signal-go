package prekeymaint

import (
	"context"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// DefaultMinAvailable is the floor under which we upload a new batch.
const DefaultMinAvailable = 10

// DefaultTopUpBatch is how many one-time EC + Kyber keys each upload adds.
const DefaultTopUpBatch = 100

// Maintainer uploads one-time prekey batches when the local store runs low.
type Maintainer struct {
	AccountStore account.Store
	SignalStores store.SignalStores
	Web          *web.Client

	MinAvailable int
	TopUpBatch   int
}

// NewMaintainer returns a maintainer with default thresholds.
func NewMaintainer(
	acctStore account.Store,
	signalStores store.SignalStores,
	webc *web.Client,
) *Maintainer {
	return &Maintainer{
		AccountStore: acctStore,
		SignalStores: signalStores,
		Web:          webc,
		MinAvailable: DefaultMinAvailable,
		TopUpBatch:   DefaultTopUpBatch,
	}
}

func (m *Maintainer) minAvailable() int {
	if m.MinAvailable > 0 {
		return m.MinAvailable
	}
	return DefaultMinAvailable
}

func (m *Maintainer) topUpBatch() int {
	if m.TopUpBatch > 0 {
		return m.TopUpBatch
	}
	return DefaultTopUpBatch
}

// MaybeTopUp uploads prekeys for the ACI namespace when either EC or Kyber
// one-time counts fall below the configured minimum. It persists the
// updated account when an upload occurs.
func (m *Maintainer) MaybeTopUp(ctx context.Context, acct *account.Account) error {
	if m == nil || m.AccountStore == nil || m.SignalStores == nil || m.Web == nil {
		return nil
	}
	if acct == nil {
		return nil
	}
	inv, ok := m.SignalStores.(store.PreKeyInventory)
	if !ok {
		return nil
	}
	ec, kem, err := inv.CountAvailableOneTimePreKeys(acct.ACIIdentity.LastResortKyberPreKey.ID)
	if err != nil {
		return fmt.Errorf("prekeymaint.Maintainer: count: %w", err)
	}
	threshold := m.minAvailable()
	if ec >= threshold && kem >= threshold {
		return nil
	}

	creds := web.Credentials{
		Username: fmt.Sprintf("%s.%d", acct.ACI, acct.DeviceID),
		Password: acct.Password,
	}
	batch := m.topUpBatch()
	upload := UploadIdentity{
		PublicKey:         acct.ACIIdentity.PublicKey,
		PrivateKey:        acct.ACIIdentity.PrivateKey,
		NextPreKeyID:      acct.ACIIdentity.NextPreKeyID,
		NextKyberPreKeyID: acct.ACIIdentity.NextKyberPreKeyID,
	}
	upload, err = UploadOneTimeBatch(ctx, m.Web, creds, web.IdentityACI, upload, batch, m.SignalStores)
	if err != nil {
		return fmt.Errorf("prekeymaint.Maintainer: upload: %w", err)
	}
	acct.ACIIdentity.NextPreKeyID = upload.NextPreKeyID
	acct.ACIIdentity.NextKyberPreKeyID = upload.NextKyberPreKeyID
	if err := m.AccountStore.SaveAccount(acct); err != nil {
		return fmt.Errorf("prekeymaint.Maintainer: save account: %w", err)
	}
	return nil
}
