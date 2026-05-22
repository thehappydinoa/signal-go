package signal

import (
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

func (c *Client) processSenderKeyDistribution(sender string, senderDevice uint32, skdmBytes []byte) error {
	if c.stores == nil {
		return fmt.Errorf("signal: no stores configured")
	}
	skdm, err := libsignal.DeserializeSenderKeyDistributionMessage(skdmBytes)
	if err != nil {
		return fmt.Errorf("deserialize SKDM: %w", err)
	}
	senderAddr, err := libsignal.NewAddress(sender, senderDevice)
	if err != nil {
		return err
	}
	h := libsignal.NewStoreHandle(c.stores)
	defer h.Release()
	if err := libsignal.ProcessSenderKeyDistributionMessage(senderAddr, skdm, h); err != nil {
		return fmt.Errorf("process SKDM: %w", err)
	}
	return nil
}

// stripContentPadding removes Signal's per-message padding (0x80 terminator
// followed only by zero bytes). If no valid padding suffix is found the
// input is returned unchanged.
func stripContentPadding(padded []byte) []byte {
	idx := -1
	for i := len(padded) - 1; i >= 0; i-- {
		if padded[i] == 0x80 {
			idx = i
			break
		}
		if padded[i] != 0 {
			return padded
		}
	}
	if idx < 0 {
		return padded
	}
	return padded[:idx]
}
