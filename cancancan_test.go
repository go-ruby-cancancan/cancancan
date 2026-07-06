// Copyright (c) the go-ruby-cancancan/cancancan authors
//
// SPDX-License-Identifier: BSD-3-Clause

package cancancan

import (
	"errors"
	"testing"
)

// article is a fake subject instance. It implements Classified so class rules can
// match it, and its fields are read through a fake AttrGet in the tests below —
// standing in for a Ruby object read via subject.send.
type article struct {
	published bool
	authorID  int
	state     string
}

func (article) CanCanClass() Class       { return "Article" }
func (article) CanCanAncestors() []Class { return []Class{"Record", "Object"} }

// artAttr is a fake AttrGet seam: it reads a named attribute off an *article.
func artAttr(subject any, key string) any {
	a := subject.(*article)
	switch key {
	case "published":
		return a.published
	case "author_id":
		return a.authorID
	case "state":
		return a.state
	default:
		return nil
	}
}

func TestManageAndAllWildcards(t *testing.T) {
	a := New()
	a.Can(Manage, All) // superuser: every action on every subject
	if !a.CanQ("destroy", &article{}) {
		t.Fatal("manage/all should permit destroy on an instance")
	}
	if !a.CanQ("anything", Class("Widget")) {
		t.Fatal("manage/all should permit any action on any class")
	}
}

func TestActionAndSubjectMiss(t *testing.T) {
	a := New()
	a.Can("read", Class("Article"))
	if a.CanQ("write", Class("Article")) { // action miss (continue)
		t.Fatal("read rule must not grant write")
	}
	if a.CanQ("read", Class("Widget")) { // subject miss (continue)
		t.Fatal("Article rule must not grant Widget")
	}
	if a.CanQ("read", Class("Article")) == false {
		t.Fatal("read Article should be granted")
	}
	if !a.CannotQ("read", Class("Widget")) {
		t.Fatal("CannotQ should be the negation of CanQ")
	}
}

func TestDefaultAliases(t *testing.T) {
	a := New()
	a.Can("read", Class("Article"))
	a.Can("create", Class("Article"))
	a.Can("update", Class("Article"))
	for _, act := range []Action{"read", "index", "show"} {
		if !a.CanQ(act, Class("Article")) {
			t.Fatalf("read alias should grant %q", act)
		}
	}
	if !a.CanQ("new", Class("Article")) {
		t.Fatal("create alias should grant new")
	}
	if !a.CanQ("edit", Class("Article")) {
		t.Fatal("update alias should grant edit")
	}
	// Direction: having :index must not grant :read.
	b := New()
	b.Can("index", Class("Article"))
	if b.CanQ("read", Class("Article")) {
		t.Fatal("index must not grant read")
	}
}

func TestActionAsActionType(t *testing.T) {
	a := New()
	a.Can(Action("read"), Class("Article")) // action passed as an Action value
	if !a.CanQ("show", Class("Article")) {
		t.Fatal("Action-typed argument should expand aliases")
	}
}

func TestCustomAliasAction(t *testing.T) {
	a := New()
	a.AliasAction("read", "list") // read now also grants list
	a.Can("read", Class("Article"))
	if !a.CanQ("list", Class("Article")) {
		t.Fatal("custom alias list should be granted via read")
	}
}

func TestAliasCycleAndDuplicateSeen(t *testing.T) {
	// A duplicate declared action and an alias cycle both exercise the
	// already-seen short-circuit in the expansion walk.
	a := New()
	a.AliasAction("read", "read") // self-cycle
	a.Can([]Action{"read", "read"}, Class("Article"))
	if !a.CanQ("show", Class("Article")) {
		t.Fatal("expansion should terminate and still grant show")
	}
}

func TestHashConditionsInstance(t *testing.T) {
	a := New()
	a.AttrGet = artAttr
	a.Can("read", Class("Article"), map[string]any{"published": true})

	if !a.CanQ("read", &article{published: true}) {
		t.Fatal("published article should be readable")
	}
	if a.CanQ("read", &article{published: false}) {
		t.Fatal("unpublished article should not be readable")
	}
	// A class query against a conditional rule is possible → true.
	if !a.CanQ("read", Class("Article")) {
		t.Fatal("class query against a conditional rule should be possible")
	}
}

func TestHashConditionMembershipAndNested(t *testing.T) {
	a := New()
	a.AttrGet = artAttr
	// []any value → membership; nested map → recurse into the attribute.
	a.Can("read", Class("Article"), map[string]any{"state": []any{"draft", "review"}})
	if !a.CanQ("read", &article{state: "review"}) {
		t.Fatal("state in set should match")
	}
	if a.CanQ("read", &article{state: "published"}) {
		t.Fatal("state not in set should not match")
	}

	// Nested association: author.author_id == 1. The nested value's AttrGet
	// receives the associated object (here we reuse the article itself).
	b := New()
	b.AttrGet = func(subject any, key string) any {
		if key == "author" {
			return subject // pretend the association returns an article
		}
		return artAttr(subject, key)
	}
	b.Can("read", Class("Article"), map[string]any{"author": map[string]any{"author_id": 1}})
	if !b.CanQ("read", &article{authorID: 1}) {
		t.Fatal("nested association author_id:1 should match")
	}
	if b.CanQ("read", &article{authorID: 2}) {
		t.Fatal("nested association author_id:2 should not match")
	}
}

func TestBlockConditions(t *testing.T) {
	a := New()
	// BlockEval stands in for a Ruby proc: published articles pass.
	a.BlockEval = func(ruleID int, subject any) bool {
		return subject.(*article).published
	}
	a.Can("moderate", Class("Article"), Block{})

	if !a.CanQ("moderate", &article{published: true}) {
		t.Fatal("block should permit a published article")
	}
	if a.CanQ("moderate", &article{published: false}) {
		t.Fatal("block should deny an unpublished article")
	}
	// Block cannot run on a class → possible → true (BlockEval must not run).
	a.BlockEval = func(ruleID int, subject any) bool {
		t.Fatal("BlockEval must not run for a class subject")
		return false
	}
	if !a.CanQ("moderate", Class("Article")) {
		t.Fatal("class query against a block rule should be possible")
	}
}

func TestReversePrecedenceAndCannotOverride(t *testing.T) {
	a := New()
	a.AttrGet = artAttr
	a.Can("read", Class("Article"))                                        // broad grant
	a.Cannot("read", Class("Article"), map[string]any{"published": false}) // revoke unpublished

	if !a.CanQ("read", &article{published: true}) {
		t.Fatal("published: cannot(false) skipped, can wins → true")
	}
	if a.CanQ("read", &article{published: false}) {
		t.Fatal("unpublished: cannot overrides can → false")
	}

	// A later plain can overrides an earlier plain cannot (reverse order).
	b := New()
	b.Cannot("destroy", Class("Article"))
	b.Can("destroy", Class("Article"))
	if !b.CanQ("destroy", Class("Article")) {
		t.Fatal("later can should override earlier cannot")
	}
}

func TestSpecificInstanceSubject(t *testing.T) {
	a := New()
	pin := &article{state: "pinned"}
	a.Can("read", pin) // rule keyed to a specific instance
	if !a.CanQ("read", pin) {
		t.Fatal("the pinned instance should be readable")
	}
	// Specific-instance rules match by Go value equality (reflect.DeepEqual): an
	// equal-valued instance matches, a differently-valued one does not.
	if !a.CanQ("read", &article{state: "pinned"}) {
		t.Fatal("an equal-valued instance should match")
	}
	if a.CanQ("read", &article{state: "unpinned"}) {
		t.Fatal("a differently-valued instance should not match")
	}
}

func TestSubjectClassAncestorAndClassToken(t *testing.T) {
	a := New()
	a.Can("read", Class("Record")) // a superclass rule
	// Instance whose ancestor chain includes Record → matches.
	if !a.CanQ("read", &article{}) {
		t.Fatal("ancestor Record should match an article instance")
	}
	// The class token itself for a non-matching class.
	if a.CanQ("read", Class("Widget")) {
		t.Fatal("Widget class token should not match a Record rule")
	}

	b := New()
	b.Can("read", Class("Article"))
	// Non-Classified, non-Class instance → no class match.
	if b.CanQ("read", 42) {
		t.Fatal("a plain int should not match a class rule")
	}
}

func TestSliceActionsAndSubjects(t *testing.T) {
	a := New()
	a.Can([]any{"read", Manage}, []any{Class("Article"), All})
	if !a.CanQ("read", Class("Article")) {
		t.Fatal("slice action should grant read")
	}
	if !a.CanQ("whatever", Class("Widget")) {
		t.Fatal("Manage-in-slice + All-in-slice should grant anything on anything")
	}
}

func TestAuthorizeBang(t *testing.T) {
	a := New()
	a.Can("read", Class("Article"))
	if err := a.AuthorizeBang("read", Class("Article")); err != nil {
		t.Fatalf("authorize! read should pass: %v", err)
	}
	err := a.AuthorizeBang("destroy", Class("Article"))
	if err == nil {
		t.Fatal("authorize! destroy should deny")
	}
	var ad *AccessDenied
	if !errors.As(err, &ad) {
		t.Fatalf("want *AccessDenied, got %T", err)
	}
	if ad.Action != "destroy" || ad.Subject.(Class) != "Article" {
		t.Fatalf("AccessDenied should carry action/subject, got %+v", ad)
	}
	if ad.Error() != DefaultAccessDeniedMessage {
		t.Fatalf("default message mismatch: %q", ad.Error())
	}
}

func TestPanicsOnUnsupportedTypes(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s: expected panic", name)
			}
		}()
		fn()
	}
	a := New()
	mustPanic("action", func() { a.Can(123, Class("Article")) })
	mustPanic("condition", func() { a.Can("read", Class("Article"), "bad-cond") })
}
