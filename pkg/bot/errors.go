package bot

import "errors"

// ErrReplyNotSupportedInGroup is returned by [Message.Reply] (and the
// other group-targeted helpers) for group messages. Group send lands
// with Phase 5.
var ErrReplyNotSupportedInGroup = errors.New("bot: replying in group threads requires Phase 5 (groups v2)")
