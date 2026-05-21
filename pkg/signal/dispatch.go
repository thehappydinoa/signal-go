package signal

import (
	"encoding/hex"
	"fmt"
	"time"

	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
)

// dispatchContent routes a decoded Content protobuf to the appropriate
// typed event constructor and emits the result.
func (c *Client) dispatchContent(sender string, senderDevice uint32, envTS, srvTS time.Time, content *sspb.Content) {
	switch {
	case content.GetDataMessage() != nil:
		c.handleDataMessage(sender, senderDevice, envTS, srvTS, content.GetDataMessage())
	case content.GetReceiptMessage() != nil:
		c.handleReceiptMessage(sender, senderDevice, content.GetReceiptMessage())
	case content.GetTypingMessage() != nil:
		c.handleTypingMessage(sender, senderDevice, content.GetTypingMessage())
	case content.GetSyncMessage() != nil:
		c.handleSyncMessage(senderDevice, envTS, content.GetSyncMessage())
	case content.GetDecryptionErrorMessage() != nil:
		c.handleDecryptionErrorMessage(sender, senderDevice, envTS)
	default:
		c.log.Debug("unhandled content type", "sender", sender)
	}
}

func (c *Client) handleDataMessage(sender string, senderDevice uint32, envTS, srvTS time.Time, dm *sspb.DataMessage) {
	ev := &MessageEvent{
		Sender:          sender,
		SenderDevice:    senderDevice,
		Timestamp:       msToTime(dm.GetTimestamp()),
		ServerTimestamp: srvTS,
		Body:            dm.GetBody(),
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = envTS
	}
	if dm.GetExpireTimer() != 0 {
		ev.ExpiresIn = time.Duration(dm.GetExpireTimer()) * time.Second
	}
	if dm.GetGroupV2() != nil && len(dm.GetGroupV2().GetMasterKey()) > 0 {
		ev.GroupID = hex.EncodeToString(dm.GetGroupV2().GetMasterKey())
	}
	c.emit(ev)
}

func (c *Client) handleReceiptMessage(sender string, senderDevice uint32, rm *sspb.ReceiptMessage) {
	var rt ReceiptType
	switch rm.GetType() {
	case sspb.ReceiptMessage_DELIVERY:
		rt = ReceiptDelivery
	case sspb.ReceiptMessage_READ:
		rt = ReceiptRead
	case sspb.ReceiptMessage_VIEWED:
		rt = ReceiptViewed
	default:
		c.log.Debug("unknown receipt type", "type", rm.GetType())
		return
	}

	timestamps := make([]time.Time, 0, len(rm.GetTimestamp()))
	for _, ts := range rm.GetTimestamp() {
		timestamps = append(timestamps, msToTime(ts))
	}

	c.emit(&ReceiptEvent{
		Sender:       sender,
		SenderDevice: senderDevice,
		Type:         rt,
		Timestamps:   timestamps,
	})
}

func (c *Client) handleTypingMessage(sender string, senderDevice uint32, tm *sspb.TypingMessage) {
	var action TypingAction
	switch tm.GetAction() {
	case sspb.TypingMessage_STARTED:
		action = TypingStarted
	case sspb.TypingMessage_STOPPED:
		action = TypingStopped
	default:
		c.log.Debug("unknown typing action", "action", tm.GetAction())
		return
	}

	ev := &TypingEvent{
		Sender:       sender,
		SenderDevice: senderDevice,
		Action:       action,
		Timestamp:    msToTime(tm.GetTimestamp()),
	}
	if tm.GetGroupId() != nil {
		ev.GroupID = hex.EncodeToString(tm.GetGroupId())
	}
	c.emit(ev)
}

func (c *Client) handleSyncMessage(senderDevice uint32, envTS time.Time, sm *sspb.SyncMessage) {
	ev := &SyncMessageEvent{
		SenderDevice: senderDevice,
		Timestamp:    envTS,
	}

	if sent := sm.GetSent(); sent != nil {
		if dm := sent.GetMessage(); dm != nil {
			ev.SentBody = dm.GetBody()
		}
		ev.SentTo = sent.GetDestinationServiceId()
	}

	if reads := sm.GetRead(); len(reads) > 0 {
		for _, r := range reads {
			ev.ReadTimestamps = append(ev.ReadTimestamps, msToTime(r.GetTimestamp()))
		}
	}

	c.emit(ev)
}

func (c *Client) handleDecryptionErrorMessage(sender string, senderDevice uint32, envTS time.Time) {
	c.emit(&DecryptionErrorEvent{
		Sender:       sender,
		SenderDevice: senderDevice,
		Timestamp:    envTS,
		Err:          fmt.Errorf("signal: peer reported decryption error"),
	})
}

// msToTime converts a millisecond Unix timestamp to [time.Time].
func msToTime(ms uint64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms))
}
