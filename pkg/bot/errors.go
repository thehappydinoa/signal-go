package bot

import "errors"

// ErrReplyNotSupportedInGroup is retained for compatibility; group replies
// including attachments are supported via [Message.Reply] and
// [Message.ReplyAttachment].
var ErrReplyNotSupportedInGroup = errors.New("bot: operation not supported in group threads")
