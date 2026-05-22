package bot

import "errors"

// ErrReplyNotSupportedInGroup is returned when a bot helper cannot act
// in a group thread. Group text reply, reactions, and typing are
// supported via Phase 5; attachment replies remain deferred.
var ErrReplyNotSupportedInGroup = errors.New("bot: operation not supported in group threads")
