// Copyright (c) the go-ruby-cancancan/cancancan authors
//
// SPDX-License-Identifier: BSD-3-Clause

package cancancan

import (
	"os/exec"
	"strings"
	"testing"
)

// This differential oracle checks the Go engine's can?/cannot? decisions against
// the real CanCanCan gem running under MRI. CanCanCan is not part of Ruby core,
// so the oracle skips itself unless `ruby -e "require 'cancan'"` succeeds (the CI
// ruby lanes install MRI but not the gem, so they skip; the deterministic suite
// alone holds coverage at 100%). It is a fidelity check, not a coverage driver.

// rubyWithCanCan locates a ruby that can load the cancancan gem, or skips.
func rubyWithCanCan(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping CanCanCan oracle")
	}
	if err := exec.Command(path, "-e", "require 'cancan'").Run(); err != nil {
		t.Skip("cancancan gem not installed; skipping CanCanCan oracle")
	}
	return path
}

// rubyDecision runs the shared Ruby Ability and prints the boolean result of
// can?(action, subjectExpr) so the Go side can compare.
const rubyOracleScript = `
$stdout.binmode
require 'cancan'
class Article
  attr_accessor :published, :author_id
  def initialize(published:, author_id:)
    @published = published
    @author_id = author_id
  end
end
class Ability
  include CanCan::Ability
  def initialize
    alias_action :list, to: :read
    can :read, Article, published: true
    can :manage, Article, author_id: 1
    cannot :destroy, Article
    can :update, :all
    can :moderate, Article do |a|
      a.published == true
    end
  end
end
A = Ability.new
PUB   = Article.new(published: true,  author_id: 2)
UNPUB = Article.new(published: false, author_id: 2)
MINE  = Article.new(published: false, author_id: 1)
SUBJ = { "PUB" => PUB, "UNPUB" => UNPUB, "MINE" => MINE, "CLASS" => Article }
action, key = ARGV
print A.can?(action.to_sym, SUBJ.fetch(key)) ? "true" : "false"
`

func rubyCan(t *testing.T, bin, action, key string) bool {
	t.Helper()
	out, err := exec.Command(bin, "-e", rubyOracleScript, "--", action, key).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby oracle error: %v\noutput:\n%s", err, out)
	}
	return strings.TrimSpace(string(out)) == "true"
}

// goAbility mirrors the Ruby Ability above, using the Go engine + fake seams.
func goAbility() (*Ability, map[string]any) {
	a := New()
	a.AliasAction("read", "list")
	a.Can("read", Class("Article"), map[string]any{"published": true})
	a.Can(Manage, Class("Article"), map[string]any{"author_id": 1})
	a.Cannot("destroy", Class("Article"))
	a.Can("update", All)
	modID := a.Can("moderate", Class("Article"), Block{})

	// article (from cancancan_test.go) implements Classified as class "Article".
	pub := &article{published: true, authorID: 2}
	unpub := &article{published: false, authorID: 2}
	mine := &article{published: false, authorID: 1}
	a.AttrGet = artAttr
	a.BlockEval = func(ruleID int, subject any) bool {
		if ruleID != modID {
			return false
		}
		return subject.(*article).published
	}
	subj := map[string]any{
		"PUB": pub, "UNPUB": unpub, "MINE": mine, "CLASS": Class("Article"),
	}
	return a, subj
}

func TestOracleAgainstCanCanCanGem(t *testing.T) {
	bin := rubyWithCanCan(t)
	a, subj := goAbility()

	actions := []string{"read", "index", "show", "list", "create", "update", "edit", "destroy", "moderate"}
	keys := []string{"PUB", "UNPUB", "MINE", "CLASS"}
	for _, act := range actions {
		for _, key := range keys {
			want := rubyCan(t, bin, act, key)
			got := a.CanQ(Action(act), subj[key])
			if got != want {
				t.Errorf("can?(%s, %s): go=%v ruby=%v", act, key, got, want)
			}
		}
	}
}
