// Copyright (c) the go-ruby-cancancan/cancancan authors
//
// SPDX-License-Identifier: BSD-3-Clause

package cancancan

import "testing"

func TestModeledErrors(t *testing.T) {
	base := &Error{Msg: "boom"}
	if base.Error() != "boom" {
		t.Fatalf("Error.Error(): got %q", base.Error())
	}

	ad := NewAccessDenied("nope", "destroy", Class("Article"))
	if ad.Error() != "nope" || ad.Action != "destroy" {
		t.Fatalf("AccessDenied mismatch: %+v", ad)
	}
	if def := NewAccessDenied("", "read", nil); def.Error() != DefaultAccessDeniedMessage {
		t.Fatalf("empty message should default, got %q", def.Error())
	}

	anp := &AuthorizationNotPerformed{Msg: "forgot authorize!"}
	if anp.Error() != "forgot authorize!" {
		t.Fatalf("AuthorizationNotPerformed.Error(): got %q", anp.Error())
	}

	for _, e := range []error{base, ad, anp} {
		if !IsCanCanError(e) {
			t.Fatalf("IsCanCanError should be true for %T", e)
		}
	}
	if IsCanCanError(errString("other")) {
		t.Fatal("IsCanCanError should be false for a foreign error")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
