// Copyright (c) the go-ruby-cancancan/cancancan authors
//
// SPDX-License-Identifier: BSD-3-Clause

package cancancan

// Error is the base error type, mirroring CanCan::Error (a StandardError
// subclass in the gem). AccessDenied and AuthorizationNotPerformed are its
// modeled subclasses; IsCanCanError reports membership in this family.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

// DefaultAccessDeniedMessage is the message CanCan::AccessDenied uses when the
// caller supplies none, matching the gem's default.
const DefaultAccessDeniedMessage = "You are not authorized to access this page."

// AccessDenied mirrors CanCan::AccessDenied (a CanCan::Error subclass). It is
// returned by AuthorizeBang when the ability denies the (Action, Subject) pair.
// It carries the denied action and subject so a host can render a tailored
// message, exactly as the gem exposes #action and #subject.
type AccessDenied struct {
	Msg     string
	Action  Action
	Subject any
}

func (e *AccessDenied) Error() string { return e.Msg }

// NewAccessDenied builds an AccessDenied for the denied (action, subject). An
// empty message is replaced by DefaultAccessDeniedMessage, mirroring
// CanCan::AccessDenied.new(message, action, subject).
func NewAccessDenied(message string, action Action, subject any) *AccessDenied {
	if message == "" {
		message = DefaultAccessDeniedMessage
	}
	return &AccessDenied{Msg: message, Action: action, Subject: subject}
}

// AuthorizationNotPerformed mirrors CanCan::AuthorizationNotPerformed (a
// CanCan::Error subclass). The gem raises it from check_authorization when a
// controller action finishes without ever calling authorize!; it is provided
// here so a host controller layer can model the same guarantee.
type AuthorizationNotPerformed struct{ Msg string }

func (e *AuthorizationNotPerformed) Error() string { return e.Msg }

// IsCanCanError reports whether err is one of the modeled CanCan error types,
// mirroring the gem's `rescue CanCan::Error` (which catches AccessDenied and
// AuthorizationNotPerformed too, since both subclass CanCan::Error).
func IsCanCanError(err error) bool {
	switch err.(type) {
	case *Error, *AccessDenied, *AuthorizationNotPerformed:
		return true
	default:
		return false
	}
}
