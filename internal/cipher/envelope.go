package cipher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/prekeymaint"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
	"github.com/thehappydinoa/signal-go/internal/store"
)

// LocalIdentitySetter is implemented by stores that accept a one-time
// injection of the linked device's identity keys (e.g. [memstore.SignalStores]).
type LocalIdentitySetter interface {
	SetLocalIdentity(pub, priv []byte, regID uint32)
}

// EnvelopeDecryptor decrypts inbound [sspb.Envelope] payloads via libsignal.
type EnvelopeDecryptor struct {
	stores         store.SignalStores
	localServiceID string
	localDeviceID  uint32
	localE164      string
	trustRoots     []*libsignal.PublicKey
	acct           *account.Account
	preKeyMaint    *prekeymaint.Maintainer
}

// NewEnvelopeDecryptor builds a decryptor for the linked ACI namespace.
func NewEnvelopeDecryptor(acct *account.Account, stores store.SignalStores) (*EnvelopeDecryptor, error) {
	if acct == nil {
		return nil, errors.New("cipher.NewEnvelopeDecryptor: nil account")
	}
	if stores == nil {
		return nil, errors.New("cipher.NewEnvelopeDecryptor: nil stores")
	}
	bootstrapStores(acct, stores)
	roots, err := libsignal.ProductionTrustRoots()
	if err != nil {
		return nil, fmt.Errorf("cipher.NewEnvelopeDecryptor: %w", err)
	}
	return &EnvelopeDecryptor{
		stores:         stores,
		localServiceID: acct.ACI,
		localDeviceID:  acct.DeviceID,
		localE164:      acct.Number,
		trustRoots:     roots,
		acct:           acct,
	}, nil
}

// SetPreKeyMaintainer wires automatic one-time prekey top-up after a
// successful inbound prekey decrypt. acct must be the same pointer passed
// to [NewEnvelopeDecryptor].
func (d *EnvelopeDecryptor) SetPreKeyMaintainer(m *prekeymaint.Maintainer) {
	d.preKeyMaint = m
}

func bootstrapStores(acct *account.Account, s store.SignalStores) {
	setter, ok := s.(LocalIdentitySetter)
	if !ok {
		return
	}
	if _, _, err := s.LocalIdentityKey(); err == nil {
		return
	}
	setter.SetLocalIdentity(
		acct.ACIIdentity.PublicKey,
		acct.ACIIdentity.PrivateKey,
		acct.ACIIdentity.RegistrationID,
	)
}

// Decrypt implements [signal.Decryptor].
func (d *EnvelopeDecryptor) Decrypt(ctx context.Context, env *sspb.Envelope) ([]byte, string, uint32, error) {
	if env == nil {
		return nil, "", 0, errors.New("cipher: nil envelope")
	}
	content := env.GetContent()
	if len(content) == 0 {
		return nil, "", 0, errors.New("cipher: empty envelope content")
	}

	switch env.GetType() {
	case sspb.Envelope_PLAINTEXT_CONTENT:
		return d.decryptPlaintext(content, env)

	case sspb.Envelope_UNIDENTIFIED_SENDER:
		return d.decryptSealed(ctx, content)

	case sspb.Envelope_DOUBLE_RATCHET:
		payload, _ := libsignal.StripVersionByte(content)
		return d.decryptWhisper(payload, env)

	case sspb.Envelope_PREKEY_MESSAGE:
		payload, _ := libsignal.StripVersionByte(content)
		pt, sender, device, err := d.decryptPreKey(payload, env)
		if err == nil {
			d.maintainPreKeys(ctx)
		}
		return pt, sender, device, err

	case sspb.Envelope_SERVER_DELIVERY_RECEIPT:
		return nil, env.GetSourceServiceId(), env.GetSourceDeviceId(), errors.New("cipher: server delivery receipt has no content")

	default:
		return nil, "", 0, fmt.Errorf("cipher: unsupported envelope type %s", env.GetType().String())
	}
}

func (d *EnvelopeDecryptor) decryptPlaintext(content []byte, env *sspb.Envelope) ([]byte, string, uint32, error) {
	if len(content) < 2 {
		return nil, "", 0, errors.New("cipher: plaintext content too short")
	}
	// Marker byte 0x00, then Content protobuf.
	return content[1:], env.GetSourceServiceId(), env.GetSourceDeviceId(), nil
}

func (d *EnvelopeDecryptor) decryptSealed(ctx context.Context, content []byte) ([]byte, string, uint32, error) {
	payload := content
	if single, err := libsignal.MultiRecipientMessageForSingleRecipient(content); err == nil {
		payload = single
	}
	res, err := libsignal.DecryptSealedSender(payload, libsignal.DecryptParams{
		Stores:         d.stores,
		LocalServiceID: d.localServiceID,
		LocalDeviceID:  d.localDeviceID,
		LocalE164:      d.localE164,
		TrustRoots:     d.trustRoots,
		ValidationTime: time.Now(),
	})
	if err != nil {
		return nil, "", 0, err
	}
	if res.ConsumedOneTimePreKey {
		d.maintainPreKeys(ctx)
	}
	return res.Plaintext, res.SenderUUID, res.SenderDevice, nil
}

func (d *EnvelopeDecryptor) maintainPreKeys(ctx context.Context) {
	if d.preKeyMaint == nil || d.acct == nil {
		return
	}
	if err := d.preKeyMaint.MaybeTopUp(ctx, d.acct); err != nil {
		// Best-effort: receive loop must not fail on upload errors.
		_ = err
	}
}

func (d *EnvelopeDecryptor) decryptWhisper(payload []byte, env *sspb.Envelope) ([]byte, string, uint32, error) {
	h := libsignal.NewStoreHandle(d.stores)
	defer h.Release()

	msg, err := libsignal.DeserializeSignalMessage(payload)
	if err != nil {
		return nil, "", 0, err
	}
	sender, err := libsignal.NewAddress(env.GetSourceServiceId(), env.GetSourceDeviceId())
	if err != nil {
		return nil, "", 0, err
	}
	local, err := libsignal.NewAddress(d.localServiceID, d.localDeviceID)
	if err != nil {
		return nil, "", 0, err
	}
	ptext, err := libsignal.DecryptSignalMessage(msg, sender, local, h)
	if err != nil {
		return nil, "", 0, err
	}
	return ptext, env.GetSourceServiceId(), env.GetSourceDeviceId(), nil
}

func (d *EnvelopeDecryptor) decryptPreKey(payload []byte, env *sspb.Envelope) ([]byte, string, uint32, error) {
	h := libsignal.NewStoreHandle(d.stores)
	defer h.Release()

	msg, err := libsignal.DeserializePreKeySignalMessage(payload)
	if err != nil {
		return nil, "", 0, err
	}
	sender, err := libsignal.NewAddress(env.GetSourceServiceId(), env.GetSourceDeviceId())
	if err != nil {
		return nil, "", 0, err
	}
	local, err := libsignal.NewAddress(d.localServiceID, d.localDeviceID)
	if err != nil {
		return nil, "", 0, err
	}
	ptext, err := libsignal.DecryptPreKeySignalMessage(msg, sender, local, h)
	if err != nil {
		return nil, "", 0, err
	}
	return ptext, env.GetSourceServiceId(), env.GetSourceDeviceId(), nil
}
