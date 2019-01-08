package golinters

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"

	"honnef.co/go/tools/config"
	"honnef.co/go/tools/stylecheck"

	"golang.org/x/tools/go/packages"
	"honnef.co/go/tools/lint"
	"honnef.co/go/tools/lint/lintutil"
	"honnef.co/go/tools/simple"
	"honnef.co/go/tools/staticcheck"
	"honnef.co/go/tools/unused"

	"github.com/golangci/golangci-lint/pkg/lint/linter"
	"github.com/golangci/golangci-lint/pkg/result"
)

const megacheckName = "megacheck"

type Megacheck struct {
	UnusedEnabled      bool
	GosimpleEnabled    bool
	StaticcheckEnabled bool
	StylecheckEnabled  bool
}

func (m Megacheck) Name() string {
	names := []string{}
	if m.UnusedEnabled {
		names = append(names, "unused")
	}
	if m.GosimpleEnabled {
		names = append(names, "gosimple")
	}
	if m.StaticcheckEnabled {
		names = append(names, "staticcheck")
	}
	if m.StylecheckEnabled {
		names = append(names, "stylecheck")
	}

	if len(names) == 1 {
		return names[0] // only one sublinter is enabled
	}

	if len(names) == 4 {
		return megacheckName // all enabled
	}

	return fmt.Sprintf("megacheck.{%s}", strings.Join(names, ","))
}

func (m Megacheck) Desc() string {
	descs := map[string]string{
		"unused":      "Checks Go code for unused constants, variables, functions and types",
		"gosimple":    "Linter for Go source code that specializes in simplifying a code",
		"staticcheck": "Staticcheck is a go vet on steroids, applying a ton of static analysis checks",
		"stylecheck":  "Stylecheck is a replacement for golint",
		"megacheck":   "3 sub-linters in one: unused, gosimple and staticcheck",
	}

	return descs[m.Name()]
}

func (m Megacheck) Run(ctx context.Context, lintCtx *linter.Context) ([]result.Issue, error) {
	issues, err := runMegacheck(lintCtx.Packages,
		m.StaticcheckEnabled, m.GosimpleEnabled, m.UnusedEnabled, m.StylecheckEnabled,
		lintCtx.Settings().Unused.CheckExported)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run megacheck")
	}

	if len(issues) == 0 {
		return nil, nil
	}

	res := make([]result.Issue, 0, len(issues))
	for _, i := range issues {
		res = append(res, result.Issue{
			Pos:        i.Position,
			Text:       markIdentifiers(i.Text),
			FromLinter: m.Name(),
		})
	}
	return res, nil
}

func runMegacheck(workingPkgs []*packages.Package,
	enableStaticcheck, enableGosimple, enableUnused, enableStylecheck, checkExportedUnused bool) ([]lint.Problem, error) {

	var checkers []lint.Checker

	if enableGosimple {
		checkers = append(checkers, simple.NewChecker())
	}
	if enableStaticcheck {
		checkers = append(checkers, staticcheck.NewChecker())
	}
	if enableStylecheck {
		checkers = append(checkers, stylecheck.NewChecker())
	}
	if enableUnused {
		uc := unused.NewChecker(unused.CheckAll)
		uc.ConsiderReflection = true
		uc.WholeProgram = checkExportedUnused
		checkers = append(checkers, unused.NewLintChecker(uc))
	}

	if len(checkers) == 0 {
		return nil, nil
	}

	cfg := config.Config{}
	opts := &lintutil.Options{
		// TODO: get current go version, but now it doesn't matter,
		// may be needed after next updates of megacheck
		GoVersion: 11,

		Config: cfg,
		// TODO: support Ignores option
	}

	return runMegacheckCheckers(checkers, opts, workingPkgs)
}

// parseIgnore is a copy from megacheck code just to not fork megacheck
func parseIgnore(s string) ([]lint.Ignore, error) {
	var out []lint.Ignore
	if len(s) == 0 {
		return nil, nil
	}
	for _, part := range strings.Fields(s) {
		p := strings.Split(part, ":")
		if len(p) != 2 {
			return nil, errors.New("malformed ignore string")
		}
		path := p[0]
		checks := strings.Split(p[1], ",")
		out = append(out, &lint.GlobIgnore{Pattern: path, Checks: checks})
	}
	return out, nil
}

func runMegacheckCheckers(cs []lint.Checker, opt *lintutil.Options, workingPkgs []*packages.Package) ([]lint.Problem, error) {
	stats := lint.PerfStats{
		CheckerInits: map[string]time.Duration{},
	}

	ignores, err := parseIgnore(opt.Ignores)
	if err != nil {
		return nil, err
	}

	var problems []lint.Problem
	if len(workingPkgs) == 0 {
		return problems, nil
	}

	l := &lint.Linter{
		Checkers:      cs,
		Ignores:       ignores,
		GoVersion:     opt.GoVersion,
		ReturnIgnored: opt.ReturnIgnored,
		Config:        opt.Config,

		MaxConcurrentJobs: opt.MaxConcurrentJobs,
		PrintStats:        opt.PrintStats,
	}
	problems = append(problems, l.Lint(workingPkgs, &stats)...)

	return problems, nil
}
