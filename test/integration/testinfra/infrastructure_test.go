package testinfra

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsNamespaceNotVisibleError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "temporal namespace not found",
			err:  serviceerror.NewNamespaceNotFound(TemporalTestNamespace),
			want: true,
		},
		{
			name: "grpc not found",
			err:  status.Error(codes.NotFound, "namespace unavailable"),
			want: true,
		},
		{
			name: "generic error text is ignored",
			err:  errors.New("namespace thand-test not found"),
			want: false,
		},
		{
			name: "grpc unavailable is not retried",
			err:  status.Error(codes.Unavailable, "transport unavailable"),
			want: false,
		},
		{
			name: "grpc deadline exceeded is not retried",
			err:  status.Error(codes.DeadlineExceeded, "deadline exceeded"),
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isNamespaceNotVisibleError(tc.err))
		})
	}
}
