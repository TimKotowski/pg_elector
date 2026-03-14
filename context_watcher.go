package pg_elector

import "context"

type Handler func()

type ContextWatcher struct {
	handleCancel Handler
	ctx          context.Context
	release      chan struct{}
}

func NewContextWatcher(handle Handler, ctx context.Context) *ContextWatcher {
	return &ContextWatcher{
		handleCancel: handle,
		ctx:          ctx,
		release:      make(chan struct{}, 1),
	}
}

func (c *ContextWatcher) Watch() {
	go c.watch()
}

func (c *ContextWatcher) Release() <-chan struct{} {
	return c.release
}

func (c *ContextWatcher) watch() {
	select {
	case <-c.ctx.Done():
		c.handleCancel()
		c.release <- struct{}{}
		return
	}
}
