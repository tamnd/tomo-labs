---
title: "Scenarios"
description: "Every task in the suite, from a baseline greeting through a small project scaffold, each graded by a checker that reads the files an agent left behind."
weight: 30
---

Ordinary tasks a capable agent should handle, each with a checker that grades the result on disk rather than on what the model said, plus the `00-hello` baseline:

| id | task |
| --- | --- |
| 00-hello | say hi, no task beyond completing the round trip |
| 01-file-organize | sort a flat pile of files into folders by extension |
| 02-json-transform | filter and sort a JSON array of users |
| 03-bugfix-fizzbuzz | fix a FizzBuzz that never prints FizzBuzz |
| 04-web-extract | fetch a page and name the cheapest product |
| 05-log-count | count HTTP 500s in an access log |
| 06-codegen-primes | write, build, and run a Go primes program |
| 07-refactor-dedupe | remove a duplicated function, keep the test green |
| 08-data-summary | total a sales CSV and find the top day |
| 09-project-scaffold | scaffold a small project and run its make target |
| 10-reasoning-calc | follow a precise two-step calculation into a file |

Run a single scenario against a single tool:

```bash
go run ./cmd/lab run tomo 03-bugfix-fizzbuzz
```

A scenario is a folder under `scenarios/` with a prompt, a setup script, and a checker; see [adding a tool](/guides/adding-a-tool/) for the layout if you want to add one of your own.
