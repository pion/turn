// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"testing"

	"github.com/pion/stun/v4"
	"github.com/stretchr/testify/assert"
)

func TestTransactionCheckIntegrity(t *testing.T) {
	integrity := stun.NewLongTermIntegrity("user", "realm", "pass")
	wrongIntegrity := stun.NewLongTermIntegrity("user", "realm", "wrong")
	tests := []struct {
		name        string
		trIntegrity stun.MessageIntegrity
		msgAttrs    []stun.Setter
		want        bool
	}{
		{
			name:        "UnsignedBadRequestAllowed",
			trIntegrity: integrity,
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				stun.CodeBadRequest,
			},
			want: true,
		},
		{
			name:        "UnsignedUnauthorizedAllowed",
			trIntegrity: integrity,
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				stun.CodeUnauthorized,
			},
			want: true,
		},
		{
			name:        "UnsignedAllocMismatchAllowed",
			trIntegrity: integrity,
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				stun.CodeAllocMismatch,
			},
			want: true,
		},
		{
			name:        "UnsignedStaleNonceAllowed",
			trIntegrity: integrity,
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				stun.CodeStaleNonce,
			},
			want: true,
		},
		{
			name:        "UnsignedNonExemptErrorRejected",
			trIntegrity: integrity,
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				stun.CodeServerError,
			},
			want: false,
		},
		{
			name:        "UnsignedSuccessRejected",
			trIntegrity: integrity,
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassSuccessResponse),
			},
			want: false,
		},
		{
			name:        "SignedErrorValidIntegrityAllowed",
			trIntegrity: integrity,
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				stun.CodeAllocMismatch,
				integrity,
			},
			want: true,
		},
		{
			name:        "SignedExemptErrorInvalidIntegrityRejected",
			trIntegrity: integrity,
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				stun.CodeAllocMismatch,
				wrongIntegrity,
			},
			want: false,
		},
		{
			name: "UnauthenticatedTransactionAllowsUnsigned",
			msgAttrs: []stun.Setter{
				stun.NewType(stun.MethodRefresh, stun.ClassSuccessResponse),
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTransaction(&TransactionConfig{Integrity: tc.trIntegrity})
			attrs := append([]stun.Setter{stun.TransactionID}, tc.msgAttrs...)
			msg := stun.MustBuild(attrs...)
			assert.Equal(t, tc.want, tr.CheckIntegrity(msg))
		})
	}
}
