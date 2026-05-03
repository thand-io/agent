//go:build !unix

package localbroker

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newDefaultHelperStarter() helperStarter {
	return func(context.Context, string, []string) (*helperSession, error) {
		return nil, status.Error(codes.Unavailable, "local privilege broker helper is only supported on unix hosts")
	}
}
