package svc

import "context"

// ServerFunc starts the server and blocks until ctx is canceled or a fatal
// error occurs. It is implemented by internal/server.Server.Run.
type ServerFunc func(ctx context.Context) error
