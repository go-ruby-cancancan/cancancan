// Copyright (c) the go-ruby-cancancan/cancancan authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package cancancan is a pure-Go (no cgo) reimplementation of the rule ENGINE of
// Ruby's CanCanCan authorization gem (v3.6, MRI 4.0.5) — the Ability rule store
// and the can?/cannot?/authorize! matching semantics — without any Ruby runtime.
// It is the CanCanCan backend for go-embedded-ruby but is a standalone, reusable
// module with no dependency on the Ruby runtime.
//
// # What it models
//
// An Ability collects can/cannot rules over (action, subject) pairs. A query
// (CanQ) resolves by scanning the rules in REVERSE definition order and taking
// the first whose action, subject, and conditions all match — so a later rule
// overrides an earlier one and a cannot overrides a can, exactly as CanCanCan's
// Ability#relevant_rules + detect do.
//
//   - Actions are symbols. Manage is the wildcard (:manage) matching every
//     action. Action aliases expand a declared action to the actions it grants:
//     the CanCanCan defaults are read→{index,show}, create→{new}, update→{edit},
//     and AliasAction adds more (the gem's alias_action).
//   - Subjects are class tokens (Class), the wildcard All (:all), or specific
//     instances. A Class rule matches that class and its instances; an instance
//     rule matches by value equality.
//   - Conditions are a hash (attribute matcher) or a block/lambda. Neither can be
//     evaluated when the subject is a class rather than an instance, so — as in
//     CanCanCan — a class query against a conditional rule is treated as possible
//     (it means "you can act on SOME such subject").
//
// # The Ruby seams
//
// The engine owns the rule store and the matching semantics; the two things it
// cannot know — how to read a Ruby object's attribute and how to run a Ruby
// condition block — are injected as function seams on the Ability:
//
//   - AttrGet(subject, key) reads an attribute for hash-condition matching. It
//     stands in for `subject.send(key)`. The go-embedded-ruby binding dispatches
//     the Ruby reader here.
//   - BlockEval(ruleID, subject) evaluates a rule's Ruby condition block against
//     an instance. Can/Cannot return the rule's ID so the host can register the
//     corresponding Ruby proc under it. It stands in for `block.call(subject)`.
package cancancan

import "reflect"

// Action is an action name — a Ruby symbol such as :read or :update.
type Action string

// manageT is the type of Manage, the :manage wildcard action.
type manageT struct{}

// Manage is the wildcard action (:manage): a rule declared with it matches every
// action, mirroring CanCanCan's :manage.
var Manage = manageT{}

// Class is a subject class token — a Ruby class identified by name. A rule
// declared with a Class matches that class itself and any of its instances (see
// Classified).
type Class string

// allT is the type of All, the :all wildcard subject.
type allT struct{}

// All is the wildcard subject (:all): a rule declared with it matches every
// subject, mirroring CanCanCan's :all.
var All = allT{}

// Block marks a rule as carrying a Ruby condition block/lambda. Pass it as a
// condition to Can/Cannot; at match time against an instance the engine calls
// Ability.BlockEval(ruleID, subject). It stands in for the gem's `&block`.
type Block struct{}

// Classified is the optional interface a subject instance may implement so the
// engine can match it against a Class rule. The go-embedded-ruby binding wraps a
// Ruby object to report its class and ancestor chain here, letting a rule on a
// superclass match an instance of a subclass — the gem's matches_subject_class?.
type Classified interface {
	CanCanClass() Class
	CanCanAncestors() []Class
}

// rule is a single can/cannot entry in an Ability's store.
type rule struct {
	id       int
	base     bool // true for can, false for cannot (the gem's base_behavior)
	actions  []Action
	manage   bool // declared with Manage
	subjects []any
	all      bool           // declared with All
	conds    map[string]any // hash conditions, nil if none
	block    bool           // has a Ruby condition block
}

// Ability is a store of can/cannot rules plus the matching engine, mirroring a
// class that `include CanCan::Ability`. Construct it with New; set AttrGet and
// BlockEval before evaluating hash- or block-conditioned rules.
type Ability struct {
	rules   []rule
	aliases map[Action][]Action
	nextID  int

	// AttrGet reads subject's attribute key for hash-condition matching. It
	// stands in for `subject.send(key)`; required only when hash-conditioned
	// rules are evaluated against instances.
	AttrGet func(subject any, key string) any
	// BlockEval evaluates the rule ruleID's Ruby condition block against
	// subject. It stands in for `block.call(subject)`; required only when
	// block-conditioned rules are evaluated against instances.
	BlockEval func(ruleID int, subject any) bool
}

// New returns an empty Ability seeded with CanCanCan's default action aliases
// (read→index,show; create→new; update→edit).
func New() *Ability {
	return &Ability{
		aliases: map[Action][]Action{
			"read":   {"index", "show"},
			"create": {"new"},
			"update": {"edit"},
		},
	}
}

// AliasAction declares that granting `to` also grants `actions`, mirroring the
// gem's `alias_action *actions, to: to`. It appends to any existing aliases for
// `to` (the defaults included).
func (a *Ability) AliasAction(to Action, actions ...Action) {
	a.aliases[to] = append(a.aliases[to], actions...)
}

// Can adds a permission rule. action is an Action/string, Manage, or a slice of
// these; subject is a Class, All, an instance, or a slice of these; conditions
// are an optional hash (map[string]any) and/or Block. It returns the new rule's
// ID (for wiring a Ruby block via BlockEval). It mirrors `can action, subject,
// conditions, &block`.
func (a *Ability) Can(action, subject any, conditions ...any) int {
	return a.addRule(true, action, subject, conditions...)
}

// Cannot adds a revocation rule with the same argument shape as Can, mirroring
// `cannot action, subject, conditions, &block`. A matching Cannot overrides an
// earlier matching Can.
func (a *Ability) Cannot(action, subject any, conditions ...any) int {
	return a.addRule(false, action, subject, conditions...)
}

func (a *Ability) addRule(base bool, action, subject any, conditions ...any) int {
	r := rule{id: a.nextID, base: base}
	a.nextID++
	r.actions, r.manage = toActions(action)
	r.subjects, r.all = toSubjects(subject)
	for _, c := range conditions {
		switch cc := c.(type) {
		case map[string]any:
			r.conds = cc
		case Block:
			r.block = true
		default:
			panic("cancancan: unsupported condition type")
		}
	}
	a.rules = append(a.rules, r)
	return r.id
}

// toActions normalizes an action argument into concrete actions plus a manage
// flag. It accepts an Action, a string, Manage, or a slice ([]Action or []any)
// of these.
func toActions(action any) (acts []Action, manage bool) {
	switch v := action.(type) {
	case manageT:
		manage = true
	case Action:
		acts = []Action{v}
	case string:
		acts = []Action{Action(v)}
	case []Action:
		acts = append(acts, v...)
	case []any:
		for _, e := range v {
			sub, m := toActions(e)
			acts = append(acts, sub...)
			manage = manage || m
		}
	default:
		panic("cancancan: unsupported action type")
	}
	return acts, manage
}

// toSubjects normalizes a subject argument into concrete subjects plus an all
// flag. It accepts All, a []any slice of subjects, or any single subject (a
// Class or an instance).
func toSubjects(subject any) (subs []any, all bool) {
	switch v := subject.(type) {
	case allT:
		all = true
	case []any:
		for _, e := range v {
			sub, al := toSubjects(e)
			subs = append(subs, sub...)
			all = all || al
		}
	default:
		subs = []any{v}
	}
	return subs, all
}

// CanQ reports whether the ability permits action on subject, mirroring
// `can?(action, subject)`. It scans rules from last-defined to first-defined and
// returns the base_behavior of the first whose action, subject, and conditions
// all match; if none matches it returns false.
func (a *Ability) CanQ(action Action, subject any) bool {
	_, isClass := subject.(Class)
	for i := len(a.rules) - 1; i >= 0; i-- {
		r := &a.rules[i]
		if !a.matchesAction(r, action) {
			continue
		}
		if !matchesSubject(r, subject) {
			continue
		}
		if a.matchesConditions(r, isClass, subject) {
			return r.base
		}
	}
	return false
}

// CannotQ is the negation of CanQ, mirroring `cannot?(action, subject)`.
func (a *Ability) CannotQ(action Action, subject any) bool {
	return !a.CanQ(action, subject)
}

// AuthorizeBang returns nil when the ability permits action on subject, or an
// *AccessDenied otherwise, mirroring `authorize!(action, subject)` (which raises
// CanCan::AccessDenied on denial). The returned error carries the action and
// subject.
func (a *Ability) AuthorizeBang(action Action, subject any) error {
	if a.CanQ(action, subject) {
		return nil
	}
	return NewAccessDenied("", action, subject)
}

// matchesAction reports whether rule r covers action, accounting for :manage and
// alias expansion (a rule declared with an aliasing action covers everything the
// alias grants).
func (a *Ability) matchesAction(r *rule, action Action) bool {
	if r.manage {
		return true
	}
	seen := map[Action]bool{}
	stack := append([]Action{}, r.actions...)
	for len(stack) > 0 {
		act := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[act] {
			continue
		}
		seen[act] = true
		stack = append(stack, a.aliases[act]...)
	}
	return seen[action]
}

// matchesSubject reports whether rule r covers subject: :all matches everything,
// a Class subject matches the class or its instances, and any other declared
// subject matches by value equality (the gem's @subjects.include?).
func matchesSubject(r *rule, subject any) bool {
	if r.all {
		return true
	}
	for _, s := range r.subjects {
		if c, ok := s.(Class); ok {
			if matchesSubjectClass(c, subject) {
				return true
			}
			continue
		}
		if reflect.DeepEqual(s, subject) {
			return true
		}
	}
	return false
}

// matchesSubjectClass reports whether subject belongs to ruleClass — either it is
// that class token, or it is an instance (Classified) whose class or an ancestor
// is ruleClass — mirroring the gem's matches_subject_class?.
func matchesSubjectClass(ruleClass Class, subject any) bool {
	if c, ok := subject.(Class); ok {
		return c == ruleClass
	}
	if inst, ok := subject.(Classified); ok {
		if inst.CanCanClass() == ruleClass {
			return true
		}
		for _, anc := range inst.CanCanAncestors() {
			if anc == ruleClass {
				return true
			}
		}
	}
	return false
}

// matchesConditions reports whether rule r's conditions hold for subject. An
// unconditional rule always holds. A conditional rule cannot be evaluated against
// a class (isClass), so it is treated as possible/relevant — the gem's behaviour
// for class-level checks. Against an instance, a block rule defers to BlockEval
// and a hash rule to matchesConditionsHash.
func (a *Ability) matchesConditions(r *rule, isClass bool, subject any) bool {
	if r.conds == nil && !r.block {
		return true
	}
	if isClass {
		return true
	}
	if r.block {
		return a.BlockEval(r.id, subject)
	}
	return a.matchesConditionsHash(r.conds, subject)
}

// matchesConditionsHash reports whether every attribute in conds matches
// subject: a nested map recurses into the associated attribute, a []any value
// matches by membership, and any other value matches by equality — mirroring the
// gem's matches_conditions_hash?.
func (a *Ability) matchesConditionsHash(conds map[string]any, subject any) bool {
	for k, v := range conds {
		attr := a.AttrGet(subject, k)
		switch want := v.(type) {
		case map[string]any:
			if !a.matchesConditionsHash(want, attr) {
				return false
			}
		case []any:
			found := false
			for _, e := range want {
				if reflect.DeepEqual(attr, e) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		default:
			if !reflect.DeepEqual(attr, v) {
				return false
			}
		}
	}
	return true
}
