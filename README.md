<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-cancancan/brand/main/social/go-ruby-cancancan-cancancan.png" alt="go-ruby-cancancan/cancancan" width="720"></p>

# cancancan — go-ruby-cancancan

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-cancancan.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the rule ENGINE of Ruby's
[CanCanCan](https://github.com/CanCanCommunity/cancancan) authorization gem**
(v3.6, MRI 4.0.5) — the `Ability` rule store and the `can?` / `cannot?` /
`authorize!` matching semantics — **without any Ruby runtime**. It mirrors the
gem's observable behaviour: reverse-precedence rule resolution (last matching
rule wins, `cannot` overrides `can`), `:manage` / `:all` wildcards, action
aliases (`read`→`index,show`, …), hash-attribute and block conditions, and
`CanCan::AccessDenied` on denial.

It is the CanCanCan backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-connection-pool](https://github.com/go-ruby-connection-pool/connection-pool)
and [go-ruby-set](https://github.com/go-ruby-set/set).

> **MRI-faithful engine, not a Rails plug.** This is the authorization *engine* —
> the rule store and matching semantics — not the Rails controller/view
> integration. The Go library owns the rules and the matching; a host (rbgo)
> supplies the Ruby condition-blocks and attribute reads through two function
> seams.

## The two Ruby seams

The engine cannot know how to read a Ruby object's attribute or how to run a Ruby
condition block, so the host injects those as functions on the `Ability`:

```go
// reads subject.send(key) for hash-condition matching
AttrGet func(subject any, key string) any
// runs the ruleID'th rule's Ruby proc against an instance
BlockEval func(ruleID int, subject any) bool
```

`Can` / `Cannot` return the new rule's integer ID, so the host can register the
corresponding Ruby block under it and dispatch through `BlockEval`. A subject
instance may implement the optional `Classified` interface (`CanCanClass()` /
`CanCanAncestors()`) so a rule on a class or superclass matches it — the gem's
`matches_subject_class?`.

## Install

```sh
go get github.com/go-ruby-cancancan/cancancan
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-cancancan/cancancan"
)

type Article struct {
	Published bool
	AuthorID  int
}

func (Article) CanCanClass() cancancan.Class       { return "Article" }
func (Article) CanCanAncestors() []cancancan.Class { return []cancancan.Class{"Record"} }

func main() {
	a := cancancan.New()
	a.AttrGet = func(s any, k string) any {
		art := s.(*Article)
		if k == "published" {
			return art.Published
		}
		return art.AuthorID
	}

	//   can :read,    Article, published: true
	//   can :manage,  Article, author_id: 1      # manage ⇒ every action
	//   cannot :destroy, Article
	a.Can("read", cancancan.Class("Article"), map[string]any{"published": true})
	a.Can(cancancan.Manage, cancancan.Class("Article"), map[string]any{"author_id": 1})
	a.Cannot("destroy", cancancan.Class("Article"))

	pub := &Article{Published: true, AuthorID: 2}
	mine := &Article{Published: false, AuthorID: 1}

	fmt.Println(a.CanQ("read", pub))                       // true  (published)
	fmt.Println(a.CanQ("show", pub))                       // true  (read alias)
	fmt.Println(a.CanQ("update", mine))                    // true  (manage author_id:1)
	fmt.Println(a.CanQ("destroy", mine))                   // false (cannot overrides)
	fmt.Println(a.CanQ("read", cancancan.Class("Article"))) // true  (possible)

	if err := a.AuthorizeBang("destroy", mine); err != nil {
		fmt.Println(err) // *cancancan.AccessDenied
	}
}
```

## API

```go
func New() *Ability                                  // seeded with default aliases

// rule store (return the new rule's ID for block wiring)
func (a *Ability) Can(action, subject any, conditions ...any) int    // can
func (a *Ability) Cannot(action, subject any, conditions ...any) int // cannot
func (a *Ability) AliasAction(to Action, actions ...Action)          // alias_action

// queries
func (a *Ability) CanQ(action Action, subject any) bool     // can?
func (a *Ability) CannotQ(action Action, subject any) bool  // cannot?
func (a *Ability) AuthorizeBang(action Action, subject any) error // authorize!

// wildcards & markers
var  Manage manageT   // :manage — matches every action
var  All    allT      // :all    — matches every subject
type Block  struct{}  // marks a rule as carrying a Ruby condition block
type Class  string    // a subject class token

// seams (fields on Ability)
AttrGet   func(subject any, key string) any
BlockEval func(ruleID int, subject any) bool

// optional subject interface for class matching
type Classified interface {
	CanCanClass() Class
	CanCanAncestors() []Class
}

// modeled errors (all report true from IsCanCanError)
type Error                     struct{ Msg string }                 // CanCan::Error
type AccessDenied              struct{ Msg string; Action Action; Subject any } // CanCan::AccessDenied
type AuthorizationNotPerformed struct{ Msg string }                 // CanCan::AuthorizationNotPerformed
func IsCanCanError(err error) bool
```

**Semantics.** `CanQ` scans rules from last-defined to first-defined and returns
the `base_behavior` of the first whose action, subject, and conditions all
match — so a later rule overrides an earlier one and a `cannot` overrides a `can`.
`:manage` matches every action; `:all` every subject. Action aliases expand a
declared action to the actions it grants (`read` grants `index`/`show`, etc.).
Hash conditions match an attribute by equality, by membership when the value is a
`[]any`, or recursively when the value is a nested `map[string]any` (an
association). A conditional rule **cannot** be evaluated against a class rather
than an instance, so a class-level query is treated as *possible* — mirroring
CanCanCan's "you can act on some such subject".

## Tests & coverage

The suite pairs deterministic, ruby-free tests (which alone hold coverage at
**100%**, so the qemu cross-arch and Windows lanes pass the gate) with a
**differential CanCanCan oracle**: a shared `Ability` is defined here and in the
real gem, and every `can?(action, subject)` over a matrix of actions × subjects
is compared. CanCanCan is not part of Ruby core, so the oracle skips itself where
the gem is not installed (the CI ruby lanes) and validates locally where it is.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

CGO-free, dependency-free, `gofmt` + `go vet` clean, and green across the six
64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le, s390x) and three OSes
(Linux, macOS, Windows).

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-cancancan/cancancan
authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
