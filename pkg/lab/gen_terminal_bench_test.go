package lab

import (
	"reflect"
	"strings"
	"testing"
)

func TestRebaseApp(t *testing.T) {
	cases := map[string]string{
		`/app/report.txt`:                    `report.txt`,
		`Path("/app/report.txt")`:            `Path("report.txt")`,
		`the log is at /app/access_log here`: `the log is at access_log here`,
		`cd /app`:                            `cd .`,
		`cd /app && ls`:                      `cd . && ls`,
		`open("/app/logs/app1.log")`:         `open("logs/app1.log")`, // nested /app path, inner app untouched
		`/application/keep`:                  `/application/keep`,     // not a /app segment
		`no app path here`:                   `no app path here`,
	}
	for in, want := range cases {
		if got := rebaseApp(in); got != want {
			t.Errorf("rebaseApp(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRebaseAppLeavesRelativePathsAlone(t *testing.T) {
	// A test that already resolves against the work tree must not be rewritten.
	src := `report = Path("report.txt")
data = open("logs/db.log")`
	if got := rebaseApp(src); got != src {
		t.Errorf("rebaseApp rewrote already-relative source:\n%s", got)
	}
}

func TestTbScanDeps(t *testing.T) {
	src := `import numpy as np
from grid_transform import solve
import os, sys
import pandas as pd
from bs4 import BeautifulSoup
import mymodule
`
	got := tbScanDeps(src)
	want := []string{"beautifulsoup4", "numpy", "pandas"} // sorted; grid_transform/mymodule/os/sys are not third-party we install
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tbScanDeps = %v, want %v", got, want)
	}
}

func TestTbScanDepsEmpty(t *testing.T) {
	// A pure-stdlib grader needs nothing beyond pytest.
	src := `import re
import json
from pathlib import Path
`
	if got := tbScanDeps(src); len(got) != 0 {
		t.Errorf("tbScanDeps = %v, want empty", got)
	}
}

func TestTbPrompt(t *testing.T) {
	p := renderPrompt("terminalbench.md", "", "Write /app/hello.txt containing a greeting.")
	if !strings.Contains(p, "Write /app/hello.txt containing a greeting.") {
		t.Fatalf("prompt missing instruction:\n%s", p)
	}
	// The one closed door this tier holds shut is stated up front.
	if !strings.Contains(p, "no network access") {
		t.Error("prompt should state the network is closed")
	}
	// This tier carries no test instruction, the same as the swebench templates.
	for _, banned := range []string{"hidden test", "pytest", "do not edit"} {
		if strings.Contains(strings.ToLower(p), banned) {
			t.Errorf("prompt should carry no test instruction, found %q:\n%s", banned, p)
		}
	}
}
