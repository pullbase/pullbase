package rollback

import (
    "context"
    "time"
)

// detachCancel safely returns a context using the provided cancellation function
// but detached from the original request's deadline/timeout. If cancelFactory is nil,
// context.Background is used.
func detachCancel(cancelFactory func() (context.Context, context.CancelFunc)) (context.Context, context.CancelFunc) {
    if cancelFactory != nil {
        return cancelFactory()
    }
    return context.WithCancel(context.Background())
}

// backgroundWithTimeout returns a context that will automatically cancel after the provided timeout.
func backgroundWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
    return context.WithTimeout(context.Background(), timeout)
}
